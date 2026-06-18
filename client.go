package shipeasy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const defaultBaseURL = "https://edge.shipeasy.dev"

type Client struct {
	apiKey       string
	baseURL      string
	http         *http.Client
	mu           sync.RWMutex
	flags        *flagsBlob
	exps         *expsBlob
	flagsETag    string
	expsETag     string
	pollInterval time.Duration
	stop         chan struct{}
	once         sync.Once
	initialized  bool
	telemetry    *telemetry

	// localMode is set by NewTestClient: Init/InitOnce never fetch and Track
	// is a no-op, so the client does zero network and is usable immediately.
	localMode bool
	// Local overrides win over fetched data in the getters. Guarded by mu
	// (the same RWMutex that protects flags/exps).
	flagOverrides   map[string]bool
	configOverrides map[string]any
	expOverrides    map[string]ExperimentResult

	// changeListeners fire after a background poll fetches NEW data (a 200, not
	// a 304). Guarded by mu. Never fired in localMode.
	changeListeners map[int]func()
	nextListenerID  int
}

type Options struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client
	// Env is the published env reported in usage telemetry (defaults to "prod").
	Env string
	// DisableTelemetry turns off per-evaluation usage beacons (ON by default).
	DisableTelemetry bool
	// TelemetryURL overrides the beacon host (defaults to defaultTelemetryURL).
	TelemetryURL string
}

func NewClient(opts Options) *Client {
	base := opts.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	hc := opts.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	env := opts.Env
	if env == "" {
		env = "prod"
	}
	telemetryURL := opts.TelemetryURL
	if telemetryURL == "" {
		telemetryURL = defaultTelemetryURL
	}
	return &Client{
		apiKey:       opts.APIKey,
		baseURL:      base,
		http:         hc,
		pollInterval: 30 * time.Second,
		stop:         make(chan struct{}),
		telemetry:    newTelemetry(telemetryURL, opts.APIKey, "server", env, opts.DisableTelemetry, hc),
	}
}

func (c *Client) Init(ctx context.Context) error {
	if c.localMode {
		return nil
	}
	if _, err := c.fetchAll(ctx); err != nil {
		return err
	}
	c.initialized = true
	go c.pollLoop()
	return nil
}

func (c *Client) InitOnce(ctx context.Context) error {
	if c.localMode || c.initialized {
		return nil
	}
	if _, err := c.fetchAll(ctx); err != nil {
		return err
	}
	c.initialized = true
	return nil
}

func (c *Client) Destroy() {
	c.once.Do(func() { close(c.stop) })
}

// Exported evaluation reasons returned in FlagDetail.Reason. They explain why
// GetFlagDetail produced its value without exposing the canonical evaluator.
const (
	// ReasonClientNotReady means Init/InitOnce has not completed, so no flag
	// blob is available yet. Value is false.
	ReasonClientNotReady = "CLIENT_NOT_READY"
	// ReasonFlagNotFound means the gate is absent from the loaded blob. Value is
	// false.
	ReasonFlagNotFound = "FLAG_NOT_FOUND"
	// ReasonOff means the gate exists but is disabled (or killswitched). Value is
	// false.
	ReasonOff = "OFF"
	// ReasonOverride means a local override (OverrideFlag) supplied the value,
	// short-circuiting evaluation and telemetry.
	ReasonOverride = "OVERRIDE"
	// ReasonRuleMatch means the gate evaluated to true for this user.
	ReasonRuleMatch = "RULE_MATCH"
	// ReasonDefault means the gate evaluated to false for this user (rules did
	// not match, or the user fell outside the rollout).
	ReasonDefault = "DEFAULT"
)

// FlagDetail is the result of GetFlagDetail: the boolean value plus a stable,
// exported reason explaining how it was reached.
type FlagDetail struct {
	Value  bool
	Reason string
}

// GetFlagDetail evaluates a flag and reports why. The reason is computed at the
// boundary without touching the canonical evaluator. Telemetry for the "gate"
// resource is emitted exactly once here for steps 2-5, and never for an
// OVERRIDE (which short-circuits before telemetry, matching GetFlag's override
// path).
func (c *Client) GetFlagDetail(name string, user User) FlagDetail {
	// 1. Override wins, short-circuit before telemetry.
	c.mu.RLock()
	if v, ok := c.flagOverrides[name]; ok {
		c.mu.RUnlock()
		return FlagDetail{Value: v, Reason: ReasonOverride}
	}
	flags := c.flags
	initialized := c.initialized
	c.mu.RUnlock()

	c.telemetry.emit("gate", name)

	// 2. Not initialized / no blob loaded yet.
	if !initialized || flags == nil {
		return FlagDetail{Value: false, Reason: ReasonClientNotReady}
	}
	// 3. Gate not present in the blob.
	g, ok := flags.Gates[name]
	if !ok {
		return FlagDetail{Value: false, Reason: ReasonFlagNotFound}
	}
	// 4. Gate present but disabled (killswitched or not enabled). These are the
	// same fields evalGate short-circuits on, so reading them here is faithful.
	if enabled(g.Killswitch) || !enabled(g.Enabled) {
		return FlagDetail{Value: false, Reason: ReasonOff}
	}
	// 5. Run the canonical evaluator.
	v := evalGate(g, user)
	if v {
		return FlagDetail{Value: true, Reason: ReasonRuleMatch}
	}
	return FlagDetail{Value: false, Reason: ReasonDefault}
}

func (c *Client) GetFlag(name string, user User) bool {
	return c.GetFlagDetail(name, user).Value
}

// GetFlagOr returns def only when the flag CANNOT be evaluated — the client is
// not initialized (CLIENT_NOT_READY) or the gate is absent (FLAG_NOT_FOUND).
// When the flag evaluates (including to false), the evaluated value is returned.
func (c *Client) GetFlagOr(name string, user User, def bool) bool {
	d := c.GetFlagDetail(name, user)
	if d.Reason == ReasonClientNotReady || d.Reason == ReasonFlagNotFound {
		return def
	}
	return d.Value
}

// GetConfigOr returns the config value, or def when the config key is absent.
// GetConfig remains the (value, ok) form.
func (c *Client) GetConfigOr(name string, def any) any {
	if v, ok := c.GetConfig(name); ok {
		return v
	}
	return def
}

func (c *Client) GetConfig(name string) (any, bool) {
	c.telemetry.emit("config", name)
	c.mu.RLock()
	defer c.mu.RUnlock()
	if v, ok := c.configOverrides[name]; ok {
		return v, true
	}
	if c.flags == nil {
		return nil, false
	}
	cfg, ok := c.flags.Configs[name]
	if !ok {
		return nil, false
	}
	return cfg.Value, true
}

func (c *Client) GetExperiment(name string, user User, defaultParams any) ExperimentResult {
	c.telemetry.emit("experiment", name)
	c.mu.RLock()
	if r, ok := c.expOverrides[name]; ok {
		c.mu.RUnlock()
		if r.Params == nil {
			r.Params = defaultParams
		}
		return r
	}
	flags := c.flags
	exps := c.exps
	c.mu.RUnlock()
	var exp *experiment
	if exps != nil {
		if e, ok := exps.Experiments[name]; ok {
			exp = &e
		}
	}
	r := evalExperiment(exp, flags, exps, user)
	if r.Params == nil {
		r.Params = defaultParams
	}
	return r
}

func (c *Client) Track(userID, eventName string, properties map[string]any) {
	if c.localMode {
		return
	}
	event := map[string]any{
		"type":       "metric",
		"event_name": eventName,
		"user_id":    userID,
		"ts":         time.Now().UnixMilli(),
	}
	if len(properties) > 0 {
		event["properties"] = properties
	}
	body, err := json.Marshal(map[string]any{"events": []any{event}})
	if err != nil {
		return
	}
	go func() {
		if err := c.post("/collect", body); err != nil {
			log.Printf("[shipeasy] track failed: %v", err)
		}
	}()
}

// OnChange registers a listener fired after a background poll loads NEW data (a
// 200 response, not a 304). It returns a cancel function that deregisters the
// listener. Listeners never fire in localMode (no polling happens there).
func (c *Client) OnChange(fn func()) (cancel func()) {
	c.mu.Lock()
	if c.changeListeners == nil {
		c.changeListeners = map[int]func(){}
	}
	id := c.nextListenerID
	c.nextListenerID++
	c.changeListeners[id] = fn
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		delete(c.changeListeners, id)
		c.mu.Unlock()
	}
}

// fireListeners invokes every registered change listener, recovering from any
// panic so one bad listener can't take down the poll goroutine.
func (c *Client) fireListeners() {
	c.mu.RLock()
	fns := make([]func(), 0, len(c.changeListeners))
	for _, fn := range c.changeListeners {
		fns = append(fns, fn)
	}
	c.mu.RUnlock()
	for _, fn := range fns {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[shipeasy] change listener panicked: %v", r)
				}
			}()
			fn()
		}()
	}
}

func (c *Client) pollLoop() {
	for {
		select {
		case <-c.stop:
			return
		case <-time.After(c.pollInterval):
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			changed, err := c.fetchAll(ctx)
			cancel()
			if err != nil {
				log.Printf("[shipeasy] poll failed: %v", err)
				continue
			}
			if changed && !c.localMode {
				c.fireListeners()
			}
		}
	}
}

// fetchAll fetches both blobs and reports whether either returned NEW data (a
// 200, not a 304).
func (c *Client) fetchAll(ctx context.Context) (bool, error) {
	interval, flagsChanged, err := c.fetchFlags(ctx)
	if err != nil {
		return false, err
	}
	expsChanged, err := c.fetchExps(ctx)
	if err != nil {
		return false, err
	}
	if interval > 0 && time.Duration(interval)*time.Second != c.pollInterval {
		c.pollInterval = time.Duration(interval) * time.Second
	}
	return flagsChanged || expsChanged, nil
}

// fetchFlags returns (pollInterval, changed, error); changed is true only when
// the response was a 200 with a fresh blob (not a 304).
func (c *Client) fetchFlags(ctx context.Context) (int, bool, error) {
	status, headers, body, err := c.get(ctx, "/sdk/flags", c.flagsETag)
	if err != nil {
		return 0, false, err
	}
	intervalStr := headers.Get("X-Poll-Interval")
	interval := 0
	if intervalStr != "" {
		interval, _ = strconv.Atoi(intervalStr)
	}
	if status == http.StatusNotModified {
		return interval, false, nil
	}
	if status != http.StatusOK {
		return 0, false, fmt.Errorf("/sdk/flags: %d", status)
	}
	var blob flagsBlob
	if err := json.Unmarshal(body, &blob); err != nil {
		return 0, false, err
	}
	c.mu.Lock()
	if etag := headers.Get("ETag"); etag != "" {
		c.flagsETag = etag
	}
	c.flags = &blob
	c.mu.Unlock()
	return interval, true, nil
}

// fetchExps returns (changed, error); changed is true only when the response
// was a 200 with a fresh blob (not a 304).
func (c *Client) fetchExps(ctx context.Context) (bool, error) {
	status, headers, body, err := c.get(ctx, "/sdk/experiments", c.expsETag)
	if err != nil {
		return false, err
	}
	if status == http.StatusNotModified {
		return false, nil
	}
	if status != http.StatusOK {
		return false, fmt.Errorf("/sdk/experiments: %d", status)
	}
	var blob expsBlob
	if err := json.Unmarshal(body, &blob); err != nil {
		return false, err
	}
	c.mu.Lock()
	if etag := headers.Get("ETag"); etag != "" {
		c.expsETag = etag
	}
	c.exps = &blob
	c.mu.Unlock()
	return true, nil
}

func (c *Client) get(ctx context.Context, path, etag string) (int, http.Header, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("X-SDK-Key", c.apiKey)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, body, err
}

func (c *Client) post(path string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-SDK-Key", c.apiKey)
	req.Header.Set("Content-Type", "text/plain")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("POST %s: %d", path, res.StatusCode)
	}
	return nil
}
