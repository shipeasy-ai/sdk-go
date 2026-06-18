package shipeasy

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Offline file data source. Lets a process run evaluations against a captured
// snapshot of the edge blobs with ZERO network — no key, no polling, no
// telemetry. The snapshot file is JSON of the shape:
//
//	{
//	  "flags":       <verbatim body of GET /sdk/flags>,
//	  "experiments": <verbatim body of GET /sdk/experiments>
//	}
//
// i.e. the two blobs exactly as the edge serves them. Evaluations run the same
// canonical evaluator as a live client; Override* setters still apply on top.

type offlineSnapshot struct {
	Flags       json.RawMessage `json:"flags"`
	Experiments json.RawMessage `json:"experiments"`
}

// NewOfflineClient reads a snapshot file (see offlineSnapshot) and returns a
// no-network client preloaded with both blobs. Init/InitOnce/Track are no-ops
// (localMode), the client is already initialized, and telemetry is disabled.
func NewOfflineClient(path string) (*Client, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("shipeasy: read offline snapshot %q: %w", path, err)
	}
	var snap offlineSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, fmt.Errorf("shipeasy: parse offline snapshot %q: %w", path, err)
	}
	c := newOfflineBase()
	if len(snap.Flags) > 0 {
		var fb flagsBlob
		if err := json.Unmarshal(snap.Flags, &fb); err != nil {
			return nil, fmt.Errorf("shipeasy: parse offline flags blob: %w", err)
		}
		c.flags = &fb
	}
	if len(snap.Experiments) > 0 {
		var eb expsBlob
		if err := json.Unmarshal(snap.Experiments, &eb); err != nil {
			return nil, fmt.Errorf("shipeasy: parse offline experiments blob: %w", err)
		}
		c.exps = &eb
	}
	return c, nil
}

// NewOfflineClientFromSnapshot builds a no-network client from in-memory blobs.
// flags and experiments are the parsed bodies of /sdk/flags and
// /sdk/experiments respectively (any value json.Marshal can round-trip into the
// internal blob shapes). Either may be nil. Init/InitOnce/Track are no-ops.
func NewOfflineClientFromSnapshot(flags, experiments any) *Client {
	c := newOfflineBase()
	if flags != nil {
		if b, err := json.Marshal(flags); err == nil {
			var fb flagsBlob
			if json.Unmarshal(b, &fb) == nil {
				c.flags = &fb
			}
		}
	}
	if experiments != nil {
		if b, err := json.Marshal(experiments); err == nil {
			var eb expsBlob
			if json.Unmarshal(b, &eb) == nil {
				c.exps = &eb
			}
		}
	}
	return c
}

// newOfflineBase returns a localMode, initialized, telemetry-off client with no
// blobs loaded.
func newOfflineBase() *Client {
	return &Client{
		baseURL:      defaultBaseURL,
		pollInterval: 30 * time.Second,
		stop:         make(chan struct{}),
		once:         sync.Once{},
		telemetry:    newTelemetry("", "", "server", "prod", true, nil),
		localMode:    true,
		initialized:  true,
	}
}
