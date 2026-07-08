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

	// --- Universe-first (mutual-exclusion pool) fields, all optional. ---

	// HoldoutGate is a gate name checked after the universe holdout carve-out:
	// when it passes for the unit, the unit is held out (never assigned).
	HoldoutGate string `json:"holdoutGate"`
	// PoolOffsetBp / PoolSizeBp describe the slice of the universe's shared
	// [0,10000) segment this experiment claims. Used only when HashVersion >= 2
	// (pooled allocation, real mutual exclusion): the unit's universe segment
	// must fall in [offset, offset+size). A nil pointer means "no slice".
	PoolOffsetBp *int `json:"poolOffsetBp"`
	PoolSizeBp   *int `json:"poolSizeBp"`
	// ReservedHeadroomBp is a tail of [0,10000) in the group-split space left
	// unassigned so a future variant can absorb it (§B5). Clamped to [0,10000].
	ReservedHeadroomBp *int `json:"reservedHeadroomBp"`
	// HashVersion >= 2 enables pooled allocation (with a slice). Absent/1 falls
	// back to the legacy independent per-experiment allocation salt.
	HashVersion *int `json:"hashVersion"`
}

type group struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
	Params any    `json:"params"`
}

type universe struct {
	HoldoutRange []int `json:"holdout_range"`
	// ParamSchema is the universe's config schema: the single source of truth for
	// param names, types, and defaults. assign() layers these under an assigned
	// variant's overrides so an unset param still returns the universe default.
	ParamSchema []universeParam `json:"param_schema"`
}

// universeParam is one entry in a universe's param_schema.
type universeParam struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default any    `json:"default"`
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

// ---- Universe assignment (mutual-exclusion pool eval) ----

// paramDefaultsFromSchema flattens a universe param schema to a plain
// name -> default map — the defaults assign() layers under a variant's override
// map (§B2). Returns nil for a nil/empty schema so the merge short-circuits.
// Mirrors @shipeasy/core.
func paramDefaultsFromSchema(schema []universeParam) map[string]any {
	if len(schema) == 0 {
		return nil
	}
	out := make(map[string]any, len(schema))
	for _, p := range schema {
		out[p.Name] = p.Default
	}
	return out
}

// mergeParams returns universeDefaults ⊕ variantOverride: a variant inherits
// every universe default it doesn't explicitly override (§B2). groupParams, when
// a map, wins per key. A non-map groupParams (or nil) with no defaults is
// returned as-is (legacy shape preserved).
func mergeParams(paramDefaults map[string]any, groupParams any) any {
	if paramDefaults == nil {
		return groupParams
	}
	out := make(map[string]any, len(paramDefaults))
	for k, v := range paramDefaults {
		out[k] = v
	}
	if gp, ok := groupParams.(map[string]any); ok {
		for k, v := range gp {
			out[k] = v
		}
	}
	return out
}

// expState is a unit's standing in one experiment: an assigned group (state
// "group", with merged params), "holdout" (universe carve-out or holdout gate —
// never assigned), or "out".
type expState int

const (
	stateOut expState = iota
	stateHoldout
	stateGroup
)

// expStanding is the result of classifyExperiment: for stateGroup, Group is the
// assigned variant and Params is ALREADY merged (universeDefaults ⊕ variant).
type expStanding struct {
	State  expState
	Group  string
	Params any
}

// classifyExperiment is the single local mirror of @shipeasy/core's
// classifyExperiment (doc 20 §B): targeting → universe holdout → holdout gate →
// sticky → allocation (pooled or legacy) → weighted group split. evalGate is a
// gate-name → boolean lookup over the flags blob so the two gate checks reuse the
// SDK's real gate evaluation. holdoutRange is the universe carve-out (nil when
// absent); paramDefaults are the universe param defaults merged under every
// returned group's params.
func classifyExperiment(
	name string,
	exp *experiment,
	u User,
	holdoutRange []int,
	paramDefaults map[string]any,
	evalGateFn func(string) bool,
	sticky StickyBucketStore,
) expStanding {
	asGroup := func(g group) expStanding {
		return expStanding{State: stateGroup, Group: g.Name, Params: mergeParams(paramDefaults, g.Params)}
	}

	if exp.TargetingGate != "" && !evalGateFn(exp.TargetingGate) {
		return expStanding{State: stateOut}
	}

	uid := pickIdentifier(u, exp.BucketBy)
	if uid == "" {
		return expStanding{State: stateOut}
	}

	// One segment in the universe's shared [0,10000) hash space. The holdout
	// carve-out AND every experiment's pool slice are disjoint ranges of THIS
	// segment — that's what makes "held out / taken / free" a real partition.
	universeSeg := Murmur3(exp.Universe+":"+uid) % 10000

	if len(holdoutRange) == 2 {
		if universeSeg >= uint32(holdoutRange[0]) && universeSeg <= uint32(holdoutRange[1]) {
			return expStanding{State: stateHoldout}
		}
	}

	if exp.HoldoutGate != "" && evalGateFn(exp.HoldoutGate) {
		return expStanding{State: stateHoldout}
	}

	// Sticky short-circuit (doc 20 §2): an enrolled unit whose stored salt prefix
	// still matches skips the allocation gate (so a shrinking allocation keeps it
	// in) and returns the stored group. A salt-prefix mismatch or a now-missing
	// group falls through to re-bucket + overwrite below.
	salt8 := saltPrefix(exp.Salt)
	if sticky != nil && name != "" {
		if entry, ok := sticky.Get(uid)[name]; ok && entry.S == salt8 {
			for _, g := range exp.Groups {
				if g.Name == entry.G {
					return asGroup(g)
				}
			}
		}
	}

	// Allocation. Pooled (hashVersion >= 2 with a slice) gives real mutual
	// exclusion: the unit's universe segment must fall in the claimed range.
	// Legacy falls back to an independent per-experiment salt so siblings overlap.
	pooled := exp.HashVersion != nil && *exp.HashVersion >= 2 &&
		exp.PoolOffsetBp != nil && exp.PoolSizeBp != nil && *exp.PoolSizeBp > 0
	if pooled {
		lo := uint32(*exp.PoolOffsetBp)
		hi := lo + uint32(*exp.PoolSizeBp)
		if universeSeg < lo || universeSeg >= hi {
			return expStanding{State: stateOut}
		}
	} else {
		if Murmur3(exp.Salt+":alloc:"+uid)%10000 >= uint32(exp.AllocationPct) {
			return expStanding{State: stateOut}
		}
	}

	// Group split over [0, usable) where usable = 10000 − reserved; a unit in the
	// reserved tail is left unassigned so an appended variant can absorb it (§B5).
	reserved := 0
	if exp.ReservedHeadroomBp != nil {
		reserved = *exp.ReservedHeadroomBp
	}
	if reserved < 0 {
		reserved = 0
	}
	if reserved > 10000 {
		reserved = 10000
	}
	usable := uint32(10000 - reserved)
	groupHash := Murmur3(exp.Salt+":group:"+uid) % 10000
	if groupHash >= usable {
		return expStanding{State: stateOut}
	}
	cumulative := uint32(0)
	for i, g := range exp.Groups {
		cumulative += uint32(g.Weight)
		if groupHash < cumulative || i == len(exp.Groups)-1 {
			if sticky != nil && name != "" {
				sticky.Set(uid, name, StickyEntry{G: g.Name, S: salt8})
			}
			return asGroup(g)
		}
	}
	return expStanding{State: stateOut}
}

// classifyOne runs the full override → classify pipeline for one experiment,
// keyed by name. It resolves the universe's holdout range + param defaults, wires
// a gate-lookup closure over the flags blob, and returns the unit's standing.
// override, when present, short-circuits to stateGroup with merged params.
func classifyOne(name string, exp *experiment, flags *flagsBlob, exps *expsBlob, u User, sticky StickyBucketStore, override *ExperimentResult) expStanding {
	var paramDefaults map[string]any
	var holdoutRange []int
	if exps != nil && exp != nil {
		if uni, ok := exps.Universes[exp.Universe]; ok {
			paramDefaults = paramDefaultsFromSchema(uni.ParamSchema)
			if len(uni.HoldoutRange) == 2 {
				holdoutRange = uni.HoldoutRange
			}
		}
	}
	if override != nil {
		return expStanding{State: stateGroup, Group: override.Group, Params: mergeParams(paramDefaults, override.Params)}
	}
	if exp == nil || exp.Status != "running" {
		return expStanding{State: stateOut}
	}
	evalGateFn := func(gname string) bool {
		if flags == nil {
			return false
		}
		g, ok := flags.Gates[gname]
		if !ok {
			return false
		}
		return evalGate(g, u)
	}
	return classifyExperiment(name, exp, u, holdoutRange, paramDefaults, evalGateFn, sticky)
}

// evalExperiment is the experiment-keyed classify entry point retained for the
// cross-language eval-vectors parity test (testdata/eval-vectors.json) and the
// internal override seam. The public read path is Universe(name).Assign(user);
// this returns the legacy ExperimentResult shape (InExperiment/Group/Params) for
// one experiment. A returned "holdout" or "out" standing maps to not-enrolled.
func evalExperiment(name string, exp *experiment, flags *flagsBlob, exps *expsBlob, u User, sticky StickyBucketStore) ExperimentResult {
	st := classifyOne(name, exp, flags, exps, u, sticky, nil)
	if st.State == stateGroup {
		return ExperimentResult{InExperiment: true, Group: st.Group, Params: st.Params}
	}
	return ExperimentResult{InExperiment: false, Group: "control"}
}

// saltPrefix returns the first 8 bytes of the salt (the sticky reshuffle key).
// Salts shorter than 8 chars use the whole salt.
func saltPrefix(salt string) string {
	if len(salt) <= 8 {
		return salt
	}
	return salt[:8]
}
