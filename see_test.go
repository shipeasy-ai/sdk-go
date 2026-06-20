package shipeasy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

// captureRT records every POST body it sees and replies 200. Used to assert the
// /collect payload see() produces (mirrors the Python monkeypatched _post_silent).
type captureRT struct {
	mu   sync.Mutex
	sent [][]byte
}

func (r *captureRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		req.Body.Close()
		r.mu.Lock()
		r.sent = append(r.sent, b)
		r.mu.Unlock()
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}, nil
}

// events parses every captured body into the flattened list of error events.
func (r *captureRT) events(t *testing.T) []seeEvent {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []seeEvent
	for _, b := range r.sent {
		var body struct {
			Events []seeEvent `json:"events"`
		}
		if err := json.Unmarshal(b, &body); err != nil {
			t.Fatalf("bad capture body %q: %v", b, err)
		}
		out = append(out, body.Events...)
	}
	return out
}

func (r *captureRT) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sent)
}

// newCaptureClient builds a real (non-test-mode) client whose /collect POSTs are
// captured, and waits for the fire-and-forget goroutine to land before reading.
func newCaptureClient(opts Options) (*Client, *captureRT) {
	rt := &captureRT{}
	opts.HTTP = &http.Client{Transport: rt}
	opts.DisableTelemetry = true
	if opts.BaseURL == "" {
		opts.BaseURL = "https://e.x"
	}
	if opts.APIKey == "" {
		opts.APIKey = "srv_key"
	}
	return NewClient(opts), rt
}

// waitFor polls until cond is true or the deadline passes (the send is async).
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !cond() {
		t.Fatalf("condition not met before deadline")
	}
}

func TestSeeCaughtErrorReportsErrorEvent(t *testing.T) {
	c, rt := newCaptureClient(Options{})
	err := errors.New("boom")
	c.See(err).CausesThe("checkout").To("use cached prices")
	waitFor(t, func() bool { return rt.count() == 1 })

	ev := rt.events(t)[0]
	if ev.Type != "error" {
		t.Errorf("type = %q, want error", ev.Type)
	}
	if ev.Kind != "caught" {
		t.Errorf("kind = %q, want caught", ev.Kind)
	}
	if ev.ErrorType != "errors.errorString" {
		t.Errorf("error_type = %q, want errors.errorString", ev.ErrorType)
	}
	if ev.Message != "boom" {
		t.Errorf("message = %q, want boom", ev.Message)
	}
	if ev.Subject != "checkout" {
		t.Errorf("subject = %q, want checkout", ev.Subject)
	}
	if ev.Outcome != "use cached prices" {
		t.Errorf("outcome = %q, want use cached prices", ev.Outcome)
	}
	if ev.Side != "server" {
		t.Errorf("side = %q, want server", ev.Side)
	}
	if ev.SDKVersion != SDKVersion {
		t.Errorf("sdk_version = %q, want %q", ev.SDKVersion, SDKVersion)
	}
	if ev.Env != "prod" {
		t.Errorf("env = %q, want prod", ev.Env)
	}
	if ev.Stack == "" {
		t.Errorf("expected a non-empty stack on a caught error")
	}
	if ev.TS == 0 {
		t.Errorf("expected a non-zero ts")
	}
}

func TestSeeExtrasBeforeToAreSanitizedAndSent(t *testing.T) {
	c, rt := newCaptureClient(Options{})
	c.See(errors.New("x")).CausesThe("photo upload").Extras(map[string]any{
		"photo_id": "p1",
		"size":     42,
		"ok":       true,
		"skip":     nil,
	}).To("be rejected")
	waitFor(t, func() bool { return rt.count() == 1 })

	ev := rt.events(t)[0]
	if _, ok := ev.Extras["skip"]; ok {
		t.Errorf("nil-valued key should be dropped: %v", ev.Extras)
	}
	if ev.Extras["photo_id"] != "p1" {
		t.Errorf("photo_id = %v, want p1", ev.Extras["photo_id"])
	}
	// JSON round-trips numbers as float64.
	if ev.Extras["size"] != float64(42) {
		t.Errorf("size = %v, want 42", ev.Extras["size"])
	}
	if ev.Extras["ok"] != true {
		t.Errorf("ok = %v, want true", ev.Extras["ok"])
	}
}

func TestSeeViolationUsesViolationKind(t *testing.T) {
	c, rt := newCaptureClient(Options{})
	c.SeeViolation("large query").CausesThe("search results").To("be trimmed")
	waitFor(t, func() bool { return rt.count() == 1 })

	ev := rt.events(t)[0]
	if ev.Kind != "violation" {
		t.Errorf("kind = %q, want violation", ev.Kind)
	}
	if ev.ErrorType != "large query" {
		t.Errorf("error_type = %q, want large query", ev.ErrorType)
	}
	if ev.Message != "large query" {
		t.Errorf("message = %q, want large query", ev.Message)
	}
	if ev.Subject != "search results" {
		t.Errorf("subject = %q, want search results", ev.Subject)
	}
	if ev.Stack != "" {
		t.Errorf("violation must not carry a stack, got %q", ev.Stack)
	}
}

func TestControlFlowMarksAndReportsNothing(t *testing.T) {
	c, rt := newCaptureClient(Options{})
	e := errors.New("not a Foo")
	c.ControlFlowException(e).Because("because it wasn't an encoded Foo").Extras(map[string]any{"tried": "Foo"})
	if !IsExpected(e) {
		t.Errorf("error should be marked expected")
	}
	// Give any (erroneous) async send a moment to land; expect none.
	time.Sleep(20 * time.Millisecond)
	if rt.count() != 0 {
		t.Errorf("control flow must report nothing, sent %d", rt.count())
	}
}

func TestControlFlowGlobalNoClientNoPanic(t *testing.T) {
	e := errors.New("expected")
	ControlFlowException(e).Because("because reasons")
	if !IsExpected(e) {
		t.Errorf("global control flow should still mark the error")
	}
}

func TestSeeToIsRequiredNoSendWithoutTerminal(t *testing.T) {
	c, rt := newCaptureClient(Options{})
	c.See(errors.New("x")).CausesThe("checkout") // no .To()
	time.Sleep(20 * time.Millisecond)
	if rt.count() != 0 {
		t.Errorf("no .To() must send nothing, sent %d", rt.count())
	}
}

func TestSeeToIsIdempotent(t *testing.T) {
	c, rt := newCaptureClient(Options{})
	chain := c.See(errors.New("x")).CausesThe("checkout")
	chain.To("a")
	chain.To("b")
	waitFor(t, func() bool { return rt.count() == 1 })
	time.Sleep(20 * time.Millisecond)
	if got := len(rt.events(t)); got != 1 {
		t.Errorf("to() called twice should send once, got %d", got)
	}
}

func TestSeeDefaultsWhenConsequenceOmitted(t *testing.T) {
	c, rt := newCaptureClient(Options{})
	c.See(errors.New("x")).To("be incomplete")
	waitFor(t, func() bool { return rt.count() == 1 })
	ev := rt.events(t)[0]
	if ev.Subject != seeDefaultSubject {
		t.Errorf("subject = %q, want %q", ev.Subject, seeDefaultSubject)
	}
}

func TestSeeTestModeIsNoop(t *testing.T) {
	c := NewTestClient()
	// A test client has no real transport; a no-op send must not even try.
	c.See(errors.New("x")).CausesThe("checkout").To("use cached prices")
	time.Sleep(20 * time.Millisecond)
	// Nothing to assert beyond "did not panic / did not block"; localMode guard
	// returns before any goroutine or network is created.
}

func TestGlobalSeeUsesLastConstructedClient(t *testing.T) {
	c, rt := newCaptureClient(Options{})
	_ = c // NewClient already registered it as the default.
	See(errors.New("global")).CausesThe("dashboard").To("show cached data")
	waitFor(t, func() bool { return rt.count() == 1 })
	ev := rt.events(t)[0]
	if ev.Subject != "dashboard" {
		t.Errorf("subject = %q, want dashboard", ev.Subject)
	}
}

func TestGlobalSeeBeforeClientWarnsAndDrops(t *testing.T) {
	SetDefaultClient(nil)
	defer SetDefaultClient(nil)
	// Must not panic and must send nothing (no client, no transport).
	See(errors.New("x")).CausesThe("checkout").To("use cached prices")
}

func TestSanitizeExtrasCapsKeysAndValueLength(t *testing.T) {
	big := map[string]any{}
	for i := 0; i < 30; i++ {
		big["k"+string(rune('a'+i%26))+string(rune('0'+i/26))] = i
	}
	longVal := make([]byte, 500)
	for i := range longVal {
		longVal[i] = 'x'
	}
	big["long"] = string(longVal)

	out := sanitizeExtras(big)
	if len(out) > seeMaxExtraKeys {
		t.Errorf("len = %d, want <= %d", len(out), seeMaxExtraKeys)
	}
	if v, ok := out["long"].(string); ok && len(v) > seeMaxExtraValue {
		t.Errorf("long value not truncated: %d", len(v))
	}
}

func TestSeePrivateAttributesStrippedFromExtras(t *testing.T) {
	c, rt := newCaptureClient(Options{PrivateAttributes: []string{"secret"}})
	c.See(errors.New("x")).CausesThe("checkout").Extras(map[string]any{
		"secret": "shh",
		"ok":     "yes",
	}).To("use cached prices")
	waitFor(t, func() bool { return rt.count() == 1 })

	ev := rt.events(t)[0]
	if _, ok := ev.Extras["secret"]; ok {
		t.Errorf("private attr must be stripped: %v", ev.Extras)
	}
	if ev.Extras["ok"] != "yes" {
		t.Errorf("ok = %v, want yes", ev.Extras["ok"])
	}
}
