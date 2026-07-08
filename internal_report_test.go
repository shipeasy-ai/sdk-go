package shipeasy

// Self-monitoring channel: when the SDK swallows an internal ("on our end")
// error via recoverRead, it also ships a structured see event to Shipeasy's OWN
// project — a baked-in destination + public client key, distinct from the
// consumer's See() path. These tests pin the wire shape, the enable gating, the
// dedup, and the never-panics guarantee.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

// A real-looking client key to exercise the send path (the baked default is an
// inert placeholder until the real key is minted).
const fakeInternalKey = "sdk_client_testfakekey00000000000000000000"

// internalCaptureRT records every outbound request (URL + header + body) and
// replies 202.
type internalCaptureRT struct {
	mu    sync.Mutex
	calls []internalCapturedCall
}

type internalCapturedCall struct {
	url    string
	key    string
	events []seeEvent
}

func (r *internalCaptureRT) RoundTrip(req *http.Request) (*http.Response, error) {
	var events []seeEvent
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		req.Body.Close()
		var body struct {
			Events []seeEvent `json:"events"`
		}
		_ = json.Unmarshal(b, &body)
		events = body.Events
	}
	r.mu.Lock()
	r.calls = append(r.calls, internalCapturedCall{
		url:    req.URL.String(),
		key:    req.Header.Get("X-SDK-Key"),
		events: events,
	})
	r.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}, nil
}

// wait for the fire-and-forget goroutine(s) to land, then snapshot the calls.
func (r *internalCaptureRT) settled(t *testing.T, want int) []internalCapturedCall {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		r.mu.Lock()
		n := len(r.calls)
		r.mu.Unlock()
		if n >= want || time.Now().After(deadline) {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	// small settle to catch an unexpected extra send
	time.Sleep(20 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]internalCapturedCall, len(r.calls))
	copy(out, r.calls)
	return out
}

// stubInternalReport resets the channel, stands in the fake key, and captures
// sends. Returns the recorder.
func stubInternalReport(t *testing.T) *internalCaptureRT {
	t.Helper()
	resetInternalReportForTest()
	setInternalIngestKeyForTest(fakeInternalKey)
	rt := &internalCaptureRT{}
	setInternalReportHTTPForTest(&http.Client{Transport: rt})
	t.Cleanup(resetInternalReportForTest)
	return rt
}

func TestReportInternalError_DestinationAndKey(t *testing.T) {
	rt := stubInternalReport(t)
	setInternalReportContext("9.9.9", true)

	reportInternalError("GetFlag", &customErr{"cannot read foo"})

	calls := rt.settled(t, 1)
	if len(calls) != 1 {
		t.Fatalf("want 1 send, got %d", len(calls))
	}
	if calls[0].url != internalIngestURL {
		t.Fatalf("url = %q, want %q", calls[0].url, internalIngestURL)
	}
	if calls[0].key != fakeInternalKey {
		t.Fatalf("X-SDK-Key = %q, want %q", calls[0].key, fakeInternalKey)
	}
}

func TestReportInternalError_StableConsequenceAndSDKExtra(t *testing.T) {
	rt := stubInternalReport(t)
	setInternalReportContext("9.9.9", true)

	reportInternalError("Client.GetExperiment", &customErr{"boom"})

	calls := rt.settled(t, 1)
	if len(calls) != 1 || len(calls[0].events) != 1 {
		t.Fatalf("want 1 event, got calls=%d", len(calls))
	}
	ev := calls[0].events[0]
	if ev.Type != "error" {
		t.Fatalf("type = %q", ev.Type)
	}
	if ev.Kind != "caught" {
		t.Fatalf("kind = %q, want caught", ev.Kind)
	}
	if ev.Subject != "Client.GetExperiment" {
		t.Fatalf("subject = %q", ev.Subject)
	}
	if ev.Outcome != "returned a safe default" {
		t.Fatalf("outcome = %q", ev.Outcome)
	}
	if ev.Message != "boom" {
		t.Fatalf("message = %q", ev.Message)
	}
	if ev.Side != "server" {
		t.Fatalf("side = %q, want server", ev.Side)
	}
	if ev.SDKVersion != "9.9.9" {
		t.Fatalf("sdk_version = %q, want 9.9.9", ev.SDKVersion)
	}
	if ev.Extras == nil || ev.Extras["sdk"] != "go" {
		t.Fatalf("extras.sdk = %v, want go", ev.Extras)
	}
}

func TestReportInternalError_NoConsumerEnv(t *testing.T) {
	rt := stubInternalReport(t)
	setInternalReportContext("9.9.9", true)

	reportInternalError("GetKillswitch", &customErr{"x"})

	calls := rt.settled(t, 1)
	if len(calls) != 1 {
		t.Fatalf("want 1 send, got %d", len(calls))
	}
	if ev := calls[0].events[0]; ev.Env != "" {
		t.Fatalf("env = %q, want empty (only SDK-side context)", ev.Env)
	}
}

func TestReportInternalError_NoopBeforeContext(t *testing.T) {
	rt := stubInternalReport(t)
	// no setInternalReportContext call
	reportInternalError("GetFlag", &customErr{"boom"})
	if calls := rt.settled(t, 0); len(calls) != 0 {
		t.Fatalf("want 0 sends before context set, got %d", len(calls))
	}
}

func TestReportInternalError_NoopWhenDisabled(t *testing.T) {
	rt := stubInternalReport(t)
	setInternalReportContext("9.9.9", false)
	reportInternalError("GetFlag", &customErr{"boom"})
	if calls := rt.settled(t, 0); len(calls) != 0 {
		t.Fatalf("want 0 sends when disabled, got %d", len(calls))
	}
}

func TestReportInternalError_InertOnPlaceholder(t *testing.T) {
	rt := stubInternalReport(t)
	setInternalIngestKeyForTest(internalPlaceholderKey)
	setInternalReportContext("9.9.9", true)
	reportInternalError("GetFlag", &customErr{"boom"})
	if calls := rt.settled(t, 0); len(calls) != 0 {
		t.Fatalf("want 0 sends while key is the placeholder, got %d", len(calls))
	}
}

func TestReportInternalError_Dedup(t *testing.T) {
	rt := stubInternalReport(t)
	setInternalReportContext("9.9.9", true)

	// Identical error type + message => same fingerprint within the 30s window
	// (mirrors a hot loop re-throwing from the same site).
	reportInternalError("GetFlag", &customErr{"same"})
	reportInternalError("GetFlag", &customErr{"same"})

	if calls := rt.settled(t, 1); len(calls) != 1 {
		t.Fatalf("want 1 send after dedup, got %d", len(calls))
	}
}

func TestReportInternalError_NeverPanics(t *testing.T) {
	resetInternalReportForTest()
	setInternalIngestKeyForTest(fakeInternalKey)
	// A transport that panics inside Do — the fire-and-forget goroutine must
	// swallow it and the caller must not panic.
	setInternalReportHTTPForTest(&http.Client{Transport: panicRT{}})
	t.Cleanup(resetInternalReportForTest)
	setInternalReportContext("9.9.9", true)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("reportInternalError panicked into caller: %v", r)
		}
	}()
	reportInternalError("GetFlag", &customErr{"boom"})
	// give the goroutine a chance to run its (recovered) panic
	time.Sleep(20 * time.Millisecond)
}

// TestRecoverReadReportsInternalError verifies the guard integration: a panic
// recovered by recoverRead is self-reported AND the documented safe default is
// still returned to the caller.
func TestRecoverReadReportsInternalError(t *testing.T) {
	rt := stubInternalReport(t)
	setInternalReportContext("9.9.9", true)

	c, user := runningExpEngine(t, panicStore{}, LogLevelSilent)

	got := c.GetExperiment("exp1", user, map[string]any{"fallback": true})
	// safe default still returned
	if got.InExperiment {
		t.Fatalf("expected safe default InExperiment=false, got %+v", got)
	}

	calls := rt.settled(t, 1)
	if len(calls) != 1 {
		t.Fatalf("want 1 internal report from recoverRead, got %d", len(calls))
	}
	if ev := calls[0].events[0]; ev.Subject != "GetExperiment" {
		t.Fatalf("subject = %q, want GetExperiment", ev.Subject)
	}
}

func TestRecoverReadNoReportOnSuccess(t *testing.T) {
	rt := stubInternalReport(t)
	setInternalReportContext("9.9.9", true)

	// A healthy engine with a loaded flag: GetFlag succeeds, recoverRead's body
	// never fires, so nothing is reported.
	c := NewTestClient()
	c.OverrideFlag("f", true)
	if !c.GetFlag("f", User{"user_id": "u"}) {
		t.Fatalf("expected overridden flag true")
	}
	if calls := rt.settled(t, 0); len(calls) != 0 {
		t.Fatalf("want 0 sends on a successful read, got %d", len(calls))
	}
}

// customErr is a minimal error whose Error() is the message.
type customErr struct{ msg string }

func (e *customErr) Error() string { return e.msg }

// panicRT is an http transport whose RoundTrip panics, to exercise the
// never-panics guarantee of the fire-and-forget send.
type panicRT struct{}

func (panicRT) RoundTrip(*http.Request) (*http.Response, error) {
	panic("adversarial transport: boom")
}
