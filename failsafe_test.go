package shipeasy

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// panicStore is an adversarial StickyBucketStore whose Get panics. It is wired
// into a running experiment so Universe().Assign()'s read path reaches it,
// exercising the defensive recover in the runtime read methods.
type panicStore struct{}

func (panicStore) Get(unit string) map[string]StickyEntry {
	panic("adversarial sticky store: boom")
}
func (panicStore) Set(unit, experiment string, entry StickyEntry) {}

// runningExpEngine builds a local-mode engine with a single running experiment
// loaded, so evalExperiment reaches the (panicking) sticky store.
func runningExpEngine(t *testing.T, store StickyBucketStore, logLevel string) (*Engine, User) {
	t.Helper()
	c := NewEngine(Options{LogLevel: logLevel})
	c.localMode = true
	c.initialized = true
	c.stickyStore = store
	c.exps = &expsBlob{
		Universes: map[string]universe{"u": {}},
		Experiments: map[string]experiment{
			"exp1": {
				Status:        "running",
				Universe:      "u",
				Salt:          "s",
				AllocationPct: 10000,
				Groups:        []group{{Name: "control", Weight: 100}},
			},
		},
	}
	return c, User{"user_id": "u1"}
}

// TestRuntimeReadNeverPanics asserts that an unexpected panic inside a runtime
// read (here, a panicking sticky store reached from Universe().Assign()) is
// caught and the documented safe default is returned instead of unwinding.
func TestRuntimeReadNeverPanics(t *testing.T) {
	c, user := runningExpEngine(t, panicStore{}, LogLevelSilent)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Assign panicked into caller: %v", r)
		}
	}()

	got := c.Universe("u").Assign(user)
	if got.Enrolled {
		t.Fatalf("expected safe default Enrolled=false, got %+v", got)
	}
	if got.Group != "" {
		t.Fatalf("expected safe default Group=\"\", got %q", got.Group)
	}
	if got.Get("fallback", true) != true {
		t.Fatalf("expected fallback echoed back, got %+v", got.Get("fallback", true))
	}

	// The bound Client form must be equally safe (built directly against an
	// engine so we don't perturb the package-global Configure state).
	c2, user2 := runningExpEngine(t, panicStore{}, LogLevelSilent)
	cl := &Client{attributes: user2, engine: c2}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Client.Universe.Assign panicked into caller: %v", r)
			}
		}()
		if a := cl.Universe("u").Assign(); a.Enrolled || a.Group != "" {
			t.Fatalf("Client.Universe.Assign safe default wrong: %+v", a)
		}
	}()
}

// TestLogLevelGating asserts LogLevel:"silent" suppresses SDK logs while the
// default "warn" emits them. It swaps the standard logger's output to a buffer
// and drives the recover path (which logs at "error").
func TestLogLevelGating(t *testing.T) {
	var buf bytes.Buffer
	origOut := log.Writer()
	origFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(origOut)
		log.SetFlags(origFlags)
	})

	// silent: the recover's error-level log is suppressed.
	buf.Reset()
	cSilent, user := runningExpEngine(t, panicStore{}, LogLevelSilent)
	_ = cSilent.Universe("u").Assign(user)
	if got := buf.String(); got != "" {
		t.Fatalf("LogLevel silent should suppress all output, got: %q", got)
	}

	// default (empty → "warn"): warn >= error, so the recover log IS emitted.
	buf.Reset()
	cDefault, user2 := runningExpEngine(t, panicStore{}, "")
	if cDefault.logLevel != LogLevelWarn {
		t.Fatalf("empty LogLevel should resolve to warn, got %q", cDefault.logLevel)
	}
	_ = cDefault.Universe("u").Assign(user2)
	if got := buf.String(); !strings.Contains(got, "[shipeasy]") || !strings.Contains(got, "Assign panicked") {
		t.Fatalf("default (warn) LogLevel should emit the error-level recover log, got: %q", got)
	}
}

// TestLogLevelResolution covers the normalization rules directly.
func TestLogLevelResolution(t *testing.T) {
	cases := map[string]string{
		"":         LogLevelWarn,
		"nonsense": LogLevelWarn,
		"silent":   LogLevelSilent,
		"error":    LogLevelError,
		"warn":     LogLevelWarn,
		"info":     LogLevelInfo,
		"debug":    LogLevelDebug,
	}
	for in, want := range cases {
		if got := resolveLogLevel(in); got != want {
			t.Fatalf("resolveLogLevel(%q) = %q, want %q", in, got, want)
		}
	}
}
