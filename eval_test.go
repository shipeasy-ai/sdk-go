package shipeasy

import "testing"

// The no-unit evaluation rule is a cross-SDK contract: a request with no unit id
// answers a fully-rolled gate as on (it needs no bucketing) but a fractional
// gate as off (it needs a stable unit). See
// experiment-platform/18-identity-bucketing.md.
func TestEvalGateNoUnit(t *testing.T) {
	full := gate{Enabled: true, Salt: "s", RolloutPct: 10000}
	if !evalGate(full, User{}) {
		t.Fatal("100% gate must be on for an unidentified request")
	}

	fractional := gate{Enabled: true, Salt: "s", RolloutPct: 5000}
	if evalGate(fractional, User{}) {
		t.Fatal("fractional gate must be off for an unidentified request")
	}

	// A disabled / killed gate is off regardless of rollout or unit.
	if evalGate(gate{Enabled: false, RolloutPct: 10000}, User{}) {
		t.Fatal("disabled gate must be off even at 100%")
	}
	if evalGate(gate{Enabled: true, Killswitch: true, RolloutPct: 10000}, User{}) {
		t.Fatal("killed gate must be off even at 100%")
	}

	// Targeting rules still gate the short-circuit: a non-matching rule wins.
	ruled := gate{
		Enabled:    true,
		RolloutPct: 10000,
		Rules:      []rule{{Attr: "plan", Op: "eq", Value: "pro"}},
	}
	if evalGate(ruled, User{}) {
		t.Fatal("a non-matching targeting rule must keep a 100% gate off")
	}
	if !evalGate(ruled, User{"plan": "pro"}) {
		t.Fatal("matching the rule should let the 100% gate through without a unit")
	}
}

func TestEvalGateWithUnitUnchanged(t *testing.T) {
	// With a unit, bucketing is deterministic; a 0% gate is always off and a
	// 100% gate always on.
	if evalGate(gate{Enabled: true, Salt: "s", RolloutPct: 0}, User{"user_id": "u1"}) {
		t.Fatal("0% gate must be off for an identified user")
	}
	if !evalGate(gate{Enabled: true, Salt: "s", RolloutPct: 10000}, User{"user_id": "u1"}) {
		t.Fatal("100% gate must be on for an identified user")
	}
}
