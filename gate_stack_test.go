package shipeasy

import (
	"encoding/json"
	"testing"
)

// Gatekeeper `stack` evaluation.
//
// Regression guard for the bug where evalGate read only the flat
// Rules+RolloutPct fields and ignored a modern gate's ordered stack. The
// canonical model is the stack (mirrors @shipeasy/core evalGatekeeper + the edge
// worker); the flat columns are a lossy approximation that can invert the result
// (a whitelist condition at 100% followed by a 0% public rollout flattens to
// rolloutPct:0). These vectors lock the SDK to the stack.

const stackTestP = "e976b15e-3ccc-44d3-821d-87f06d5a0e43"

func pct(n int) *int { return &n }

// whitelistGateJSON is the exact shape the KV rebuild ships for a whitelist
// gatekeeper: a condition (no explicit rolloutPct ⇒ 100%) that whitelists a
// project, then a locked 0% public rollout. The flat columns are the lossy
// approximation and must NOT be what decides the result. Decoded from JSON so we
// also exercise the `stack` decoder.
func whitelistGate(t *testing.T) gate {
	t.Helper()
	raw := `{
		"name": "release_module",
		"enabled": 1,
		"salt": "caf3a1ae",
		"rules": [{"attr": "project_id", "op": "in", "value": ["` + stackTestP + `"]}],
		"rolloutPct": 0,
		"stack": [
			{"id": "gq578snc", "type": "condition", "pass": "all",
			 "rules": [{"attr": "project_id", "op": "in", "value": ["` + stackTestP + `"]}]},
			{"id": "gu0uein4", "type": "rollout", "rolloutPct": 0, "bucketBy": "user_id", "salt": "public"}
		]
	}`
	var g gate
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		t.Fatalf("decode whitelist gate: %v", err)
	}
	if len(g.Stack) != 2 {
		t.Fatalf("stack did not decode: got %d entries", len(g.Stack))
	}
	return g
}

func TestStackWhitelistedCallerPassesDespiteFlatZero(t *testing.T) {
	g := whitelistGate(t)
	// The regression: the flat path would read "matches whitelist AND 0% bucket"
	// = false. The stack short-circuits on the 100% condition → true.
	u := User{"user_id": "cdewqzx@gmail.com", "project_id": stackTestP}
	if !evalGate(g, u) {
		t.Fatal("whitelisted caller must pass via the stack even though flat rolloutPct is 0")
	}
}

func TestStackNonWhitelistedCallerHidden(t *testing.T) {
	g := whitelistGate(t)
	u := User{"user_id": "someone@else.com", "project_id": "other-project"}
	if evalGate(g, u) {
		t.Fatal("non-whitelisted caller must be off (condition misses, public rollout 0%)")
	}
}

func TestStackWhitelistedNoIdentity(t *testing.T) {
	g := whitelistGate(t)
	// No user_id/anonymous_id: a fully-rolled (100%) condition is answerable
	// without a unit id.
	if !evalGate(g, User{"project_id": stackTestP}) {
		t.Fatal("whitelisted caller with no identity must pass (100% condition needs no unit)")
	}
}

func TestStackConditionGatesOnOwnRollout(t *testing.T) {
	g := gate{
		Enabled:    1,
		Salt:       "s",
		RolloutPct: 0,
		Stack: []stackEntry{
			{
				ID:         "c1",
				Type:       "condition",
				Pass:       "all",
				Rules:      []rule{{Attr: "project_id", Op: "in", Value: []any{stackTestP}}},
				RolloutPct: pct(0), // matched but 0% → never
			},
		},
	}
	if evalGate(g, User{"user_id": "u1", "project_id": stackTestP}) {
		t.Fatal("a matched condition at 0% must still be off")
	}
}

func TestStackPassAny(t *testing.T) {
	g := gate{
		Enabled: 1,
		Salt:    "s",
		Stack: []stackEntry{
			{
				ID:   "c1",
				Type: "condition",
				Pass: "any",
				Rules: []rule{
					{Attr: "plan", Op: "eq", Value: "pro"},
					{Attr: "project_id", Op: "in", Value: []any{stackTestP}},
				},
			},
		},
	}
	// One branch matches (project_id) even though plan does not → pass.
	if !evalGate(g, User{"user_id": "u", "plan": "free", "project_id": stackTestP}) {
		t.Fatal("pass:any must pass when a single rule matches")
	}
	if evalGate(g, User{"user_id": "u", "plan": "free", "project_id": "x"}) {
		t.Fatal("pass:any must fail when no rule matches")
	}
}

func TestStackCatchAllRollout(t *testing.T) {
	g := gate{
		Enabled: 1,
		Salt:    "s",
		Stack: []stackEntry{
			{
				ID:    "c1",
				Type:  "condition",
				Pass:  "all",
				Rules: []rule{{Attr: "project_id", Op: "in", Value: []any{stackTestP}}},
			},
			{ID: "public", Type: "rollout", RolloutPct: pct(10000)}, // everyone else: 100%
		},
	}
	if !evalGate(g, User{"user_id": "u", "project_id": "not-whitelisted"}) {
		t.Fatal("a caller who misses the condition must fall through to the 100% catch-all rollout")
	}
}

func TestStackDisabledOrKilledOff(t *testing.T) {
	base := whitelistGate(t)
	u := User{"user_id": "u", "project_id": stackTestP}

	disabled := base
	disabled.Enabled = 0
	if evalGate(disabled, u) {
		t.Fatal("a disabled stacked gate must be off")
	}

	killed := base
	killed.Killswitch = 1
	if evalGate(killed, u) {
		t.Fatal("a killed stacked gate must be off")
	}
}

func TestStacklessGateUsesFlatPath(t *testing.T) {
	on := gate{Enabled: 1, Salt: "s", RolloutPct: 10000}
	off := gate{Enabled: 1, Salt: "s", RolloutPct: 0}
	if !evalGate(on, User{"user_id": "u"}) {
		t.Fatal("stack-less 100% gate must be on via the flat path")
	}
	if evalGate(off, User{"user_id": "u"}) {
		t.Fatal("stack-less 0% gate must be off via the flat path")
	}
}
