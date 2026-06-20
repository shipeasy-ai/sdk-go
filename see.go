package shipeasy

// see — shipeasy error. Structured error reporting for the server SDK.
//
// Mirrors @shipeasy/sdk (packages/ts-sdk/src/see/core.ts) and the Python
// reference (packages/server-sdks/sdk-python/shipeasy/_see.py). Every handled
// error documents its product *consequence*, not just its stack:
//
//	if err := chargeCard(order); err != nil {
//	    shipeasy.See(err).CausesThe("checkout").Extras(map[string]any{
//	        "order_id": order.ID,
//	    }).To("use the backup processor")
//	}
//
// Dispatch model (differs from TS, which uses a microtask): .To(outcome) is the
// terminal — it builds the wire event and fire-and-forgets the POST to
// /collect. CausesThe() and Extras() are chainable setters that may be called
// in any order BEFORE .To(). If a chain never calls .To(), nothing is sent.
//
// If you don't know the consequence of an error, don't handle it here.

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// Limits (mirror core.ts; kept in sync with the worker's /collect).
const (
	seeMaxMessage     = 500
	seeMaxStack       = 8000
	seeMaxSubject     = 200 // subject, outcome, error_type
	seeMaxExtraValue  = 200
	seeMaxExtraKeys   = 20
	seeDedupWindowMs  = 30_000
	seeMaxPerProcess  = 25
	seeDefaultSubject = "app"
	seeDefaultOutcome = "hit an error"
)

func seeTruncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}

// Violation is a non-exception problem. The Name is a stable fingerprint key —
// put variable data in Extras(), never in the name.
type Violation struct {
	Name string
}

// SeeViolation constructs a Violation value. Pass it to See(), or use the
// SeeViolation chain entrypoints, which build it for you.
func NewViolation(name string) Violation { return Violation{Name: name} }

// sanitizeExtras drops nil values, keeps only string/finite-number/bool,
// truncates string values to 200 chars, caps at 20 keys, and returns nil if
// nothing is kept. Map iteration order in Go is random, so the 20-key cap is
// non-deterministic about WHICH keys survive — acceptable, matching the spec's
// "cap at 20" intent.
func sanitizeExtras(extras map[string]any) map[string]any {
	if len(extras) == 0 {
		return nil
	}
	out := make(map[string]any)
	n := 0
	for k, v := range extras {
		if v == nil {
			continue
		}
		if n >= seeMaxExtraKeys {
			break
		}
		switch val := v.(type) {
		case bool:
			out[k] = val
		case string:
			out[k] = seeTruncate(val, seeMaxExtraValue)
		case int:
			out[k] = val
		case int8:
			out[k] = val
		case int16:
			out[k] = val
		case int32:
			out[k] = val
		case int64:
			out[k] = val
		case uint:
			out[k] = val
		case uint8:
			out[k] = val
		case uint16:
			out[k] = val
		case uint32:
			out[k] = val
		case uint64:
			out[k] = val
		case float32:
			f := float64(val)
			if math.IsInf(f, 0) || math.IsNaN(f) {
				continue
			}
			out[k] = val
		case float64:
			if math.IsInf(val, 0) || math.IsNaN(val) {
				continue
			}
			out[k] = val
		default:
			continue // drop anything else (slices, maps, structs, ...)
		}
		n++
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// seeEvent is the type:"error" event accepted by POST /collect. JSON keys match
// the TS/Python wire shape exactly.
type seeEvent struct {
	Type       string         `json:"type"`
	Kind       string         `json:"kind"`
	ErrorType  string         `json:"error_type"`
	Message    string         `json:"message"`
	Stack      string         `json:"stack,omitempty"`
	Subject    string         `json:"subject"`
	Outcome    string         `json:"outcome"`
	Extras     map[string]any `json:"extras,omitempty"`
	Side       string         `json:"side"`
	Env        string         `json:"env,omitempty"`
	SDKVersion string         `json:"sdk_version"`
	TS         int64          `json:"ts"`
}

// buildSeeEvent constructs the wire event from a finalized chain. For a Go error
// the error_type is the concrete type name (via %T) and a stack is captured via
// runtime/debug.Stack(). For a Violation, kind="violation" and no stack.
func buildSeeEvent(problem any, subject, outcome string, extras map[string]any, env string) seeEvent {
	var errorType, message, stack, kind string

	switch p := problem.(type) {
	case Violation:
		errorType = p.Name
		message = p.Name
		kind = "violation"
	case *Violation:
		errorType = p.Name
		message = p.Name
		kind = "violation"
	case error:
		errorType = strings.TrimPrefix(fmt.Sprintf("%T", p), "*")
		if errorType == "" {
			errorType = "error"
		}
		message = p.Error()
		if message == "" {
			message = errorType
		}
		stack = strings.TrimSpace(string(debug.Stack()))
		kind = "caught"
	case string:
		errorType = "error"
		message = p
		kind = "caught"
	default:
		errorType = "error"
		message = fmt.Sprintf("%v", p)
		kind = "caught"
	}

	ev := seeEvent{
		Type:       "error",
		Kind:       kind,
		ErrorType:  seeTruncate(errorType, seeMaxSubject),
		Message:    seeTruncate(message, seeMaxMessage),
		Subject:    seeTruncate(subject, seeMaxSubject),
		Outcome:    seeTruncate(outcome, seeMaxSubject),
		Side:       "server",
		Env:        env,
		SDKVersion: SDKVersion,
		TS:         time.Now().UnixMilli(),
	}
	if stack != "" {
		ev.Stack = seeTruncate(stack, seeMaxStack)
	}
	if clean := sanitizeExtras(extras); clean != nil {
		ev.Extras = clean
	}
	return ev
}

// ---- Spam limiter (mirror SeeLimiter) ----

func seeTopStackLine(stack string) string {
	if stack == "" {
		return ""
	}
	for _, line := range strings.Split(stack, "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "at ") || strings.Contains(s, ".go:") || strings.Contains(s, "line ") {
			if len(s) > 200 {
				return s[:200]
			}
			return s
		}
	}
	return ""
}

// seeLimiter is a per-process spam guard: identical events within a 30s window
// collapse to one send, and a hard cap bounds total sends. Thread-safe.
type seeLimiter struct {
	mu     sync.Mutex
	max    int
	window int64
	last   map[string]int64
	sent   int
}

func newSeeLimiter() *seeLimiter {
	return &seeLimiter{
		max:    seeMaxPerProcess,
		window: seeDedupWindowMs,
		last:   map[string]int64{},
	}
}

func (l *seeLimiter) shouldSend(ev seeEvent) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sent >= l.max {
		return false
	}
	msg := ev.Message
	if len(msg) > 200 {
		msg = msg[:200]
	}
	key := strings.Join([]string{ev.Kind, ev.ErrorType, msg, seeTopStackLine(ev.Stack)}, "|")
	now := time.Now().UnixMilli()
	if prev, ok := l.last[key]; ok && now-prev < l.window {
		return false
	}
	l.last[key] = now
	l.sent++
	return true
}

// ---- Fluent chains ----

// SeeChain accumulates the consequence (subject) + extras for a problem. The
// terminal .To(outcome) builds the event and fire-and-forgets the report. It is
// idempotent: a second .To() is a no-op.
type SeeChain struct {
	client  *Client
	problem any
	subject string
	outcome string
	extras  map[string]any
	done    bool
}

// CausesThe sets the consequence subject (e.g. "checkout").
func (s *SeeChain) CausesThe(subject string) *SeeChain {
	if s == nil {
		return s
	}
	s.subject = subject
	return s
}

// Extras merges sanitizable key→value context. Repeat calls merge (later wins).
func (s *SeeChain) Extras(extras map[string]any) *SeeChain {
	if s == nil || len(extras) == 0 {
		return s
	}
	if s.extras == nil {
		s.extras = make(map[string]any, len(extras))
	}
	for k, v := range extras {
		s.extras[k] = v
	}
	return s
}

// To is the terminal: it sets the outcome, builds the event, and fire-and-forgets
// the report. Idempotent. Reporting never blocks or panics into caller code.
func (s *SeeChain) To(outcome string) {
	if s == nil || s.done {
		return
	}
	s.done = true
	s.outcome = outcome
	if s.client == nil {
		return // no-op chain (global See before any client)
	}
	s.client.dispatchSee(s)
}

// ControlFlowChain marks an error as expected control flow and reports NOTHING.
// .Because(reason) returns a tail with an optional .Extras() for local debugging.
type ControlFlowChain struct {
	err error
}

// Because marks the error expected (best-effort) and returns a tail. Reports
// nothing. The reason conventionally starts with "because" — not enforced.
func (c *ControlFlowChain) Because(reason string) *ControlFlowTail {
	if c == nil {
		return &ControlFlowTail{}
	}
	markExpected(c.err, reason, nil)
	return &ControlFlowTail{err: c.err, reason: reason}
}

// ControlFlowTail carries the marked error so .Extras() can attach local-only
// debug context. Nothing here is ever transmitted.
type ControlFlowTail struct {
	err    error
	reason string
}

// Extras stores additional local-only debug context on the expected-error mark.
// Never sent over the network.
func (t *ControlFlowTail) Extras(extras map[string]any) *ControlFlowTail {
	if t == nil || t.err == nil {
		return t
	}
	markExpected(t.err, t.reason, extras)
	return t
}

// ---- Expected-error marks (control flow) ----
//
// Go has no way to stamp metadata onto an arbitrary error value (Python uses
// setattr). We keep a package-level map keyed by the error pointer instead. It
// is best-effort and bounded only by process lifetime; control_flow is meant
// for narrow, deliberate uses, so unbounded growth is not a practical concern.

type expectedMark struct {
	Because string
	Extras  map[string]any
}

var expectedMarks sync.Map // map[error]expectedMark

func markExpected(err error, because string, extras map[string]any) {
	if err == nil {
		return
	}
	mark := expectedMark{Because: because}
	if clean := sanitizeExtras(extras); clean != nil {
		mark.Extras = clean
	}
	expectedMarks.Store(err, mark)
}

// IsExpected reports whether err was marked as expected control flow via
// ControlFlowException(err).Because(...). Useful for tests and local assertions.
func IsExpected(err error) bool {
	if err == nil {
		return false
	}
	_, ok := expectedMarks.Load(err)
	return ok
}

// ---- Instance API ----

// See reports a handled error (or a thrown non-error problem) via this client.
// Fire-and-forget; never blocks or panics into the request path. Terminate with
// .To(outcome):
//
//	client.See(err).CausesThe("checkout").To("use cached prices")
func (c *Client) See(problem any) *SeeChain {
	return &SeeChain{client: c, problem: problem}
}

// SeeViolation reports a non-exception problem. The name is a stable fingerprint
// key — put variable data in .Extras(), never the name.
func (c *Client) SeeViolation(name string) *SeeChain {
	return &SeeChain{client: c, problem: Violation{Name: name}}
}

// ControlFlowException marks an error as expected control flow — reports nothing.
func (c *Client) ControlFlowException(err error) *ControlFlowChain {
	return &ControlFlowChain{err: err}
}

// dispatchSee builds the wire event and fire-and-forgets the POST to /collect.
// No-op in localMode. Spam-guarded. Never panics into caller code.
func (c *Client) dispatchSee(s *SeeChain) {
	if c.localMode {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[shipeasy] see() send failed: %v", r)
		}
	}()
	subject := s.subject
	if subject == "" {
		subject = seeDefaultSubject
	}
	outcome := s.outcome
	if outcome == "" {
		outcome = seeDefaultOutcome
	}
	ev := buildSeeEvent(s.problem, subject, outcome, c.stripPrivate(s.extras), c.env)
	if c.seeLimiter != nil && !c.seeLimiter.shouldSend(ev) {
		return
	}
	body, err := json.Marshal(map[string]any{"events": []seeEvent{ev}})
	if err != nil {
		log.Printf("[shipeasy] see() marshal failed: %v", err)
		return
	}
	go func() {
		if err := c.post("/collect", body); err != nil {
			log.Printf("[shipeasy] see() failed: %v", err)
		}
	}()
}

// ---- Package-level default client + global API ----

var (
	defaultClient   *Client
	defaultClientMu sync.RWMutex
)

// SetDefaultClient registers the client backing the package-level See(),
// SeeViolation(), and ControlFlowException() functions. NewClient calls this
// automatically (last constructed wins).
func SetDefaultClient(c *Client) {
	defaultClientMu.Lock()
	defaultClient = c
	defaultClientMu.Unlock()
}

func resolveDefaultClient() *Client {
	defaultClientMu.RLock()
	defer defaultClientMu.RUnlock()
	return defaultClient
}

// See reports a handled error via the default client (the last one constructed).
// Before any client exists it logs a warning and returns a no-op chain — it
// never panics.
func See(problem any) *SeeChain {
	c := resolveDefaultClient()
	if c == nil {
		log.Printf("[shipeasy] see() called before a client was created — error dropped")
		return &SeeChain{problem: problem}
	}
	return c.See(problem)
}

// SeeViolation reports a non-exception problem via the default client.
func SeeViolation(name string) *SeeChain {
	c := resolveDefaultClient()
	if c == nil {
		log.Printf("[shipeasy] see() called before a client was created — error dropped")
		return &SeeChain{problem: Violation{Name: name}}
	}
	return c.SeeViolation(name)
}

// ControlFlowException marks an error as expected control flow (reports nothing).
// Works without a client — it only marks the error value.
func ControlFlowException(err error) *ControlFlowChain {
	return &ControlFlowChain{err: err}
}
