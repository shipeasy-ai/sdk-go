package shipeasy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---- Feature A: GetFlagOr / GetConfigOr ----

func TestGetFlagOr(t *testing.T) {
	c := NewTestClient()

	// Not found → returns def (both true and false defaults).
	if got := c.GetFlagOr("missing", User{}, true); got != true {
		t.Errorf("GetFlagOr(missing, def=true) = %v, want true (FLAG_NOT_FOUND)", got)
	}
	if got := c.GetFlagOr("missing", User{}, false); got != false {
		t.Errorf("GetFlagOr(missing, def=false) = %v, want false", got)
	}

	// Evaluates to false → returns the evaluated false, NOT def.
	c.OverrideFlag("off_flag", false)
	if got := c.GetFlagOr("off_flag", User{}, true); got != false {
		t.Errorf("GetFlagOr(off_flag, def=true) = %v, want false (evaluated false beats def)", got)
	}

	// Evaluates to true.
	c.OverrideFlag("on_flag", true)
	if got := c.GetFlagOr("on_flag", User{}, false); got != true {
		t.Errorf("GetFlagOr(on_flag, def=false) = %v, want true", got)
	}
}

// def returned when the client is not ready (uninitialized).
func TestGetFlagOrNotReady(t *testing.T) {
	c := NewClient(Options{APIKey: "k", DisableTelemetry: true})
	// Never Init'd → initialized=false.
	if got := c.GetFlagOr("anything", User{}, true); got != true {
		t.Errorf("GetFlagOr on not-ready client = %v, want def=true (CLIENT_NOT_READY)", got)
	}
}

func TestGetConfigOr(t *testing.T) {
	c := NewTestClient()
	if got := c.GetConfigOr("missing", "fallback"); got != "fallback" {
		t.Errorf("GetConfigOr(missing) = %v, want fallback", got)
	}
	c.OverrideConfig("present", map[string]any{"k": "v"})
	got := c.GetConfigOr("present", "fallback")
	m, _ := got.(map[string]any)
	if m["k"] != "v" {
		t.Errorf("GetConfigOr(present) = %v, want the real value", got)
	}
}

// ---- Feature B: GetFlagDetail reasons ----

func TestGetFlagDetailReasons(t *testing.T) {
	// OVERRIDE
	c := NewTestClient()
	c.OverrideFlag("ov", true)
	if d := c.GetFlagDetail("ov", User{}); d.Reason != ReasonOverride || !d.Value {
		t.Errorf("override: got %+v, want {true OVERRIDE}", d)
	}

	// CLIENT_NOT_READY (uninitialized, no blob)
	notReady := NewClient(Options{APIKey: "k", DisableTelemetry: true})
	if d := notReady.GetFlagDetail("x", User{}); d.Reason != ReasonClientNotReady || d.Value {
		t.Errorf("not-ready: got %+v, want {false CLIENT_NOT_READY}", d)
	}

	// Build a client with a real blob (initialized) for the remaining reasons.
	withBlob := NewOfflineClientFromSnapshot(map[string]any{
		"gates": map[string]any{
			"off":   map[string]any{"enabled": false, "salt": "s", "rolloutPct": 10000},
			"kill":  map[string]any{"enabled": true, "killswitch": true, "salt": "s", "rolloutPct": 10000},
			"fullon": map[string]any{"enabled": true, "salt": "s", "rolloutPct": 10000},
			"zero":  map[string]any{"enabled": true, "salt": "s", "rolloutPct": 0},
			"rule": map[string]any{
				"enabled": true, "salt": "s", "rolloutPct": 10000,
				"rules": []any{map[string]any{"attr": "plan", "op": "eq", "value": "pro"}},
			},
		},
		"configs": map[string]any{},
	}, nil)

	// FLAG_NOT_FOUND
	if d := withBlob.GetFlagDetail("nope", User{}); d.Reason != ReasonFlagNotFound || d.Value {
		t.Errorf("not-found: got %+v, want {false FLAG_NOT_FOUND}", d)
	}
	// OFF (disabled)
	if d := withBlob.GetFlagDetail("off", User{"user_id": "u"}); d.Reason != ReasonOff || d.Value {
		t.Errorf("off: got %+v, want {false OFF}", d)
	}
	// OFF (killswitch)
	if d := withBlob.GetFlagDetail("kill", User{"user_id": "u"}); d.Reason != ReasonOff || d.Value {
		t.Errorf("killswitch: got %+v, want {false OFF}", d)
	}
	// RULE_MATCH (fully rolled, enabled → true)
	if d := withBlob.GetFlagDetail("fullon", User{"user_id": "u"}); d.Reason != ReasonRuleMatch || !d.Value {
		t.Errorf("full-on: got %+v, want {true RULE_MATCH}", d)
	}
	// DEFAULT (enabled but 0% rollout for an identified user → false)
	if d := withBlob.GetFlagDetail("zero", User{"user_id": "u"}); d.Reason != ReasonDefault || d.Value {
		t.Errorf("zero-rollout: got %+v, want {false DEFAULT}", d)
	}
	// DEFAULT (enabled, rule does not match → false)
	if d := withBlob.GetFlagDetail("rule", User{"user_id": "u", "plan": "free"}); d.Reason != ReasonDefault || d.Value {
		t.Errorf("rule-miss: got %+v, want {false DEFAULT}", d)
	}
	// RULE_MATCH (rule matches, fully rolled)
	if d := withBlob.GetFlagDetail("rule", User{"user_id": "u", "plan": "pro"}); d.Reason != ReasonRuleMatch || !d.Value {
		t.Errorf("rule-match: got %+v, want {true RULE_MATCH}", d)
	}

	// GetFlag delegates to .Value.
	if !withBlob.GetFlag("fullon", User{"user_id": "u"}) {
		t.Errorf("GetFlag should mirror GetFlagDetail.Value")
	}
}

// ---- Feature C: OnChange ----

// changingRT serves a flags/exps body that changes (and ETags that change)
// once switched. It lets a poll observe a 200 with new data.
type changingRT struct {
	mu      sync.Mutex
	version int
}

func (r *changingRT) bump() {
	r.mu.Lock()
	r.version++
	r.mu.Unlock()
}

func (r *changingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	v := r.version
	r.mu.Unlock()
	etag := "v" + string(rune('0'+v))

	// If the client already has this version's ETag, reply 304.
	if req.Header.Get("If-None-Match") == etag {
		return &http.Response{
			StatusCode: http.StatusNotModified,
			Header:     http.Header{},
			Body:       io.NopCloser(bytes.NewReader(nil)),
		}, nil
	}

	var body []byte
	if req.URL.Path == "/sdk/flags" {
		body = []byte(`{"gates":{},"configs":{}}`)
	} else {
		body = []byte(`{"experiments":{},"universes":{}}`)
	}
	h := http.Header{}
	h.Set("ETag", etag)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     h,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

func TestOnChangeFiresOnNewData(t *testing.T) {
	rt := &changingRT{}
	c := NewClient(Options{APIKey: "k", DisableTelemetry: true, HTTP: &http.Client{Transport: rt}})

	var fired int32
	cancel := c.OnChange(func() { atomic.AddInt32(&fired, 1) })

	// Initial fetch (version 0): a 200, but Init doesn't fire listeners.
	if _, err := c.fetchAll(context.Background()); err != nil {
		t.Fatalf("initial fetchAll: %v", err)
	}

	// Same version again → 304s, no change → no fire.
	changed, err := c.fetchAll(context.Background())
	if err != nil {
		t.Fatalf("second fetchAll: %v", err)
	}
	if changed {
		t.Fatalf("expected no change on unchanged version")
	}
	if changed && !c.localMode {
		c.fireListeners()
	}
	if atomic.LoadInt32(&fired) != 0 {
		t.Fatalf("listener should not fire when nothing changed")
	}

	// New version → 200 with fresh data → change → fire.
	rt.bump()
	changed, err = c.fetchAll(context.Background())
	if err != nil {
		t.Fatalf("third fetchAll: %v", err)
	}
	if !changed {
		t.Fatalf("expected change after version bump")
	}
	if changed && !c.localMode {
		c.fireListeners()
	}
	if atomic.LoadInt32(&fired) != 1 {
		t.Fatalf("listener should fire once on new data, got %d", atomic.LoadInt32(&fired))
	}

	// Cancel deregisters: another change must not fire.
	cancel()
	rt.bump()
	changed, _ = c.fetchAll(context.Background())
	if changed && !c.localMode {
		c.fireListeners()
	}
	if atomic.LoadInt32(&fired) != 1 {
		t.Fatalf("cancelled listener should not fire again, got %d", atomic.LoadInt32(&fired))
	}
}

// A panicking listener must not crash fireListeners or block the others.
func TestOnChangeListenerPanicRecovered(t *testing.T) {
	c := NewClient(Options{APIKey: "k", DisableTelemetry: true})
	c.OnChange(func() { panic("boom") })
	var ok int32
	c.OnChange(func() { atomic.AddInt32(&ok, 1) })
	c.fireListeners() // must not panic
	if atomic.LoadInt32(&ok) != 1 {
		t.Fatalf("a panicking listener should not stop the others")
	}
}

// Listeners never fire in localMode (the poll guard); verify the guard logic by
// confirming a test client never reaches fireListeners via a poll.
func TestOnChangeLocalModeNoFire(t *testing.T) {
	c := NewTestClient()
	var fired int32
	c.OnChange(func() { atomic.AddInt32(&fired, 1) })
	// localMode clients don't poll; simulate the poll guard explicitly.
	changed := true
	if changed && !c.localMode {
		c.fireListeners()
	}
	if atomic.LoadInt32(&fired) != 0 {
		t.Fatalf("localMode must not fire listeners")
	}
}

// ---- Feature D: offline client ----

func TestNewOfflineClientFromSnapshot(t *testing.T) {
	flags := map[string]any{
		"gates": map[string]any{
			"on": map[string]any{"enabled": true, "salt": "s", "rolloutPct": 10000},
		},
		"configs": map[string]any{
			"copy": map[string]any{"value": map[string]any{"cta": "Buy"}},
		},
	}
	exps := map[string]any{
		"experiments": map[string]any{
			"exp": map[string]any{
				"status": "running", "salt": "s", "allocationPct": 10000,
				"groups": []any{
					map[string]any{"name": "control", "weight": 5000, "params": map[string]any{"v": 1}},
					map[string]any{"name": "treatment", "weight": 5000, "params": map[string]any{"v": 2}},
				},
			},
		},
		"universes": map[string]any{},
	}

	c := NewOfflineClientFromSnapshot(flags, exps)
	c.http = &http.Client{Transport: failRT{t}} // prove no network

	if err := c.Init(context.Background()); err != nil {
		t.Fatalf("Init offline: %v", err)
	}
	if !c.GetFlag("on", User{"user_id": "u"}) {
		t.Errorf("offline flag should evaluate true")
	}
	if v, ok := c.GetConfig("copy"); !ok {
		t.Errorf("offline config missing")
	} else if m, _ := v.(map[string]any); m["cta"] != "Buy" {
		t.Errorf("offline config value = %v", v)
	}
	r := c.GetExperiment("exp", User{"user_id": "u"}, nil)
	if !r.InExperiment {
		t.Errorf("offline experiment should enrol identified user")
	}

	// Overrides apply on top of the snapshot.
	c.OverrideFlag("on", false)
	if c.GetFlag("on", User{"user_id": "u"}) {
		t.Errorf("override should win over snapshot")
	}

	// Track is a no-op (no network).
	c.Track("u", "evt", nil)
}

func TestNewOfflineClientFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.json")
	snap := map[string]any{
		"flags": map[string]any{
			"gates": map[string]any{
				"beta": map[string]any{"enabled": true, "salt": "s", "rolloutPct": 10000},
			},
			"configs": map[string]any{},
		},
		"experiments": map[string]any{
			"experiments": map[string]any{},
			"universes":   map[string]any{},
		},
	}
	raw, _ := json.Marshal(snap)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := NewOfflineClient(path)
	if err != nil {
		t.Fatalf("NewOfflineClient: %v", err)
	}
	c.http = &http.Client{Transport: failRT{t}}
	if !c.GetFlag("beta", User{"user_id": "u"}) {
		t.Errorf("offline file flag should be true")
	}
	if c.GetFlag("absent", User{"user_id": "u"}) {
		t.Errorf("absent flag should be false")
	}

	// Missing file → error.
	if _, err := NewOfflineClient(filepath.Join(dir, "nope.json")); err == nil {
		t.Errorf("expected error for missing snapshot file")
	}
}

// Sanity: GetFlagDetail on a never-initialized client always reports not-ready,
// even with a poll interval that would otherwise tick.
func TestDetailNotReadyStable(t *testing.T) {
	c := NewClient(Options{APIKey: "k", DisableTelemetry: true})
	c.pollInterval = time.Hour
	if d := c.GetFlagDetail("x", User{}); d.Reason != ReasonClientNotReady {
		t.Errorf("reason = %q, want CLIENT_NOT_READY", d.Reason)
	}
}
