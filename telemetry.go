package shipeasy

// Per-evaluation usage telemetry. Fires one fire-and-forget HTTP beacon per
// evaluation so usage is counted by Cloudflare's native per-path analytics.
// Mirrors the contract in the TypeScript reference SDK and
// experiment-platform/15-usage-metering.md. The path carries sha256(apiKey) —
// never the raw key — plus side/env, then feature/resource. A long-lived Go
// process emits reliably; the 2s dedup window bounds volume under loops.

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultTelemetryURL = "https://t.shipeasy.ai"

type telemetry struct {
	prefix   string // <endpoint>/t/<hash>/<side>/<env>
	disabled bool
	dedupeMs int64
	http     *http.Client
	mu       sync.Mutex
	last     map[string]int64
}

func newTelemetry(endpoint, sdkKey, side, env string, disabled bool, hc *http.Client) *telemetry {
	endpoint = strings.TrimRight(endpoint, "/")
	t := &telemetry{
		disabled: disabled || sdkKey == "" || endpoint == "",
		dedupeMs: 2000,
		http:     hc,
		last:     map[string]int64{},
	}
	if !t.disabled {
		sum := sha256.Sum256([]byte(sdkKey))
		t.prefix = endpoint + "/t/" + hex.EncodeToString(sum[:]) + "/" + side + "/" + url.PathEscape(env)
	}
	return t
}

// emit fires a best-effort usage beacon for one evaluation. Never blocks the
// caller (the goroutine owns the request) and never affects evaluation.
func (t *telemetry) emit(feature, resource string) {
	if t.disabled {
		return
	}
	if t.dedupeMs > 0 {
		key := feature + "/" + resource
		now := time.Now().UnixMilli()
		t.mu.Lock()
		if last, ok := t.last[key]; ok && now-last < t.dedupeMs {
			t.mu.Unlock()
			return
		}
		t.last[key] = now
		t.mu.Unlock()
	}
	u := t.prefix + "/" + feature + "/" + url.PathEscape(resource)
	go func() {
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			return
		}
		res, err := t.http.Do(req)
		if err != nil {
			return
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}()
}
