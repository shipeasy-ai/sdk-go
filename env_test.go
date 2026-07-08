package shipeasy

import (
	"strings"
	"testing"
	"time"
)

// boolPtr is a tiny helper for the *bool egress options.
func boolPtr(b bool) *bool { return &b }

// TestIsProductionEnvPrecedence exercises the native-env-var precedence and the
// fall-back to the SDK's own configured Env. t.Setenv restores the (production)
// value TestMain set once the test returns.
func TestIsProductionEnvPrecedence(t *testing.T) {
	// Clear every native var for the pure fall-back cases.
	clearNative := func(t *testing.T) {
		for _, name := range nativeEnvVars {
			t.Setenv(name, "")
		}
	}

	t.Run("SHIPEASY_ENV production wins", func(t *testing.T) {
		clearNative(t)
		t.Setenv("SHIPEASY_ENV", "production")
		if !isProductionEnv("dev") {
			t.Fatal("SHIPEASY_ENV=production should be production even with configuredEnv=dev")
		}
	})

	t.Run("SHIPEASY_ENV prod (short) wins", func(t *testing.T) {
		clearNative(t)
		t.Setenv("SHIPEASY_ENV", "PROD")
		if !isProductionEnv("dev") {
			t.Fatal("SHIPEASY_ENV=PROD (case-insensitive) should be production")
		}
	})

	t.Run("SHIPEASY_ENV non-prod value is not production", func(t *testing.T) {
		clearNative(t)
		t.Setenv("SHIPEASY_ENV", "staging")
		if isProductionEnv("prod") {
			t.Fatal("SHIPEASY_ENV=staging should NOT be production, even with configuredEnv=prod")
		}
	})

	t.Run("precedence order: SHIPEASY_ENV over APP_ENV/GO_ENV/ENV", func(t *testing.T) {
		clearNative(t)
		t.Setenv("SHIPEASY_ENV", "production")
		t.Setenv("APP_ENV", "development")
		t.Setenv("GO_ENV", "development")
		t.Setenv("ENV", "development")
		if !isProductionEnv("dev") {
			t.Fatal("SHIPEASY_ENV must win over APP_ENV/GO_ENV/ENV")
		}
	})

	t.Run("falls through to APP_ENV then GO_ENV then ENV", func(t *testing.T) {
		clearNative(t)
		t.Setenv("APP_ENV", "production")
		if !isProductionEnv("dev") {
			t.Fatal("APP_ENV=production should be production when SHIPEASY_ENV unset")
		}
		t.Setenv("APP_ENV", "")
		t.Setenv("GO_ENV", "production")
		if !isProductionEnv("dev") {
			t.Fatal("GO_ENV=production should be production when SHIPEASY_ENV/APP_ENV unset")
		}
		t.Setenv("GO_ENV", "")
		t.Setenv("ENV", "prod")
		if !isProductionEnv("dev") {
			t.Fatal("ENV=prod should be production when the higher-precedence vars are unset")
		}
	})

	t.Run("no native var: fall back to configured Env", func(t *testing.T) {
		clearNative(t)
		if !isProductionEnv("prod") {
			t.Fatal("no native var + configuredEnv=prod should be production")
		}
		if isProductionEnv("dev") {
			t.Fatal("no native var + configuredEnv=dev should NOT be production")
		}
	})

	t.Run("no native var and empty configured Env defaults to prod", func(t *testing.T) {
		clearNative(t)
		if !isProductionEnv("") {
			t.Fatal("no native var + empty configuredEnv should default to production")
		}
	})
}

// TestNetworkOffByDefaultInDev asserts (a) offline-by-default in dev: no request
// fires. With SHIPEASY_ENV=development and no explicit IsNetworkEnabled, the
// engine runs in localMode — Init/Track/telemetry are all no-ops.
func TestNetworkOffByDefaultInDev(t *testing.T) {
	t.Setenv("SHIPEASY_ENV", "development")
	hc, ch := recordingClient()
	c := NewEngine(Options{APIKey: "k", TelemetryURL: "https://e.x", HTTP: hc})

	if !c.localMode {
		t.Fatal("expected localMode (offline) by default in a non-production env")
	}
	// Reads still work from overrides / defaults; no beacon, no Track egress.
	c.OverrideFlag("g", true)
	if !c.GetFlag("g", User{}) {
		t.Fatal("override read should still work while offline")
	}
	c.Track("u1", "purchase", nil)

	if urls := collectURLs(ch, 1, 300*time.Millisecond); len(urls) != 0 {
		t.Fatalf("expected ZERO outbound requests in dev by default, got %v", urls)
	}
}

// TestExplicitNetworkOnOverridesDevDefault asserts (b) an explicit
// IsNetworkEnabled:true restores egress even in a non-production env. It uses the
// Track path (an /collect POST) rather than telemetry, because usage telemetry
// follows its own env-derived default (off in dev) independently of the master
// network switch.
func TestExplicitNetworkOnOverridesDevDefault(t *testing.T) {
	t.Setenv("SHIPEASY_ENV", "development")
	hc, ch := recordingClient()
	c := NewEngine(Options{
		APIKey:           "k",
		BaseURL:          "https://edge.x",
		HTTP:             hc,
		IsNetworkEnabled: boolPtr(true),
	})

	if c.localMode {
		t.Fatal("explicit IsNetworkEnabled:true should NOT be localMode, even in dev")
	}
	c.Track("u1", "purchase", nil) // fires an /collect POST

	urls := collectURLs(ch, 1, 2*time.Second)
	if len(urls) == 0 {
		t.Fatal("expected an /collect request when network explicitly enabled in dev")
	}
	if !strings.Contains(strings.Join(urls, " "), "https://edge.x/collect") {
		t.Fatalf("Track did not target the edge /collect endpoint: %v", urls)
	}
}

// TestNetworkOnByDefaultInProduction asserts (c) on by default in production:
// telemetry fires with no explicit option.
func TestNetworkOnByDefaultInProduction(t *testing.T) {
	t.Setenv("SHIPEASY_ENV", "production")
	hc, ch := recordingClient()
	c := NewEngine(Options{APIKey: "k", TelemetryURL: "https://e.x", HTTP: hc})

	if c.localMode {
		t.Fatal("production default should be network-ON (not localMode)")
	}
	c.GetFlag("g", User{})

	if urls := collectURLs(ch, 1, 2*time.Second); len(urls) == 0 {
		t.Fatal("expected a telemetry beacon in production by default")
	}
}

// TestExplicitNetworkOffOverridesProdDefault asserts an explicit false wins over
// the production default (and is distinguishable from the nil zero value).
func TestExplicitNetworkOffOverridesProdDefault(t *testing.T) {
	t.Setenv("SHIPEASY_ENV", "production")
	hc, ch := recordingClient()
	c := NewEngine(Options{
		APIKey:           "k",
		TelemetryURL:     "https://e.x",
		HTTP:             hc,
		IsNetworkEnabled: boolPtr(false),
	})

	if !c.localMode {
		t.Fatal("explicit IsNetworkEnabled:false should force localMode even in production")
	}
	c.GetFlag("g", User{})
	if urls := collectURLs(ch, 1, 300*time.Millisecond); len(urls) != 0 {
		t.Fatalf("expected ZERO requests when network explicitly off, got %v", urls)
	}
}

// TestTrackingOffByDefaultInDevKeepsNetwork asserts telemetry follows the env
// default independently: in dev, if the caller forces the network ON but leaves
// tracking unset, usage telemetry stays OFF (quiet by default) while other
// egress is allowed.
func TestTrackingOffByDefaultInDevKeepsNetwork(t *testing.T) {
	t.Setenv("SHIPEASY_ENV", "development")
	hc, ch := recordingClient()
	c := NewEngine(Options{
		APIKey:           "k",
		TelemetryURL:     "https://e.x",
		HTTP:             hc,
		IsNetworkEnabled: boolPtr(true), // network on, tracking left to env default (off in dev)
	})

	if c.localMode {
		t.Fatal("network explicitly on should not be localMode")
	}
	if !c.telemetry.disabled {
		t.Fatal("usage telemetry should default OFF in dev even when network is on")
	}
	c.GetFlag("g", User{})
	if urls := collectURLs(ch, 1, 300*time.Millisecond); len(urls) != 0 {
		t.Fatalf("expected ZERO telemetry beacons (tracking off) in dev, got %v", urls)
	}
}

// TestExplicitTrackingOnInDev asserts IsTrackingEnabled:true turns telemetry on
// in dev (given network is available).
func TestExplicitTrackingOnInDev(t *testing.T) {
	t.Setenv("SHIPEASY_ENV", "development")
	hc, ch := recordingClient()
	c := NewEngine(Options{
		APIKey:            "k",
		TelemetryURL:      "https://e.x",
		HTTP:              hc,
		IsNetworkEnabled:  boolPtr(true),
		IsTrackingEnabled: boolPtr(true),
	})
	if c.telemetry.disabled {
		t.Fatal("IsTrackingEnabled:true should enable telemetry")
	}
	c.GetFlag("g", User{})
	if urls := collectURLs(ch, 1, 2*time.Second); len(urls) == 0 {
		t.Fatal("expected a telemetry beacon with IsTrackingEnabled:true in dev")
	}
}

// TestDisableTelemetryBackCompat asserts the deprecated DisableTelemetry:true
// still forces telemetry off in production.
func TestDisableTelemetryBackCompat(t *testing.T) {
	t.Setenv("SHIPEASY_ENV", "production")
	hc, ch := recordingClient()
	c := NewEngine(Options{
		APIKey:           "k",
		TelemetryURL:     "https://e.x",
		HTTP:             hc,
		DisableTelemetry: true,
	})
	if !c.telemetry.disabled {
		t.Fatal("DisableTelemetry:true must force telemetry off even in production")
	}
	c.GetFlag("g", User{})
	if urls := collectURLs(ch, 1, 300*time.Millisecond); len(urls) != 0 {
		t.Fatalf("expected ZERO beacons with DisableTelemetry:true, got %v", urls)
	}
}

// TestConfiguredEnvDrivesEgressWithoutNativeVar asserts the fall-back path: with
// no native env var set, Options.Env: "dev" makes the engine quiet, while the
// default (prod) keeps it on.
func TestConfiguredEnvDrivesEgressWithoutNativeVar(t *testing.T) {
	for _, name := range nativeEnvVars {
		t.Setenv(name, "")
	}
	dev := NewEngine(Options{APIKey: "k", Env: "dev"})
	if !dev.localMode {
		t.Fatal("Env:dev with no native var should be offline by default")
	}
	prod := NewEngine(Options{APIKey: "k", Env: "prod"})
	if prod.localMode {
		t.Fatal("Env:prod with no native var should be network-on by default")
	}
}
