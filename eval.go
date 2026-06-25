package shipeasy

import (
	"fmt"
	"regexp"
	"strconv"
)

type ExperimentResult struct {
	InExperiment bool
	Group        string
	Params       any
}

type User map[string]any

type gate struct {
	Enabled    any    `json:"enabled"`
	Killswitch any    `json:"killswitch"`
	Salt       string `json:"salt"`
	RolloutPct int    `json:"rolloutPct"`
	Rules      []rule `json:"rules"`
}

type rule struct {
	Attr  string `json:"attr"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

type experiment struct {
	Status        string  `json:"status"`
	TargetingGate string  `json:"targetingGate"`
	Universe      string  `json:"universe"`
	Salt          string  `json:"salt"`
	AllocationPct int     `json:"allocationPct"`
	Groups        []group `json:"groups"`
	// BucketBy is an optional attribute to bucket on instead of the individual
	// (e.g. "company_id" to keep a whole org on one variant). When empty/absent,
	// bucketing falls back to user_id ?? anonymous_id. Drives the holdout,
	// allocation AND group hashes so all three agree. See experiment-platform doc 20.
	BucketBy string `json:"bucketBy"`
}

type group struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
	Params any    `json:"params"`
}

type universe struct {
	HoldoutRange []int `json:"holdout_range"`
}

type flagsBlob struct {
	Gates   map[string]gate `json:"gates"`
	Configs map[string]struct {
		Value any `json:"value"`
	} `json:"configs"`
	// Killswitches ride the flags blob alongside gates. Each entry carries a
	// top-level value/enabled plus an optional per-key switches map (the
	// dashboard "switches" feature). Read via Engine.GetKillswitch.
	Killswitches map[string]killswitch `json:"killswitches"`
}

// killswitch is one entry in the flags blob's killswitches map. Value (or the
// legacy Enabled) is the top-level kill state; Switches holds named per-key
// overrides.
type killswitch struct {
	Value    any            `json:"value"`
	Enabled  any            `json:"enabled"`
	Switches map[string]any `json:"switches"`
}

type expsBlob struct {
	Experiments map[string]experiment `json:"experiments"`
	Universes   map[string]universe   `json:"universes"`
}

func enabled(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x == 1
	case int:
		return x == 1
	}
	return false
}

func toNum(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

func userID(u User) string {
	if v, ok := u["user_id"]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	if v, ok := u["anonymous_id"]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// pickIdentifier resolves the bucketing unit for experiment assignment. When
// bucketBy is set and the user carries a non-empty string (or numeric) value for
// it, that value is the unit (e.g. "company_id" keeps a whole org on one
// variant). Otherwise it falls back to user_id ?? anonymous_id. Mirrors the
// canonical pickIdentifier in packages/core/src/eval/gate.ts.
func pickIdentifier(u User, bucketBy string) string {
	if bucketBy != "" {
		if v, ok := u[bucketBy]; ok && v != nil {
			switch x := v.(type) {
			case string:
				if x != "" {
					return x
				}
			case float64:
				return strconv.FormatFloat(x, 'g', -1, 64)
			case int:
				return strconv.Itoa(x)
			case int64:
				return strconv.FormatInt(x, 10)
			}
		}
	}
	return userID(u)
}

func matchRule(r rule, u User) bool {
	actual := u[r.Attr]
	switch r.Op {
	case "eq":
		return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", r.Value)
	case "neq":
		return fmt.Sprintf("%v", actual) != fmt.Sprintf("%v", r.Value)
	case "in", "not_in":
		arr, ok := r.Value.([]any)
		if !ok {
			return r.Op == "not_in"
		}
		found := false
		for _, x := range arr {
			if fmt.Sprintf("%v", x) == fmt.Sprintf("%v", actual) {
				found = true
				break
			}
		}
		if r.Op == "in" {
			return found
		}
		return !found
	case "contains":
		as, aok := actual.(string)
		vs, vok := r.Value.(string)
		if aok && vok {
			return regexp.QuoteMeta(vs) != "" && contains(as, vs)
		}
		if arr, ok := actual.([]any); ok {
			for _, x := range arr {
				if fmt.Sprintf("%v", x) == fmt.Sprintf("%v", r.Value) {
					return true
				}
			}
		}
		return false
	case "regex":
		as, aok := actual.(string)
		vs, vok := r.Value.(string)
		if !aok || !vok {
			return false
		}
		re, err := regexp.Compile(vs)
		if err != nil {
			return false
		}
		return re.MatchString(as)
	case "gt", "gte", "lt", "lte":
		a, aok := toNum(actual)
		b, bok := toNum(r.Value)
		if !aok || !bok {
			return false
		}
		switch r.Op {
		case "gt":
			return a > b
		case "gte":
			return a >= b
		case "lt":
			return a < b
		case "lte":
			return a <= b
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func evalGate(g gate, u User) bool {
	if enabled(g.Killswitch) {
		return false
	}
	if !enabled(g.Enabled) {
		return false
	}
	for _, r := range g.Rules {
		if !matchRule(r, u) {
			return false
		}
	}
	uid := userID(u)
	if uid == "" {
		// No unit id (an unidentified request before any anon id is minted): a
		// fully-rolled gate is on for everyone, so it can be answered without
		// bucketing; a fractional rollout genuinely needs a stable unit, so deny
		// until one exists. Rules above are still checked, so targeting wins.
		// See experiment-platform/18-identity-bucketing.md.
		return g.RolloutPct >= 10000
	}
	return Murmur3(g.Salt+":"+uid)%10000 < uint32(g.RolloutPct)
}

func evalExperiment(name string, exp *experiment, flags *flagsBlob, exps *expsBlob, u User, sticky StickyBucketStore) ExperimentResult {
	notIn := ExperimentResult{InExperiment: false, Group: "control"}
	if exp == nil || exp.Status != "running" {
		return notIn
	}
	if exp.TargetingGate != "" {
		if flags == nil {
			return notIn
		}
		g, ok := flags.Gates[exp.TargetingGate]
		if !ok || !evalGate(g, u) {
			return notIn
		}
	}
	uid := pickIdentifier(u, exp.BucketBy)
	if uid == "" {
		return notIn
	}
	if exp.Universe != "" && exps != nil {
		if uni, ok := exps.Universes[exp.Universe]; ok && len(uni.HoldoutRange) == 2 {
			seg := Murmur3(exp.Universe+":"+uid) % 10000
			if seg >= uint32(uni.HoldoutRange[0]) && seg <= uint32(uni.HoldoutRange[1]) {
				return notIn
			}
		}
	}

	// Sticky short-circuit (doc 20 §2): an enrolled unit whose stored salt
	// prefix still matches skips the allocation gate (so a shrinking allocation
	// keeps it in) and returns the stored group without re-running the pick. A
	// salt-prefix mismatch or a now-missing group falls through to re-bucket and
	// overwrite below.
	salt8 := saltPrefix(exp.Salt)
	if sticky != nil && name != "" {
		if entry, ok := sticky.Get(uid)[name]; ok && entry.S == salt8 {
			for _, g := range exp.Groups {
				if g.Name == entry.G {
					return ExperimentResult{InExperiment: true, Group: g.Name, Params: g.Params}
				}
			}
			// Stored group gone — fall through to re-bucket + overwrite.
		}
	}

	if Murmur3(exp.Salt+":alloc:"+uid)%10000 >= uint32(exp.AllocationPct) {
		return notIn
	}
	groupHash := Murmur3(exp.Salt+":group:"+uid) % 10000
	cumulative := uint32(0)
	for i, g := range exp.Groups {
		cumulative += uint32(g.Weight)
		if groupHash < cumulative || i == len(exp.Groups)-1 {
			if sticky != nil && name != "" {
				sticky.Set(uid, name, StickyEntry{G: g.Name, S: salt8})
			}
			return ExperimentResult{InExperiment: true, Group: g.Name, Params: g.Params}
		}
	}
	return notIn
}

// saltPrefix returns the first 8 bytes of the salt (the sticky reshuffle key).
// Salts shorter than 8 chars use the whole salt.
func saltPrefix(salt string) string {
	if len(salt) <= 8 {
		return salt
	}
	return salt[:8]
}
