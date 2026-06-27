package shipeasy

import (
	"testing"
)

// bindGlobal wires a live (non-localMode) engine pointed at the fake /collect
// server as the process-wide global, with the identity attrs transform, so
// NewClient(user) builds a bound Client that egresses to the test server.
func bindGlobal(eng *Engine) {
	globalEngineMu.Lock()
	globalEngine = eng
	globalAttrs = identityAttrsFn
	globalEngineMu.Unlock()
	SetDefaultEngine(eng)
}

// The bound Client.Track derives the unit id from the bound attribute map
// (user_id wins) and reaches the engine's /collect with that id and a metric
// event — no user argument is passed.
func TestBoundClientTrackUsesBoundUserID(t *testing.T) {
	resetGlobalsForTest()
	defer resetGlobalsForTest()

	cs := newCollectServer(t)
	eng := cs.liveClient(Options{})
	bindGlobal(eng)

	c := NewClient(User{"user_id": "u42", "plan": "pro"})
	cs.expect(1, func() { c.Track("checkout", map[string]any{"amount": 9.99}) })

	events := cs.all()
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 metric event, got %d: %v", len(events), events)
	}
	ev := events[0]
	if ev["type"] != "metric" {
		t.Errorf("type = %v, want metric", ev["type"])
	}
	if ev["event_name"] != "checkout" {
		t.Errorf("event_name = %v, want checkout", ev["event_name"])
	}
	if ev["user_id"] != "u42" {
		t.Errorf("user_id = %v, want u42 (derived from bound attrs)", ev["user_id"])
	}
	props, _ := ev["properties"].(map[string]any)
	if props["amount"] != 9.99 {
		t.Errorf("properties.amount = %v, want 9.99", props["amount"])
	}
}

// With no user_id, the bound Client.Track falls back to anonymous_id.
func TestBoundClientTrackFallsBackToAnonymousID(t *testing.T) {
	resetGlobalsForTest()
	defer resetGlobalsForTest()

	cs := newCollectServer(t)
	eng := cs.liveClient(Options{})
	bindGlobal(eng)

	c := NewClient(User{"anonymous_id": "anon-7"})
	cs.expect(1, func() { c.Track("view", nil) })

	ev := cs.all()[0]
	if ev["user_id"] != "anon-7" {
		t.Errorf("user_id = %v, want anon-7 (anonymous_id fallback)", ev["user_id"])
	}
}

// The bound Client.LogExposure re-evaluates the experiment against the bound
// attribute map and emits one exposure for an enrolled user — no user argument.
func TestBoundClientLogExposureUsesBoundAttrs(t *testing.T) {
	resetGlobalsForTest()
	defer resetGlobalsForTest()

	cs := newCollectServer(t)
	eng := cs.liveClient(Options{})
	eng.exps = &expsBlob{Experiments: map[string]experiment{
		"exp": {
			Status:        "running",
			Salt:          "saltvalue",
			AllocationPct: 10000,
			Groups:        []group{{Name: "control", Weight: 5000}, {Name: "treatment", Weight: 5000}},
		},
	}}
	eng.initialized = true
	bindGlobal(eng)

	c := NewClient(User{"user_id": "u42"})
	want := c.GetExperiment("exp", nil)
	if !want.InExperiment {
		t.Fatalf("precondition: u42 should be enrolled")
	}

	cs.expect(1, func() { c.LogExposure("exp") })

	events := cs.all()
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 exposure event, got %d: %v", len(events), events)
	}
	ev := events[0]
	if ev["type"] != "exposure" {
		t.Errorf("type = %v, want exposure", ev["type"])
	}
	if ev["experiment"] != "exp" {
		t.Errorf("experiment = %v, want exp", ev["experiment"])
	}
	if ev["group"] != want.Group {
		t.Errorf("group = %v, want %v", ev["group"], want.Group)
	}
	if ev["user_id"] != "u42" {
		t.Errorf("user_id = %v, want u42", ev["user_id"])
	}
}
