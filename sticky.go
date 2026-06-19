package shipeasy

import "sync"

// StickyEntry is one persisted sticky assignment for a (unit, experiment) pair:
// the chosen group plus the 8-char salt prefix that minted it. A salt change on
// the experiment moves the prefix, which is the reshuffle key — a stored entry
// whose S no longer matches the live salt prefix is ignored and re-bucketed.
type StickyEntry struct {
	G string `json:"g"` // group name
	S string `json:"s"` // 8-char salt prefix that produced G
}

// StickyBucketStore is a pluggable sticky-bucketing store for the server (doc 20
// §2). It is keyed by the bucketing unit (the pickIdentifier-resolved id); the
// value is that unit's per-experiment assignments. When no store is supplied to
// the client, experiment assignment is purely deterministic (today's behaviour).
//
// Implementations must be safe for concurrent use — eval reads and writes the
// store from request goroutines.
type StickyBucketStore interface {
	// Get returns the unit's per-experiment assignments (keyed by experiment
	// name), or nil if the unit has none.
	Get(unit string) map[string]StickyEntry
	// Set records (or overwrites) the assignment for one (unit, experiment).
	Set(unit, experiment string, entry StickyEntry)
}

// inMemoryStickyStore is a process-local, Map-backed StickyBucketStore. Handy
// for tests and single-process servers; not shared across instances.
type inMemoryStickyStore struct {
	mu sync.RWMutex
	m  map[string]map[string]StickyEntry
}

// NewInMemoryStickyStore returns a process-local sticky store. An optional seed
// preloads assignments (the outer key is the unit, the inner key the experiment
// name); the store copies the seed so the caller's map is not retained.
func NewInMemoryStickyStore(seed ...map[string]map[string]StickyEntry) StickyBucketStore {
	m := map[string]map[string]StickyEntry{}
	if len(seed) > 0 && seed[0] != nil {
		for unit, exps := range seed[0] {
			inner := map[string]StickyEntry{}
			for exp, e := range exps {
				inner[exp] = e
			}
			m[unit] = inner
		}
	}
	return &inMemoryStickyStore{m: m}
}

func (s *inMemoryStickyStore) Get(unit string) map[string]StickyEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, ok := s.m[unit]
	if !ok {
		return nil
	}
	// Return a copy so callers can't mutate the store's internal map.
	out := make(map[string]StickyEntry, len(entries))
	for k, v := range entries {
		out[k] = v
	}
	return out
}

func (s *inMemoryStickyStore) Set(unit, experiment string, entry StickyEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inner, ok := s.m[unit]
	if !ok {
		inner = map[string]StickyEntry{}
		s.m[unit] = inner
	}
	inner[experiment] = entry
}
