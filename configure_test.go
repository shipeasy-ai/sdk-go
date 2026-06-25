package shipeasy

import (
	"sync"
	"testing"
)

// resetGlobalsForTest wipes the process-wide Configure state so each test starts
// clean. Configure uses sync.Once (first-config-wins in production); tests reset
// the Once and the globals directly to exercise both the configured and the
// not-configured paths in one process.
func resetGlobalsForTest() {
	globalEngineMu.Lock()
	globalEngine = nil
	globalAttrs = nil
	globalEngineMu.Unlock()
	configureOnce = sync.Once{}
	SetDefaultEngine(nil)
}

// seedGlobalEngine installs an offline (no-network) engine + attribute transform
// as the package globals, mimicking what Configure does but without the network
// fetch. flags is a raw /sdk/flags body.
func seedGlobalEngine(flags any, attrs func(any) User) {
	eng := NewOfflineClientFromSnapshot(flags, nil)
	globalEngineMu.Lock()
	globalEngine = eng
	if attrs == nil {
		attrs = identityAttrsFn
	}
	globalAttrs = attrs
	globalEngineMu.Unlock()
	SetDefaultEngine(eng)
}

// A gate "pro_feature" on for plan=pro (fully rolled, rule-gated).
var proGateBlob = map[string]any{
	"gates": map[string]any{
		"pro_feature": map[string]any{
			"enabled": true, "salt": "s", "rolloutPct": 10000,
			"rules": []any{map[string]any{"attr": "plan", "op": "eq", "value": "pro"}},
		},
	},
	"configs": map[string]any{
		"copy": map[string]any{"value": map[string]any{"cta": "Buy"}},
	},
	"killswitches": map[string]any{
		"payments": map[string]any{"value": true, "switches": map[string]any{"eu": false}},
	},
}

// Configure({apiKey}) then NewClient({...}).GetFlag(...) works with the identity
// transform (the user passed is already a User attribute map).
func TestConfigureThenNewClientGetFlag(t *testing.T) {
	resetGlobalsForTest()
	defer resetGlobalsForTest()

	seedGlobalEngine(proGateBlob, nil) // identity transform

	pro := NewClient(User{"user_id": "u1", "plan": "pro"})
	if !pro.GetFlag("pro_feature") {
		t.Errorf("pro user GetFlag(pro_feature) = false, want true")
	}
	free := NewClient(User{"user_id": "u2", "plan": "free"})
	if free.GetFlag("pro_feature") {
		t.Errorf("free user GetFlag(pro_feature) = true, want false")
	}

	// A plain map[string]any is also accepted by the identity transform.
	pro2 := NewClient(map[string]any{"user_id": "u3", "plan": "pro"})
	if !pro2.GetFlag("pro_feature") {
		t.Errorf("map[string]any pro user GetFlag = false, want true")
	}
}

// The Attributes transform is applied once in the constructor: a raw custom user
// object is mapped to the attribute map evaluation actually uses.
func TestConfigureAttributesTransformApplied(t *testing.T) {
	resetGlobalsForTest()
	defer resetGlobalsForTest()

	type account struct {
		ID   string
		Tier string
	}
	transform := func(u any) User {
		a := u.(*account)
		return User{"user_id": a.ID, "plan": a.Tier} // map Tier -> plan
	}
	seedGlobalEngine(proGateBlob, transform)

	c := NewClient(&account{ID: "acct1", Tier: "pro"})
	if !c.GetFlag("pro_feature") {
		t.Errorf("transform not applied: GetFlag(pro_feature) = false, want true (Tier->plan=pro)")
	}
	// The bound attribute map must be the TRANSFORMED one.
	if c.attributes["plan"] != "pro" || c.attributes["user_id"] != "acct1" {
		t.Errorf("bound attributes = %v, want {user_id:acct1 plan:pro}", c.attributes)
	}

	free := NewClient(&account{ID: "acct2", Tier: "free"})
	if free.GetFlag("pro_feature") {
		t.Errorf("free tier mapped user got pro_feature = true, want false")
	}
}

// Other bound methods forward with the bound user / to the engine.
func TestBoundClientForwarders(t *testing.T) {
	resetGlobalsForTest()
	defer resetGlobalsForTest()
	seedGlobalEngine(proGateBlob, nil)

	c := NewClient(User{"user_id": "u1", "plan": "pro"})

	if d := c.GetFlagDetail("pro_feature"); !d.Value || d.Reason != ReasonRuleMatch {
		t.Errorf("GetFlagDetail = %+v, want {true RULE_MATCH}", d)
	}
	if c.GetFlagOr("missing", true) != true {
		t.Errorf("GetFlagOr(missing, true) should fall back to def")
	}
	if v, ok := c.GetConfig("copy"); !ok || v == nil {
		t.Errorf("GetConfig(copy) = %v,%v want a value", v, ok)
	}
	if c.GetConfigOr("nope", "fb") != "fb" {
		t.Errorf("GetConfigOr(nope) should return fallback")
	}
	// Killswitch: top-level true, but the "eu" per-key switch is false.
	if !c.GetKillswitch("payments") {
		t.Errorf("GetKillswitch(payments) = false, want true (top-level engaged)")
	}
	if c.GetKillswitch("payments", "eu") {
		t.Errorf("GetKillswitch(payments, eu) = true, want false (per-key override off)")
	}
	if c.GetKillswitch("absent") {
		t.Errorf("GetKillswitch(absent) = true, want false")
	}
}

// Constructing a Client before Configure must fail loudly (panic).
func TestNewClientBeforeConfigurePanics(t *testing.T) {
	resetGlobalsForTest()
	defer resetGlobalsForTest()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("NewClient before Configure did not panic")
		}
		msg, _ := r.(string)
		if msg == "" {
			t.Fatalf("panic value = %v, want a descriptive string", r)
		}
	}()
	_ = NewClient(User{"user_id": "u1"})
}

// Configure is first-config-wins (idempotent) and returns the global engine; a
// second call does not replace the engine. Uses localMode-style options with no
// network by pointing at an unused base — InitOnce runs in a goroutine and any
// failure is swallowed (logged), so the test only asserts the engine identity.
func TestConfigureFirstWins(t *testing.T) {
	resetGlobalsForTest()
	defer resetGlobalsForTest()

	e1 := Configure(Options{APIKey: "k1", DisableTelemetry: true, BaseURL: "http://127.0.0.1:0"})
	e2 := Configure(Options{APIKey: "k2", DisableTelemetry: true, BaseURL: "http://127.0.0.1:0"})
	if e1 == nil || e2 == nil {
		t.Fatalf("Configure returned nil")
	}
	if e1 != e2 {
		t.Errorf("Configure is not first-wins: second call built a new engine")
	}
	if e1.apiKey != "k1" {
		t.Errorf("engine apiKey = %q, want k1 (first config wins)", e1.apiKey)
	}
	if ConfiguredEngine() != e1 {
		t.Errorf("ConfiguredEngine() did not return the configured engine")
	}
}
