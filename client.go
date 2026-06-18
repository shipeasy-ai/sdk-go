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
	if err := c.fetchAll(ctx); err != nil {
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
	if err := c.fetchAll(ctx); err != nil {
		return err
	}
	c.initialized = true
	return nil
}

func (c *Client) Destroy() {
	c.once.Do(func() { close(c.stop) })
}

func (c *Client) GetFlag(name string, user User) bool {
	c.telemetry.emit("gate", name)
	c.mu.RLock()
	defer c.mu.RUnlock()
	if v, ok := c.flagOverrides[name]; ok {
		return v
	}
	if c.flags == nil {
		return false
	}
	g, ok := c.flags.Gates[name]
	if !ok {
		return false
	}
	return evalGate(g, user)
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

func (c *Client) pollLoop() {
	for {
		select {
		case <-c.stop:
			return
		case <-time.After(c.pollInterval):
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := c.fetchAll(ctx); err != nil {
				log.Printf("[shipeasy] poll failed: %v", err)
			}
			cancel()
		}
	}
}

func (c *Client) fetchAll(ctx context.Context) error {
	interval, err := c.fetchFlags(ctx)
	if err != nil {
		return err
	}
	if err := c.fetchExps(ctx); err != nil {
		return err
	}
	if interval > 0 && time.Duration(interval)*time.Second != c.pollInterval {
		c.pollInterval = time.Duration(interval) * time.Second
	}
	return nil
}

func (c *Client) fetchFlags(ctx context.Context) (int, error) {
	status, headers, body, err := c.get(ctx, "/sdk/flags", c.flagsETag)
	if err != nil {
		return 0, err
	}
	intervalStr := headers.Get("X-Poll-Interval")
	interval := 0
	if intervalStr != "" {
		interval, _ = strconv.Atoi(intervalStr)
	}
	if status == http.StatusNotModified {
		return interval, nil
	}
	if status != http.StatusOK {
		return 0, fmt.Errorf("/sdk/flags: %d", status)
	}
	var blob flagsBlob
	if err := json.Unmarshal(body, &blob); err != nil {
		return 0, err
	}
	c.mu.Lock()
	if etag := headers.Get("ETag"); etag != "" {
		c.flagsETag = etag
	}
	c.flags = &blob
	c.mu.Unlock()
	return interval, nil
}

func (c *Client) fetchExps(ctx context.Context) error {
	status, headers, body, err := c.get(ctx, "/sdk/experiments", c.expsETag)
	if err != nil {
		return err
	}
	if status == http.StatusNotModified {
		return nil
	}
	if status != http.StatusOK {
		return fmt.Errorf("/sdk/experiments: %d", status)
	}
	var blob expsBlob
	if err := json.Unmarshal(body, &blob); err != nil {
		return err
	}
	c.mu.Lock()
	if etag := headers.Get("ETag"); etag != "" {
		c.expsETag = etag
	}
	c.exps = &blob
	c.mu.Unlock()
	return nil
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
