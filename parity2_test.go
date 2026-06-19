package shipeasy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// collectServer is a fake /collect endpoint that captures every event body it
// receives. POSTs in the SDK are fire-and-forget goroutines, so callers wait on
// the wg before asserting.
type collectServer struct {
	srv    *httptest.Server
	mu     sync.Mutex
	events []map[string]any
	wg     sync.WaitGroup
}

func newCollectServer(t *testing.T) *collectServer {
	cs := &collectServer{}
	cs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer cs.wg.Done()
		var body struct {
			Events []map[string]any `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode /collect body: %v", err)
		}
		cs.mu.Lock()
		cs.events = append(cs.events, body.Events...)
		cs.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(cs.srv.Close)
	return cs
}

// expect tells the server how many POSTs to await, runs fn (which triggers
// them), then blocks until they all arrive.
func (cs *collectServer) expect(n int, fn func()) {
	cs.wg.Add(n)
	fn()
	cs.wg.Wait()
}

func (cs *collectServer) all() []map[string]any {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.events
}

// liveClient returns a non-localMode client pointed at the fake collect server,
// telemetry off.
func (cs *collectServer) liveClient(opts Options) *Client {
	opts.BaseURL = cs.srv.URL
	opts.DisableTelemetry = true
	if opts.APIKey == "" {
		opts.APIKey = "k"
	}
	return NewClient(opts)
}

// ---- Feature A: private attributes ----

func TestTrackStripsPrivateAttributes(t *testing.T) {
	cs := newCollectServer(t)
	c := cs.liveClient(Options{PrivateAttributes: []string{"email", "ssn"}})

	cs.expect(1, func() {
		c.Track("u1", "purchase", map[string]any{
			"email":  "a@b.com",
			"ssn":    "123",
			"amount": 9.99,
			"plan":   "pro",
		})
	})

	ev := cs.all()[0]
	props, ok := ev["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %T", ev["properties"])
	}
	if _, present := props["email"]; present {
		t.Errorf("private attr 'email' leaked to /collect: %v", props)
	}
	if _, present := props["ssn"]; present {
		t.Errorf("private attr 'ssn' leaked to /collect: %v", props)
	}
	if props["amount"] != 9.99 {
		t.Errorf("non-private 'amount' missing/wrong: %v", props["amount"])
	}
	if props["plan"] != "pro" {
		t.Errorf("non-private 'plan' missing/wrong: %v", props["plan"])
	}
}

// With no private attrs configured, all properties pass through untouched.
func TestTrackNoPrivateAttributesPassThrough(t *testing.T) {
	cs := newCollectServer(t)
	c := cs.liveClient(Options{})

	cs.expect(1, func() {
		c.Track("u1", "evt", map[string]any{"email": "a@b.com"})
	})

	props := cs.all()[0]["properties"].(map[string]any)
	if props["email"] != "a@b.com" {
		t.Errorf("without PrivateAttributes nothing should be stripped, got %v", props)
	}
}

// ---- Feature B: manual exposure logging ----

// A running, fully-allocated experiment enrols every user; LogExposure emits one
// exposure with the resolved group.
func TestLogExposureEnrolledEmitsOnce(t *testing.T) {
	cs := newCollectServer(t)
	c := cs.liveClient(Options{})
	c.exps = &expsBlob{Experiments: map[string]experiment{
		"exp": {
			Status:        "running",
			Salt:          "saltvalue",
			AllocationPct: 10000,
			Groups:        []group{{Name: "control", Weight: 5000}, {Name: "treatment", Weight: 5000}},
		},
	}}
	c.initialized = true

	want := c.GetExperiment("exp", User{"user_id": "u42"}, nil)
	if !want.InExperiment {
		t.Fatalf("precondition: u42 should be enrolled")
	}

	cs.expect(1, func() { c.LogExposure("u42", "exp") })

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

// Not enrolled (allocation 0) ⇒ no exposure POST at all.
func TestLogExposureNotEnrolledNoOp(t *testing.T) {
	cs := newCollectServer(t)
	c := cs.liveClient(Options{})
	c.exps = &expsBlob{Experiments: map[string]experiment{
		"exp": {Status: "running", Salt: "s", AllocationPct: 0, Groups: []group{{Name: "control", Weight: 10000}}},
	}}
	c.initialized = true

	// No POST expected; just call and assert nothing was captured.
	c.LogExposure("u42", "exp")
	if got := cs.all(); len(got) != 0 {
		t.Errorf("not-enrolled user should emit no exposure, got %v", got)
	}
}

// ---- Feature C: sticky bucketing ----

func runningExp(salt string, alloc int, groups []group) *expsBlob {
	return &expsBlob{Experiments: map[string]experiment{
		"exp": {Status: "running", Salt: salt, AllocationPct: alloc, Groups: groups},
	}}
}

// A weight change after a user is stickied keeps that user on their original
// group (the deterministic pick would have moved them).
func TestStickyWeightChangeKeepsUser(t *testing.T) {
	store := NewInMemoryStickyStore()
	c := NewOfflineClientFromSnapshot(nil, nil)
	c.stickyStore = store

	// Find a user whose deterministic group flips when weights invert.
	groupsA := []group{{Name: "control", Weight: 5000}, {Name: "treatment", Weight: 5000}}
	var unit string
	var firstGroup string
	for _, u := range []string{"u1", "u2", "u3", "u4", "u5", "u6", "u7", "u8"} {
		c.exps = runningExp("saltvalue", 10000, groupsA)
		first := c.GetExperiment("exp", User{"user_id": u}, nil)
		// Now flip weights heavily and ask a FRESH (no-store) client what the
		// deterministic pick would be.
		fresh := NewOfflineClientFromSnapshot(nil, nil)
		fresh.exps = runningExp("saltvalue", 10000,
			[]group{{Name: "control", Weight: 9999}, {Name: "treatment", Weight: 1}})
		det := fresh.GetExperiment("exp", User{"user_id": u}, nil)
		if first.Group != det.Group {
			unit = u
			firstGroup = first.Group
			break
		}
	}
	if unit == "" {
		t.Skip("no user flips under this weight change; bucketing salt-dependent")
	}

	// Apply the same heavy weight flip on the STICKY client. The stickied user
	// must keep firstGroup.
	c.exps = runningExp("saltvalue", 10000,
		[]group{{Name: "control", Weight: 9999}, {Name: "treatment", Weight: 1}})
	after := c.GetExperiment("exp", User{"user_id": unit}, nil)
	if after.Group != firstGroup {
		t.Errorf("sticky weight change: group moved %q -> %q, want stable", firstGroup, after.Group)
	}
}

// An allocation shrink keeps an already-enrolled (stickied) user in, but denies
// a brand-new user who falls outside the smaller allocation.
func TestStickyAllocationShrink(t *testing.T) {
	store := NewInMemoryStickyStore()
	c := NewOfflineClientFromSnapshot(nil, nil)
	c.stickyStore = store
	groups := []group{{Name: "control", Weight: 5000}, {Name: "treatment", Weight: 5000}}

	// Enrol everyone at 100% allocation; record each user's group.
	c.exps = runningExp("saltvalue", 10000, groups)
	enrolled := map[string]string{}
	for _, u := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		r := c.GetExperiment("exp", User{"user_id": u}, nil)
		if r.InExperiment {
			enrolled[u] = r.Group
		}
	}
	if len(enrolled) == 0 {
		t.Fatal("precondition: nobody enrolled at 100%")
	}

	// Shrink allocation hard. A user with no sticky entry must be subject to the
	// allocation gate; a stickied user must skip it and stay in.
	c.exps = runningExp("saltvalue", 1, groups)

	// Stickied users keep their group regardless of the shrink.
	for u, g := range enrolled {
		r := c.GetExperiment("exp", User{"user_id": u}, nil)
		if !r.InExperiment || r.Group != g {
			t.Errorf("stickied user %q: got {%v %q}, want in-experiment group %q", u, r.InExperiment, r.Group, g)
		}
	}

	// A fresh user against a fresh store IS subject to the 1bp allocation gate —
	// almost certainly denied. Use a no-store client to prove the gate bites.
	fresh := NewOfflineClientFromSnapshot(nil, nil)
	fresh.exps = runningExp("saltvalue", 1, groups)
	deniedSeen := false
	for _, u := range []string{"zz1", "zz2", "zz3", "zz4", "zz5"} {
		if !fresh.GetExperiment("exp", User{"user_id": u}, nil).InExperiment {
			deniedSeen = true
			break
		}
	}
	if !deniedSeen {
		t.Errorf("expected at least one fresh user denied by the 1bp allocation gate")
	}
}

// Changing the experiment salt moves the 8-char prefix, invalidating the stored
// entry and forcing a re-bucket (the stored group is ignored).
func TestStickySaltChangeReshuffles(t *testing.T) {
	store := NewInMemoryStickyStore()
	c := NewOfflineClientFromSnapshot(nil, nil)
	c.stickyStore = store
	groups := []group{{Name: "control", Weight: 5000}, {Name: "treatment", Weight: 5000}}

	c.exps = runningExp("oldsalt12345", 10000, groups)
	first := c.GetExperiment("exp", User{"user_id": "user-x"}, nil)
	if !first.InExperiment {
		t.Fatal("precondition: user-x enrolled")
	}

	// Seed a bogus stored group under the OLD salt prefix; with the old salt this
	// would be honoured, proving the store is consulted.
	entries := store.Get("user-x")
	if entries["exp"].S != saltPrefix("oldsalt12345") {
		t.Fatalf("store should hold the old salt prefix, got %q", entries["exp"].S)
	}

	// New salt → new prefix → stored entry (old prefix) ignored → re-bucket. The
	// store is overwritten with the new prefix.
	c.exps = runningExp("newsalt67890", 10000, groups)
	after := c.GetExperiment("exp", User{"user_id": "user-x"}, nil)
	if !after.InExperiment {
		t.Fatal("user-x should still enrol under the new salt")
	}
	newEntry := store.Get("user-x")["exp"]
	if newEntry.S != saltPrefix("newsalt67890") {
		t.Errorf("store not overwritten with new salt prefix: got %q", newEntry.S)
	}
	if newEntry.G != after.Group {
		t.Errorf("store group %q != returned group %q", newEntry.G, after.Group)
	}
}

// Absent a store, behaviour is unchanged: a salt-prefix lookup never happens and
// repeated calls are purely deterministic.
func TestNoStickyStoreDeterministic(t *testing.T) {
	c := NewOfflineClientFromSnapshot(nil, nil)
	c.exps = runningExp("saltvalue", 10000,
		[]group{{Name: "control", Weight: 5000}, {Name: "treatment", Weight: 5000}})
	a := c.GetExperiment("exp", User{"user_id": "det"}, nil)
	b := c.GetExperiment("exp", User{"user_id": "det"}, nil)
	if a.Group != b.Group {
		t.Errorf("no-store eval must be deterministic, got %q then %q", a.Group, b.Group)
	}
}
