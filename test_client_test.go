package shipeasy

import (
	"context"
	"net/http"
	"testing"
)

// failRT fails the test if any HTTP request is attempted — proves zero network.
type failRT struct{ t *testing.T }

func (f failRT) RoundTrip(req *http.Request) (*http.Response, error) {
	f.t.Fatalf("unexpected network call to %s", req.URL.String())
	return nil, nil
}

// NewTestClient must need no network: Init/InitOnce never fetch and return nil,
// even with an HTTP client that fails on any request.
func TestNewTestClientNoNetwork(t *testing.T) {
	c := NewTestClient()
	c.http = &http.Client{Transport: failRT{t}}

	if err := c.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := c.InitOnce(context.Background()); err != nil {
		t.Fatalf("InitOnce: %v", err)
	}

	// With no overrides and no fetched data, getters return zero values.
	if c.GetFlag("missing", User{}) {
		t.Errorf("unseeded flag should be false")
	}
	if v, ok := c.GetConfig("missing"); ok || v != nil {
		t.Errorf("unseeded config should be (nil,false); got (%v,%v)", v, ok)
	}
	if r := c.GetExperiment("missing", User{}, nil); r.InExperiment {
		t.Errorf("unseeded experiment should not be in-experiment")
	}
}

func TestOverrideFlag(t *testing.T) {
	c := NewTestClient()
	c.OverrideFlag("new_checkout", true)
	if !c.GetFlag("new_checkout", User{}) {
		t.Errorf("OverrideFlag(true) not reflected by GetFlag")
	}
	c.OverrideFlag("new_checkout", false)
	if c.GetFlag("new_checkout", User{}) {
		t.Errorf("OverrideFlag(false) not reflected by GetFlag")
	}
}

func TestOverrideConfig(t *testing.T) {
	c := NewTestClient()
	want := map[string]any{"cta": "Buy now"}
	c.OverrideConfig("billing_copy", want)
	v, ok := c.GetConfig("billing_copy")
	if !ok {
		t.Fatalf("GetConfig override should return ok=true")
	}
	m, _ := v.(map[string]any)
	if m["cta"] != "Buy now" {
		t.Errorf("GetConfig override value mismatch: %v", v)
	}
}

func TestOverrideExperiment(t *testing.T) {
	c := NewTestClient()
	params := map[string]any{"color": "green"}
	c.OverrideExperiment("checkout_button", "treatment", params)
	r := c.GetExperiment("checkout_button", User{}, map[string]any{"color": "blue"})
	if !r.InExperiment {
		t.Errorf("override experiment should be InExperiment")
	}
	if r.Group != "treatment" {
		t.Errorf("group = %q, want treatment", r.Group)
	}
	m, _ := r.Params.(map[string]any)
	if m["color"] != "green" {
		t.Errorf("params = %v, want color=green (override wins over default)", r.Params)
	}
}

// A nil-params override still falls back to the call's defaultParams.
func TestOverrideExperimentNilParamsUsesDefault(t *testing.T) {
	c := NewTestClient()
	c.OverrideExperiment("exp", "control", nil)
	r := c.GetExperiment("exp", User{}, map[string]any{"k": "default"})
	if !r.InExperiment || r.Group != "control" {
		t.Fatalf("override not applied: %+v", r)
	}
	m, _ := r.Params.(map[string]any)
	if m["k"] != "default" {
		t.Errorf("nil-params override should fall back to defaultParams; got %v", r.Params)
	}
}

func TestClearOverrides(t *testing.T) {
	c := NewTestClient()
	c.OverrideFlag("f", true)
	c.OverrideConfig("c", 1)
	c.OverrideExperiment("e", "g", nil)
	c.ClearOverrides()

	if c.GetFlag("f", User{}) {
		t.Errorf("flag override should be cleared")
	}
	if _, ok := c.GetConfig("c"); ok {
		t.Errorf("config override should be cleared")
	}
	if r := c.GetExperiment("e", User{}, nil); r.InExperiment {
		t.Errorf("experiment override should be cleared")
	}
}

func TestTrackNoOp(t *testing.T) {
	c := NewTestClient()
	c.http = &http.Client{Transport: failRT{t}}
	// Must not panic and must not hit the network.
	c.Track("u_1", "purchase", map[string]any{"amount": 49})
}

// Overrides also work on a normal client and win over fetched data.
func TestOverrideWinsOnNormalClient(t *testing.T) {
	c := NewClient(Options{APIKey: "k", DisableTelemetry: true})
	c.OverrideFlag("g", true)
	if !c.GetFlag("g", User{"user_id": "u"}) {
		t.Errorf("override should win even on a normal client")
	}
}
