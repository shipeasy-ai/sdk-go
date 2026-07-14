package shipeasy

import "sort"

// Assignment is the result of universe(name).Assign(user) — a unit's standing in
// a universe. A universe is a mutual-exclusion pool, so a unit lands in AT MOST
// ONE experiment. Never throws: an un-enrolled unit still resolves Get() to the
// universe defaults (or your fallback).
//
// Exposure is logged ON READ (spec step 7): the single exposure fires the first
// time an enrolled unit's param is actually read via Get(), not at Assign() time
// — so an assignment that is computed but never read logs nothing. Deduped per
// process; the durable per-(unit, experiment, group) dedup lives server-side.
// Use Peek() to read without logging.
type Assignment struct {
	// Name is the experiment the unit landed in, or "" when not enrolled.
	Name string
	// Group is the assigned variant/group name, or "" when not enrolled.
	Group string
	// Enrolled is true iff the unit is enrolled in an experiment in this universe.
	Enrolled bool
	// params is already merged (universeDefaults ⊕ variantOverride) when enrolled;
	// defaults-only (or empty) when not.
	params map[string]any
	// onExpose fires the single exposure the first time an enrolled param is
	// read; nil when not enrolled (nothing to expose). Deduped downstream.
	onExpose func()
	// exposed is a shared flag (pointer so value-copies of the Assignment share
	// it) that guards onExpose to fire at most once per handle.
	exposed *bool
}

// Get reads a resolved param: the assigned variant's override, else the universe
// default, else fallback. Works even when not enrolled (the variant layer is
// absent, so you get universeDefault ?? fallback). The first enrolled read logs
// the single exposure; use Peek to read without logging.
func (a Assignment) Get(field string, fallback any) any {
	// On-read exposure: the first param read of an enrolled assignment logs one
	// exposure. Reading Enrolled/Name/Group does NOT log — only Get does.
	if a.onExpose != nil && a.exposed != nil && !*a.exposed {
		*a.exposed = true
		a.onExpose()
	}
	return a.lookup(field, fallback)
}

// Peek reads a resolved param WITHOUT logging an exposure — the read-only
// counterpart to Get (spec step 7 opt-out). Same lookup semantics as Get.
func (a Assignment) Peek(field string, fallback any) any {
	return a.lookup(field, fallback)
}

func (a Assignment) lookup(field string, fallback any) any {
	if a.params != nil {
		if v, ok := a.params[field]; ok {
			return v
		}
	}
	return fallback
}

// UniverseHandle is a reusable handle bound to one universe on an Engine. Get it
// from Engine.Universe(name); call Assign(user) to pick the <=1 experiment the
// unit is pooled into (auto-logging a single deduped exposure when enrolled).
type UniverseHandle struct {
	engine *Engine
	name   string
}

// Universe returns a reusable handle bound to universe name:
// engine.Universe("checkout").Assign(user). This is the sole experiment read
// path — a caller asks a universe, not an experiment. See Assign.
func (c *Engine) Universe(name string) *UniverseHandle {
	return &UniverseHandle{engine: c, name: name}
}

// Assign assigns user within the bound universe. A universe is a
// mutual-exclusion pool, so the unit lands in AT MOST ONE experiment; the
// returned Assignment exposes the variant + resolved params and auto-logs a
// single exposure when enrolled. An un-enrolled unit still resolves Get() to the
// universe defaults. Never panics into the caller.
func (h *UniverseHandle) Assign(user User) (result Assignment) {
	defer recoverRead(h.engine, "Universe.Assign", &result)
	return h.engine.assignUniverse(h.name, user)
}

// assignUniverse is the engine-level universe assignment. See UniverseHandle.Assign.
func (c *Engine) assignUniverse(universeName string, user User) Assignment {
	c.telemetry.emit("experiment", universeName)

	c.mu.RLock()
	flags := c.flags
	exps := c.exps
	sticky := c.stickyStore
	var expOv map[string]ExperimentResult
	if len(c.expOverrides) > 0 {
		expOv = make(map[string]ExperimentResult, len(c.expOverrides))
		for k, v := range c.expOverrides {
			expOv[k] = v
		}
	}
	c.mu.RUnlock()

	var paramDefaults map[string]any
	if exps != nil {
		if uni, ok := exps.Universes[universeName]; ok {
			paramDefaults = paramDefaultsFromSchema(uni.ParamSchema)
		}
	}
	notEnrolled := Assignment{params: paramDefaults}
	if notEnrolled.params == nil {
		notEnrolled.params = map[string]any{}
	}
	if exps == nil {
		return notEnrolled
	}

	// Candidate running experiments in this universe. Deterministic order:
	// pool-slice offset asc (slices are disjoint so <=1 matches under pooling),
	// then name. A universe-held-out or unallocated unit falls through to the
	// defaults-only handle.
	type cand struct {
		name string
		exp  experiment
	}
	candidates := make([]cand, 0)
	for name, e := range exps.Experiments {
		if e.Universe == universeName && e.Status == "running" {
			candidates = append(candidates, cand{name: name, exp: e})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		oi, oj := 0, 0
		if candidates[i].exp.PoolOffsetBp != nil {
			oi = *candidates[i].exp.PoolOffsetBp
		}
		if candidates[j].exp.PoolOffsetBp != nil {
			oj = *candidates[j].exp.PoolOffsetBp
		}
		if oi != oj {
			return oi < oj
		}
		return candidates[i].name < candidates[j].name
	})

	for _, cd := range candidates {
		exp := cd.exp
		var ov *ExperimentResult
		if r, ok := expOv[cd.name]; ok {
			ov = &r
		}
		st := classifyOne(cd.name, &exp, flags, exps, user, sticky, ov)
		if st.State == stateGroup {
			params, _ := st.Params.(map[string]any)
			if params == nil {
				params = map[string]any{}
			}
			// On-read exposure (spec step 7): defer the single exposure to the
			// first param read via onExpose, instead of firing it here at assign
			// time.
			name, group := cd.name, st.Group
			exposed := false
			return Assignment{
				Name:     name,
				Group:    group,
				Enrolled: true,
				params:   params,
				exposed:  &exposed,
				onExpose: func() { c.postExposure(user, name, group) },
			}
		}
		// "holdout"/"out": try the next candidate — under pooling only one slice
		// can match, so the loop naturally lands on the winner (or falls through).
	}
	return notEnrolled
}
