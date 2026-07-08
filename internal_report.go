package shipeasy

// Internal self-monitoring channel — SDK bugs that are "on our end".
//
// When the SDK swallows one of its OWN internal errors (the recoverRead
// last-resort guard in client.go, which keeps a GetFlag/GetConfig/… from
// panicking into product code even when an internal invariant is violated), it
// ALSO ships a structured see event here — to Shipeasy's OWN project, NOT the
// consumer's — so the SDK team can track SDK-internal failures across every app
// the SDK runs in.
//
// This is deliberately distinct from the customer-facing See() path, which
// authenticates with the consumer's key and lands in the consumer's dashboard.
// Internal errors must never pollute a customer's Errors tab, and the SDK team
// must see them centrally — so this channel has its own baked-in destination +
// credential.
//
// Guarantees (identical to telemetry/see): fire-and-forget, never blocks, never
// panics into product code, deduped/rate-limited. A failed send is swallowed
// silently — it must never log (that would risk recursion through recoverRead).

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

// ---- Baked-in destination ----
//
// The main Shipeasy project (`.shipeasy` project). The credential is a PUBLIC
// client key — the same class of credential already embedded verbatim in every
// browser bundle that ships the client SDK, and mirroring how the CLI bakes
// Shipeasy's own public key for setup-bug self-reporting — so baking it into the
// published package is safe. /collect treats it as a write-only ingest key; it
// grants no read access. The canonical ingest host is api.shipeasy.ai (the SDK
// default baseURL), which routes /collect to the edge worker.
const internalIngestURL = "https://api.shipeasy.ai/collect"

// internalPlaceholderKey is the sentinel used until the real key is minted +
// baked. While internalIngestKey is still the placeholder the channel stays
// fully inert (see reportInternalError), so a build that ships before the key is
// provisioned never fires doomed requests. Mint the key with:
//
//	shipeasy keys create --type client --env prod \
//	  --name "SDK internal error self-reporting" --scopes events:write
//
// then replace the internalIngestKey initializer below with the returned value.
const internalPlaceholderKey = "sdk_client_REPLACE_WITH_SHIPEASY_INTERNAL_ERROR_KEY"

// internalIngestKey is the baked-in credential for the self-monitoring channel.
// SWAP THIS to the real minted public client key (see internalPlaceholderKey).
// While it equals the placeholder the channel is fully inert.
var internalIngestKey = "sdk_client_00bd4608a03e4084922978f9522614d5"

// internalReportOutcome is the fixed outcome half of the stable consequence. The
// label (the recoverRead operation name, e.g. "GetFlag") is the subject; the
// outcome is fixed. Both are constant per operation — no variable data — so
// occurrences of the same internal bug fold into one issue on our dashboard
// (fingerprint = error_type + normalized message + top stack + subject|outcome).
const internalReportOutcome = "returned a safe default"

// internalReportSDKID marks which language SDK reported the internal error.
const internalReportSDKID = "go"

// internalKeyConfigured reports whether a real key has been baked in (not the
// placeholder sentinel).
func internalKeyConfigured() bool {
	return internalIngestKey != "" && internalIngestKey != internalPlaceholderKey
}

// internalReportCtx carries the side + version + enable flag for the channel.
// Mirrors the TS module-level context: set once from NewEngine. Nil until
// configured — a report before configure is a no-op (nothing to attribute it to).
type internalReportCtx struct {
	sdkVersion string
	enabled    bool
}

var (
	internalReportMu      sync.Mutex
	internalReportContext *internalReportCtx
	internalReportLimiter = newSeeLimiter()
	// internalReportHTTP is the client used to POST the self-report. Separate
	// from any Engine's http client (this channel has its own destination +
	// credential). Overridable in tests via setInternalReportHTTPForTest.
	internalReportHTTP = &http.Client{Timeout: 10 * time.Second}
)

// setInternalReportContext wires the self-monitoring channel. Called from
// NewEngine with the bundle's version. enabled defaults on; it is forced off in
// localMode (test/offline — no network) and when the caller opts out via
// Options.DisableInternalErrorReporting.
func setInternalReportContext(sdkVersion string, enabled bool) {
	internalReportMu.Lock()
	internalReportContext = &internalReportCtx{sdkVersion: sdkVersion, enabled: enabled}
	internalReportMu.Unlock()
}

// reportInternalError reports an SDK-internal error to Shipeasy's own project.
// Called from recoverRead. label is the swallowed operation (e.g. "GetFlag") and
// becomes the stable issue subject. problem is the recovered panic value. Never
// panics.
func reportInternalError(label string, problem any) {
	defer func() { _ = recover() }() // self-reporting must never panic into product code

	internalReportMu.Lock()
	ctx := internalReportContext
	internalReportMu.Unlock()

	if ctx == nil || !ctx.enabled || !internalKeyConfigured() {
		return
	}

	// Reuse the SDK's own see-event builder so the fingerprint fields match the
	// customer see() path exactly. env is empty: internal reports carry only the
	// SDK-side context, never a consumer env/url. sdkVersion is overridden below
	// because buildSeeEvent stamps the package-global SDKVersion.
	ev := buildSeeEvent(problem, label, internalReportOutcome, map[string]any{"sdk": internalReportSDKID}, "")
	ev.SDKVersion = ctx.sdkVersion

	if internalReportLimiter != nil && !internalReportLimiter.shouldSend(ev) {
		return
	}

	body, err := json.Marshal(map[string]any{"events": []seeEvent{ev}})
	if err != nil {
		return
	}
	sendInternalReport(body)
}

// sendInternalReport fire-and-forgets the POST to the baked-in ingest. text/plain
// matches the SDK's existing /collect posts (the worker reads the raw body as
// JSON). Any failure — network, non-2xx — is swallowed silently.
func sendInternalReport(body []byte) {
	go func() {
		defer func() { _ = recover() }()
		req, err := http.NewRequest(http.MethodPost, internalIngestURL, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("X-SDK-Key", internalIngestKey)
		req.Header.Set("Content-Type", "text/plain")
		res, err := internalReportHTTP.Do(req)
		if err != nil {
			return
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}()
}

// ---- Test seams (mirror the TS __* seams) ----

// resetInternalReportForTest clears context + limiter + key so a spec starts from
// a clean, inert channel.
func resetInternalReportForTest() {
	internalReportMu.Lock()
	internalReportContext = nil
	internalReportLimiter = newSeeLimiter()
	internalIngestKey = internalPlaceholderKey
	internalReportHTTP = &http.Client{Timeout: 10 * time.Second}
	internalReportMu.Unlock()
}

// setInternalIngestKeyForTest stands in a real-looking key so specs can exercise
// the send path without the (deliberately inert) placeholder blocking it.
func setInternalIngestKeyForTest(key string) {
	internalReportMu.Lock()
	internalIngestKey = key
	internalReportMu.Unlock()
}

// setInternalReportHTTPForTest injects an http.Client whose transport captures
// the outbound request.
func setInternalReportHTTPForTest(hc *http.Client) {
	internalReportMu.Lock()
	internalReportHTTP = hc
	internalReportMu.Unlock()
}
