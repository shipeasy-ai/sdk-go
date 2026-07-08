package shipeasy

import (
	"os"
	"testing"
)

// TestMain is the package-wide test bootstrap. It forces the internal
// self-monitoring ingest key back to the inert placeholder BEFORE any test runs,
// so the whole suite defaults to an inert channel.
//
// Why this is required: the baked-in internalIngestKey is a REAL production
// public client key. NewEngine (client.go) calls setInternalReportContext with
// enabled=true by default, so any test that constructs an engine via NewEngine
// and then trips recoverRead (e.g. the fail-safe / never-panics tests that wire
// an adversarial panicking sticky store into a running experiment) would call
// reportInternalError with the channel enabled AND keyConfigured() true —
// firing a real POST to https://api.shipeasy.ai/collect and polluting
// Shipeasy's own Errors dashboard from CI.
//
// Resetting internalIngestKey to internalPlaceholderKey here makes
// internalKeyConfigured() false, which gates every send in reportInternalError,
// for the entire run and for any single test run in isolation.
//
// The internal-report unit tests (internal_report_test.go) intentionally stand
// in their own fake key via setInternalIngestKeyForTest and inject a stubbed
// HTTP transport (no real network), then reset to the placeholder via
// t.Cleanup(resetInternalReportForTest) — so this global default does not break
// them; it only ensures every OTHER test sees an inert channel.
func TestMain(m *testing.M) {
	internalIngestKey = internalPlaceholderKey
	os.Exit(m.Run())
}
