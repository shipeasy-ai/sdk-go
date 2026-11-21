package shipeasy

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Cross-impl fixture (shared with every Shipeasy SDK; copied from core).
const (
	fxKey     = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA"
	fxKeyID   = "k1"
	fxCookie  = "eyJ2IjoxLCJwaWQiOiJwcm9qX2ZpeHR1cmUiLCJrZXlJZCI6ImsxIiwibm9uY2UiOiJmaXh0dXJlLW5vbmNlIiwiZXhwIjo0MTAyNDQ0ODAwLCJvdiI6eyJnYXRlcyI6eyJsb2JieV9ob2xkIjp0cnVlLCJvbGRfbmF2IjpmYWxzZX0sImNvbmZpZ3MiOnsiYmFubmVyIjp7InRleHQiOiJoaSJ9fSwiZXhwcyI6eyJjaGVja291dF9leHAiOiJ0cmVhdG1lbnQifX19.8qd-pHu1e96Th-jcu4BZOhLmZjwhKrgn-uI8IWBr5Og"
	fxExpired = "eyJ2IjoxLCJwaWQiOiJwcm9qX2ZpeHR1cmUiLCJrZXlJZCI6ImsxIiwibm9uY2UiOiJmaXh0dXJlLW5vbmNlIiwiZXhwIjoxMDAwLCJvdiI6eyJnYXRlcyI6eyJsb2JieV9ob2xkIjp0cnVlLCJvbGRfbmF2IjpmYWxzZX0sImNvbmZpZ3MiOnsiYmFubmVyIjp7InRleHQiOiJoaSJ9fSwiZXhwcyI6eyJjaGVja291dF9leHAiOiJ0cmVhdG1lbnQifX19.fZEp7eGjZSdi5EvTB-5NZLbZpVXbcWbIaEYRmPvPbd8"
	fxTamper  = "eyJ2IjoxLCJwaWQiOiJwcm9qX2ZpeHR1cmUiLCJrZXlJZCI6ImsxIiwibm9uY2UiOiJmaXh0dXJlLW5vbmNlIiwiZXhwIjo0MTAyNDQ0ODAwLCJvdiI6eyJnYXRlcyI6eyJsb2JieV9ob2xkIjp0cnVlLCJvbGRfbmF2IjpmYWxzZX0sImNvbmZpZ3MiOnsiYmFubmVyIjp7InRleHQiOiJoaSJ9fSwiZXhwcyI6eyJjaGVja291dF9leHAiOiJ0cmVhdG1lbnQifX19.8qd-pHu1e96Th-jcu4BZOhLmZjwhKrgn-uI8IWBr5AA"
)

func TestVerifyOverrideCookieFixture(t *testing.T) {
	ov, err := verifyOverrideCookie(fxCookie, fxKey, fxKeyID, time.Now())
	if err != nil {
		t.Fatalf("fixture should verify: %v", err)
	}
	if !ov.gates["lobby_hold"] || ov.gates["old_nav"] {
		t.Fatalf("gates: %v", ov.gates)
	}
	if m, _ := ov.configs["banner"].(map[string]any); m["text"] != "hi" {
		t.Fatalf("configs: %v", ov.configs)
	}
	if ov.exps["checkout_exp"] != "treatment" {
		t.Fatalf("exps: %v", ov.exps)
	}
}

func TestVerifyOverrideCookieRejects(t *testing.T) {
	now := time.Now()
	cases := map[string][3]string{
		"expired":     {fxExpired, fxKey, fxKeyID},
		"tampered":    {fxTamper, fxKey, fxKeyID},
		"wrong keyId": {fxCookie, fxKey, "k2"},
		"wrong key":   {fxCookie, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", fxKeyID},
		"no dot":      {"abc", fxKey, fxKeyID},
		"two dots":    {fxCookie + ".x", fxKey, fxKeyID},
		"leading dot": {"." + fxCookie, fxKey, fxKeyID},
	}
	for name, c := range cases {
		if _, err := verifyOverrideCookie(c[0], c[1], c[2], now); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

func TestExtractOverrideCookie(t *testing.T) {
	if got := extractOverrideCookie(fxCookie); got != fxCookie {
		t.Fatal("bare value")
	}
	if got := extractOverrideCookie("a=1; se_ov=" + fxCookie + "; b=2"); got != fxCookie {
		t.Fatalf("header: %q", got)
	}
	if got := extractOverrideCookie("a=1; se_ov=x%2Ey; b=2"); got != "x.y" {
		t.Fatalf("url-decode: %q", got)
	}
	for _, in := range []string{"", "a=1; b=2", "nodots"} {
		if got := extractOverrideCookie(in); got != "" {
			t.Fatalf("%q => %q, want empty", in, got)
		}
	}
}

func overridesEngine(withKey bool) *Engine {
	flags := map[string]any{
		"gates": map[string]any{
			"lobby_hold": map[string]any{"enabled": 0, "rolloutPct": 0},
			"old_nav":    map[string]any{"enabled": 1, "rolloutPct": 10000},
		},
		"configs":      map[string]any{"banner": map[string]any{"value": "real"}},
		"killswitches": map[string]any{"checkout": map[string]any{"value": false}},
	}
	if withKey {
		flags["overrideKey"] = fxKey
		flags["overrideKeyId"] = fxKeyID
	}
	exps := map[string]any{
		"universes": map[string]any{"u": map[string]any{}},
		"experiments": map[string]any{"checkout_exp": map[string]any{
			"status": "running", "universe": "u", "salt": "s", "allocationPct": 10000,
			"groups": []any{
				map[string]any{"name": "control", "weight": 10000, "params": map[string]any{"c": "ctl"}},
				map[string]any{"name": "treatment", "weight": 0, "params": map[string]any{"c": "trt"}},
			},
		}},
	}
	return NewOfflineClientFromSnapshot(flags, exps)
}

func TestRequestOverridesClient(t *testing.T) {
	eng := overridesEngine(true)
	installGlobalEngine(eng, nil)
	user := User{"user_id": "u1"}
	ctx := WithRequestOverrides(context.Background(), "a=1; se_ov="+fxCookie)

	c := NewClient(user).WithContext(ctx)
	if d := c.GetFlagDetail("lobby_hold"); !d.Value || d.Reason != ReasonOverride {
		t.Fatalf("in scope: %+v", d)
	}
	if c.GetFlag("old_nav") {
		t.Fatal("old_nav should be forced off")
	}
	if !c.GetKillswitch("lobby_hold") {
		t.Fatal("gates namespace should cover kill switches")
	}
	if c.GetKillswitch("lobby_hold", "sub") {
		t.Fatal("per-switch-key reads are left alone")
	}
	if v, ok := c.GetConfig("banner"); !ok || v.(map[string]any)["text"] != "hi" {
		t.Fatalf("config: %v", v)
	}
	if v := c.GetConfigOr("banner", "d"); v.(map[string]any)["text"] != "hi" {
		t.Fatalf("configOr: %v", v)
	}
	a := c.Universe("u").Assign()
	if !a.Enrolled || a.Group != "treatment" || a.Peek("c", "") != "trt" {
		t.Fatalf("assign: %+v", a)
	}

	// Outside the scope: normal evaluation.
	plain := NewClient(user)
	if d := plain.GetFlagDetail("lobby_hold"); d.Value || d.Reason == ReasonOverride {
		t.Fatalf("outside scope: %+v", d)
	}
	if v, _ := plain.GetConfig("banner"); v != "real" {
		t.Fatalf("outside scope config: %v", v)
	}
	if g := plain.Universe("u").Assign().Group; g != "control" {
		t.Fatalf("outside scope group: %s", g)
	}

	// Programmatic override wins over the cookie.
	eng.OverrideFlag("lobby_hold", false)
	eng.OverrideConfig("banner", "prog")
	eng.OverrideExperiment("checkout_exp", "control", nil)
	if d := c.GetFlagDetail("lobby_hold"); d.Value || d.Reason != ReasonOverride {
		t.Fatalf("programmatic: %+v", d)
	}
	if v, _ := c.GetConfig("banner"); v != "prog" {
		t.Fatalf("programmatic config: %v", v)
	}
	if g := c.Universe("u").Assign().Group; g != "control" {
		t.Fatalf("programmatic exp: %s", g)
	}
}

func TestRequestOverridesFailClosed(t *testing.T) {
	user := User{"user_id": "u1"}
	// No key served by the platform.
	installGlobalEngine(overridesEngine(false), nil)
	c := NewClient(user).WithContext(WithRequestOverrides(context.Background(), fxCookie))
	if c.GetFlag("lobby_hold") {
		t.Fatal("no key: must not trust overrides")
	}
	// Key present but cookie bad.
	installGlobalEngine(overridesEngine(true), nil)
	for _, ck := range []string{fxExpired, fxTamper, ""} {
		c := NewClient(user).WithContext(WithRequestOverrides(context.Background(), ck))
		if c.GetFlag("lobby_hold") {
			t.Fatalf("cookie %.12q must be rejected", ck)
		}
	}
}

func TestMiddlewareScopesOverrides(t *testing.T) {
	installGlobalEngine(overridesEngine(true), nil)
	var in, after bool
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		in = NewClient(User{"user_id": "u1"}).WithContext(r.Context()).GetFlag("lobby_hold")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: OverrideCookieName, Value: fxCookie})
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !in {
		t.Fatal("middleware should scope verified overrides to the request context")
	}
	// The scope is the request's context: a fresh client has none.
	after = NewClient(User{"user_id": "u1"}).GetFlag("lobby_hold")
	if after {
		t.Fatal("override leaked outside the request")
	}
	// A request without the cookie sees normal evaluation.
	h2 := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		in = NewClient(User{"user_id": "u1"}).WithContext(r.Context()).GetFlag("lobby_hold")
	}))
	h2.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if in {
		t.Fatal("no cookie => no override")
	}
}

const fxDev = "eyJnYXRlcyI6eyJsb2JieV9ob2xkIjp0cnVlLCJvbGRfbmF2IjpmYWxzZX0sImNvbmZpZ3MiOnsiYmFubmVyIjp7InRleHQiOiJoaSJ9fSwiZXhwcyI6eyJjaGVja291dF9leHAiOiJ0cmVhdG1lbnQifX0"

func TestDevOverrideDecode(t *testing.T) {
	ov := decodeDevOverrideCookie(extractDevOverrideCookie("a=1; se_ov_dev=" + fxDev + "; b=2"))
	if ov == nil || !ov.gates["lobby_hold"] || ov.gates["old_nav"] || ov.exps["checkout_exp"] != "treatment" {
		t.Fatalf("decode: %+v", ov)
	}
	if decodeDevOverrideCookie(extractDevOverrideCookie("se_ov_dev=!!!;")) != nil {
		t.Fatal("malformed must be ignored")
	}
	if decodeDevOverrideCookie(extractDevOverrideCookie(fxDev)) == nil {
		t.Fatal("bare value")
	}
}

func TestDevOverridesGuards(t *testing.T) {
	user := User{"user_id": "u1"}
	hdr := "se_ov_dev=" + fxDev
	read := func(eng *Engine) bool {
		installGlobalEngine(eng, nil)
		return NewClient(user).WithContext(WithRequestOverrides(context.Background(), hdr)).GetFlag("lobby_hold")
	}
	// Option off.
	if read(overridesEngine(true)) {
		t.Fatal("option off: ignored")
	}
	// Option on, non-production.
	eng := overridesEngine(false)
	eng.devOverrides = true
	if !read(eng) {
		t.Fatal("option on: honored")
	}
	// NewEngine in a production process: ignored.
	t.Setenv("SHIPEASY_ENV", "production")
	if e := NewEngine(Options{APIKey: "k", DevOverrides: true, IsNetworkEnabled: boolPtr(false)}); e.devOverrides {
		t.Fatal("production: must ignore DevOverrides")
	}
	t.Setenv("SHIPEASY_ENV", "development")
	if e := NewEngine(Options{APIKey: "k", DevOverrides: true, IsNetworkEnabled: boolPtr(false)}); !e.devOverrides {
		t.Fatal("non-production: must honor DevOverrides")
	}
	if e := NewEngine(Options{APIKey: "k", IsNetworkEnabled: boolPtr(false)}); e.devOverrides {
		t.Fatal("default off")
	}
}

func TestSignedBeatsDevOverride(t *testing.T) {
	// Dev says lobby_hold=true; signed cookie says old_nav=false, lobby_hold=true.
	// Build a dev cookie that conflicts: old_nav=true (signed fixture says false).
	devJSON := `{"gates":{"old_nav":true,"extra":true}}`
	dev := base64.RawURLEncoding.EncodeToString([]byte(devJSON))
	eng := overridesEngine(true)
	eng.devOverrides = true
	installGlobalEngine(eng, nil)
	ctx := WithRequestOverrides(context.Background(), "se_ov="+fxCookie+"; se_ov_dev="+dev)
	c := NewClient(User{"user_id": "u1"}).WithContext(ctx)
	if c.GetFlag("old_nav") {
		t.Fatal("signed (false) must beat dev (true)")
	}
	if d := c.GetFlagDetail("extra"); !d.Value || d.Reason != ReasonOverride {
		t.Fatalf("dev-only name: %+v", d)
	}
	if !c.GetFlag("lobby_hold") {
		t.Fatal("signed lobby_hold")
	}
}

func TestOverridesHeaderFallback(t *testing.T) {
	user := User{"user_id": "u1"}
	eng := overridesEngine(true)
	installGlobalEngine(eng, nil)
	flag := func(ctx context.Context, n string) bool {
		return NewClient(user).WithContext(ctx).GetFlag(n)
	}
	// Header-only signed cookie applies.
	ctx := WithRequestOverrides(context.Background(), "", "se_ov="+fxCookie)
	if !flag(ctx, "lobby_hold") {
		t.Fatal("header-only signed cookie should apply")
	}
	// Tampered header cookie is rejected.
	if flag(WithRequestOverrides(context.Background(), "", "se_ov="+fxTamper), "lobby_hold") {
		t.Fatal("tampered header must be rejected")
	}
	// Header-only dev cookie: guard off => ignored; guard on => applied.
	hdr := "se_ov_dev=" + fxDev
	if flag(WithRequestOverrides(context.Background(), "", hdr), "lobby_hold") {
		t.Fatal("dev header ignored without DevOverrides")
	}
	eng.devOverrides = true
	if !flag(WithRequestOverrides(context.Background(), "", hdr), "lobby_hold") {
		t.Fatal("dev header should apply under the guards")
	}
	// Cookie beats header for the same name: dev cookie says lobby_hold=false.
	devOff := base64.RawURLEncoding.EncodeToString([]byte(`{"gates":{"lobby_hold":false}}`))
	if flag(WithRequestOverrides(context.Background(), "se_ov_dev="+devOff, hdr), "lobby_hold") {
		t.Fatal("Cookie header must win over X-Shipeasy-Overrides")
	}
	// Middleware reads the header.
	var got bool
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = NewClient(user).WithContext(r.Context()).GetFlag("lobby_hold")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("x-shipeasy-overrides", "se_ov="+fxCookie)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !got {
		t.Fatal("middleware should read X-Shipeasy-Overrides")
	}
}

func TestExtractUnrelatedSingleCookieIsNotBare(t *testing.T) {
	if got := extractOverrideCookie("theme=a.b"); got != "" {
		t.Fatalf("unrelated cookie treated as bare se_ov: %q", got)
	}
	// So the X-Shipeasy-Overrides fallback still applies.
	installGlobalEngine(overridesEngine(true), nil)
	ctx := WithRequestOverrides(context.Background(), "theme=a.b", "se_ov="+fxCookie)
	if !NewClient(User{"user_id": "u1"}).WithContext(ctx).GetFlag("lobby_hold") {
		t.Fatal("header fallback must apply beside an unrelated cookie")
	}
}

func TestExtractDevUnrelatedSingleCookieIsNotBare(t *testing.T) {
	if got := extractDevOverrideCookie("theme=a.b"); got != "" {
		t.Fatalf("unrelated cookie treated as bare se_ov_dev: %q", got)
	}
	if got := extractDevOverrideCookie("theme=a; se_ov_dev=" + fxDev); got != fxDev {
		t.Fatalf("header parse: %q", got)
	}
}
