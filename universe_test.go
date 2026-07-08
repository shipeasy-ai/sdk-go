package shipeasy

import (
	"fmt"
	"testing"
)

// Universe-first assignment (the mutual-exclusion pool model, doc 20 §B).
//
// engine.Universe(name).Assign(user) returns an Assignment: the <=1 experiment
// the unit landed in within the universe, its variant, and resolved params
// (variant override ?? universe default ?? fallback). These tests lock the merge
// (§B2), the not-enrolled defaults path, pooled mutual exclusion (§B4), reserved
// headroom (§B5), and the holdout gate (§B3). They seed the blobs directly (no
// network) the way the eval-vectors / parity tests do. Mirrors ts-sdk's
// src/__tests__/universe-assign.test.ts.

const universeMod = 10000

func universeSeg(universe, uid string) uint32 {
	return Murmur3(universe+":"+uid) % universeMod
}

// assignEngine builds a no-network, initialized engine seeded with the given
// flags + exps blobs.
func assignEngine(flags *flagsBlob, exps *expsBlob) *Engine {
	c := NewTestClient()
	c.flags = flags
	c.exps = exps
	return c
}

func intPtr(v int) *int { return &v }

// §B2 — param merge: variant override wins, unset params inherit the universe
// default, unknown fields fall back to the caller's fallback.
func TestAssignParamMerge(t *testing.T) {
	e := assignEngine(
		&flagsBlob{},
		&expsBlob{
			Universes: map[string]universe{
				"u": {ParamSchema: []universeParam{
					{Name: "button_color", Type: "string", Default: "red"},
					{Name: "size", Type: "int", Default: float64(1)},
				}},
			},
			Experiments: map[string]experiment{
				"exp": {
					Universe:      "u",
					AllocationPct: 10000,
					Salt:          "s",
					Status:        "running",
					Groups:        []group{{Name: "treatment", Weight: 10000, Params: map[string]any{"button_color": "blue"}}},
				},
			},
		},
	)

	a := e.Universe("u").Assign(User{"user_id": "u1"})
	if !a.Enrolled {
		t.Fatalf("u1 should be enrolled")
	}
	if a.Group != "treatment" {
		t.Fatalf("group = %q, want treatment", a.Group)
	}
	// Overridden by the variant.
	if a.Get("button_color", nil) != "blue" {
		t.Errorf("button_color = %v, want blue (variant override)", a.Get("button_color", nil))
	}
	// Not overridden → inherited from the universe default.
	if a.Get("size", nil) != float64(1) {
		t.Errorf("size = %v, want 1 (universe default)", a.Get("size", nil))
	}
	// Absent everywhere → the caller's fallback.
	if a.Get("missing", "fb") != "fb" {
		t.Errorf("missing = %v, want fb (fallback)", a.Get("missing", "fb"))
	}
}

// Not enrolled (allocation 0) → group empty, but the universe default still
// resolves through Get().
func TestAssignNotEnrolledGetsUniverseDefaults(t *testing.T) {
	e := assignEngine(
		&flagsBlob{},
		&expsBlob{
			Universes: map[string]universe{
				"u": {ParamSchema: []universeParam{{Name: "button_color", Type: "string", Default: "red"}}},
			},
			Experiments: map[string]experiment{
				"exp": {
					Universe:      "u",
					AllocationPct: 0, // nobody allocated
					Salt:          "s",
					Status:        "running",
					Groups:        []group{{Name: "treatment", Weight: 10000, Params: map[string]any{"button_color": "blue"}}},
				},
			},
		},
	)

	a := e.Universe("u").Assign(User{"user_id": "u1"})
	if a.Enrolled {
		t.Fatalf("allocation 0 should not enrol")
	}
	if a.Group != "" {
		t.Fatalf("group = %q, want empty", a.Group)
	}
	// Not enrolled → universe default, not the variant override.
	if a.Get("button_color", nil) != "red" {
		t.Errorf("button_color = %v, want red (universe default, not variant)", a.Get("button_color", nil))
	}
}

// §B4 — pooled mutual exclusion: two experiments in ONE universe, hashVersion 2,
// disjoint pool slices A=[0,4000), B=[4000,8000); segment >= 8000 is unallocated
// headroom. No unit lands in both; each slice + the free tail all get some units.
func TestAssignPooledMutualExclusion(t *testing.T) {
	e := assignEngine(
		&flagsBlob{},
		&expsBlob{
			Universes: map[string]universe{"u": {}},
			Experiments: map[string]experiment{
				"expA": {
					Universe: "u", HashVersion: intPtr(2), PoolOffsetBp: intPtr(0), PoolSizeBp: intPtr(4000),
					AllocationPct: 10000, Salt: "sA", Status: "running",
					Groups: []group{{Name: "A", Weight: 10000, Params: map[string]any{}}},
				},
				"expB": {
					Universe: "u", HashVersion: intPtr(2), PoolOffsetBp: intPtr(4000), PoolSizeBp: intPtr(4000),
					AllocationPct: 10000, Salt: "sB", Status: "running",
					Groups: []group{{Name: "B", Weight: 10000, Params: map[string]any{}}},
				},
			},
		},
	)

	inA, inB, neither := 0, 0, 0
	for i := 0; i < 400; i++ {
		uid := fmt.Sprintf("u%d", i)
		a := e.Universe("u").Assign(User{"user_id": uid})
		// assign returns <=1 experiment, so double-enrolment is impossible by
		// design; cross-check the landing against the unit's own universe segment.
		seg := universeSeg("u", uid)
		switch a.Name {
		case "expA":
			inA++
			if seg >= 4000 {
				t.Errorf("uid %q in expA but seg=%d (>=4000)", uid, seg)
			}
		case "expB":
			inB++
			if seg < 4000 || seg >= 8000 {
				t.Errorf("uid %q in expB but seg=%d (not in [4000,8000))", uid, seg)
			}
		default:
			neither++
			if a.Enrolled {
				t.Errorf("uid %q not in A/B but Enrolled=true", uid)
			}
			if seg < 8000 {
				t.Errorf("uid %q not enrolled but seg=%d (<8000)", uid, seg)
			}
		}
	}
	// The partition is real: all three buckets are populated over 400 users.
	if inA == 0 || inB == 0 || neither == 0 {
		t.Errorf("partition not populated: inA=%d inB=%d neither=%d", inA, inB, neither)
	}
	if inA+inB+neither != 400 {
		t.Errorf("counts don't sum to 400: %d", inA+inB+neither)
	}
}

// §B5 — reserved headroom: 100% allocation, one group summing to 5000 with
// reservedHeadroomBp 5000 → units whose group hash falls in the reserved tail are
// left not-enrolled. Both the enrolled and reserved buckets are populated.
func TestAssignReservedHeadroom(t *testing.T) {
	e := assignEngine(
		&flagsBlob{},
		&expsBlob{
			Universes: map[string]universe{"u": {}},
			Experiments: map[string]experiment{
				"exp": {
					Universe: "u", AllocationPct: 10000, ReservedHeadroomBp: intPtr(5000),
					Salt: "s", Status: "running",
					Groups: []group{{Name: "control", Weight: 5000, Params: map[string]any{}}},
				},
			},
		},
	)

	enrolled, reserved := 0, 0
	for i := 0; i < 400; i++ {
		a := e.Universe("u").Assign(User{"user_id": fmt.Sprintf("u%d", i)})
		if a.Enrolled {
			enrolled++
		} else {
			reserved++
		}
	}
	// Both populated: allocation is 100% yet the reserved tail carves out ~half.
	if enrolled == 0 || reserved == 0 {
		t.Errorf("reserved headroom not exercised: enrolled=%d reserved=%d", enrolled, reserved)
	}
}

// §B3 — holdoutGate: a unit for whom the holdout gate passes is held out (not
// enrolled).
func TestAssignHoldoutGate(t *testing.T) {
	e := assignEngine(
		&flagsBlob{Gates: map[string]gate{
			// enabled, 100% rollout, no rules → passes for every identified unit.
			"hg": {Rules: []rule{}, RolloutPct: 10000, Salt: "hg", Enabled: float64(1)},
		}},
		&expsBlob{
			Universes: map[string]universe{"u": {}},
			Experiments: map[string]experiment{
				"exp": {
					Universe: "u", HoldoutGate: "hg", AllocationPct: 10000,
					Salt: "s", Status: "running",
					Groups: []group{{Name: "treatment", Weight: 10000, Params: map[string]any{}}},
				},
			},
		},
	)

	a := e.Universe("u").Assign(User{"user_id": "u1"})
	if a.Enrolled {
		t.Errorf("holdout-gated unit should not be enrolled")
	}
	if a.Group != "" {
		t.Errorf("group = %q, want empty (held out)", a.Group)
	}
}
