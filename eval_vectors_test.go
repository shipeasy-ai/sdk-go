package shipeasy

import (
	"encoding/json"
	"os"
	"testing"
)

// Cross-language eval-parity golden vectors. The fixture in testdata/eval-vectors.json
// is a byte-identical copy of packages/core/src/eval/__fixtures__/eval-vectors.json
// (the platform's canonical source of truth). This SDK's bucketing MUST reproduce
// every vector, or it has silently drifted from the platform.

type evalVectors struct {
	BucketModulo int `json:"bucketModulo"`
	Hash         []struct {
		Input string `json:"input"`
		Hash  uint32 `json:"hash"`
	} `json:"hash"`
	Gate []struct {
		Note string `json:"note"`
		Gate gate   `json:"gate"`
		User User   `json:"user"`
		Pass bool   `json:"pass"`
	} `json:"gate"`
	Experiment []struct {
		Note         string     `json:"note"`
		Experiment   experiment `json:"experiment"`
		User         User       `json:"user"`
		Flags        map[string]bool `json:"flags"`
		HoldoutRange []int      `json:"holdoutRange"`
		Result       struct {
			InExperiment bool    `json:"inExperiment"`
			Group        *string `json:"group"`
		} `json:"result"`
	} `json:"experiment"`
}

func loadVectors(t *testing.T) evalVectors {
	t.Helper()
	raw, err := os.ReadFile("testdata/eval-vectors.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var v evalVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return v
}

func TestEvalVectorsHash(t *testing.T) {
	v := loadVectors(t)
	if len(v.Hash) == 0 {
		t.Fatal("no hash vectors loaded")
	}
	n := 0
	for _, hv := range v.Hash {
		got := Murmur3(hv.Input)
		if got != hv.Hash {
			t.Errorf("Murmur3(%q) = %d, want %d", hv.Input, got, hv.Hash)
			continue
		}
		n++
	}
	t.Logf("hash vectors passed: %d/%d", n, len(v.Hash))
}

func TestEvalVectorsGate(t *testing.T) {
	v := loadVectors(t)
	if len(v.Gate) == 0 {
		t.Fatal("no gate vectors loaded")
	}
	n := 0
	for i, gv := range v.Gate {
		got := evalGate(gv.Gate, gv.User)
		if got != gv.Pass {
			t.Errorf("gate vector %d (%s): evalGate = %v, want %v", i, gv.Note, got, gv.Pass)
			continue
		}
		n++
	}
	t.Logf("gate vectors passed: %d/%d", n, len(v.Gate))
}

func TestEvalVectorsExperiment(t *testing.T) {
	v := loadVectors(t)
	if len(v.Experiment) == 0 {
		t.Fatal("no experiment vectors loaded")
	}
	n := 0
	for i, ev := range v.Experiment {
		exp := ev.Experiment

		// Map the fixture's flags{gateName:bool} onto a flagsBlob the SDK's
		// evalExperiment consumes: a true flag becomes a fully-rolled enabled gate
		// (passes for any unit), a false flag a disabled gate (never passes).
		flags := &flagsBlob{Gates: map[string]gate{}}
		for name, on := range ev.Flags {
			if on {
				flags.Gates[name] = gate{Enabled: true, RolloutPct: 10000}
			} else {
				flags.Gates[name] = gate{Enabled: false}
			}
		}

		// Map the fixture's holdoutRange onto the experiment's universe.
		exps := &expsBlob{Universes: map[string]universe{}}
		if ev.HoldoutRange != nil {
			exps.Universes[exp.Universe] = universe{HoldoutRange: ev.HoldoutRange}
		}

		got := evalExperiment(&exp, flags, exps, ev.User)

		if got.InExperiment != ev.Result.InExperiment {
			t.Errorf("experiment vector %d (%s): InExperiment = %v, want %v", i, ev.Note, got.InExperiment, ev.Result.InExperiment)
			continue
		}
		if ev.Result.InExperiment {
			if ev.Result.Group == nil {
				t.Errorf("experiment vector %d (%s): fixture inExperiment=true but group is null", i, ev.Note)
				continue
			}
			if got.Group != *ev.Result.Group {
				t.Errorf("experiment vector %d (%s): group = %q, want %q", i, ev.Note, got.Group, *ev.Result.Group)
				continue
			}
		}
		n++
	}
	t.Logf("experiment vectors passed: %d/%d", n, len(v.Experiment))
}
