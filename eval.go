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
	Enabled    any   `json:"enabled"`
	Killswitch any   `json:"killswitch"`
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
	Gates   map[string]gate                  `json:"gates"`
	Configs map[string]struct{ Value any `json:"value"` } `json:"configs"`
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
		return false
	}
	return Murmur3(g.Salt+":"+uid)%10000 < uint32(g.RolloutPct)
}

func evalExperiment(exp *experiment, flags *flagsBlob, exps *expsBlob, u User) ExperimentResult {
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
	uid := userID(u)
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
	if Murmur3(exp.Salt+":alloc:"+uid)%10000 >= uint32(exp.AllocationPct) {
		return notIn
	}
	groupHash := Murmur3(exp.Salt+":group:"+uid) % 10000
	cumulative := uint32(0)
	for i, g := range exp.Groups {
		cumulative += uint32(g.Weight)
		if groupHash < cumulative || i == len(exp.Groups)-1 {
			return ExperimentResult{InExperiment: true, Group: g.Name, Params: g.Params}
		}
	}
	return notIn
}
