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
	if a := c.Universe("missing").Assign(User{}); a.Enrolled {
		t.Errorf("unseeded universe should not enrol")
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

// seedRunningExp loads one running experiment (in universe "u") into a test
// engine so an OverrideExperiment on it surfaces through Universe().Assign() —
// assign only considers running candidates in the loaded blob (the override still
// wins over the fetched groups).
func seedRunningExp(c *Engine, expName string) {
	c.mu.Lock()
	c.exps = &expsBlob{
		Universes: map[string]universe{"u": {}},
		Experiments: map[string]experiment{
			expName: {Status: "running", Universe: "u", Salt: "s", AllocationPct: 10000,
				Groups: []group{{Name: "control", Weight: 10000}}},
		},
	}
	c.mu.Unlock()
}

func TestOverrideExperiment(t *testing.T) {
	c := NewTestClient()
	seedRunningExp(c, "checkout_button")
	c.OverrideExperiment("checkout_button", "treatment", map[string]any{"color": "green"})
	a := c.Universe("u").Assign(User{})
	if !a.Enrolled {
		t.Errorf("override experiment should be enrolled")
	}
	if a.Name != "checkout_button" {
		t.Errorf("name = %q, want checkout_button", a.Name)
	}
	if a.Group != "treatment" {
		t.Errorf("group = %q, want treatment", a.Group)
	}
	if a.Get("color", "blue") != "green" {
		t.Errorf("color = %v, want green (override wins)", a.Get("color", "blue"))
	}
}

func TestClearOverrides(t *testing.T) {
	c := NewTestClient()
	seedRunningExp(c, "e")
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
	// Override cleared → assign falls back to the seeded experiment's real groups
	// (still enrolled, but in "control", not the forced "g").
	if a := c.Universe("u").Assign(User{"user_id": "u1"}); a.Group == "g" {
		t.Errorf("experiment override should be cleared, still saw forced group %q", a.Group)
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
	c := NewEngine(Options{APIKey: "k", DisableTelemetry: true})
	c.OverrideFlag("g", true)
	if !c.GetFlag("g", User{"user_id": "u"}) {
		t.Errorf("override should win even on a normal client")
	}
}
