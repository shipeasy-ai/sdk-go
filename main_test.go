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
// It also declares the test suite production-equivalent for EGRESS by setting
// SHIPEASY_ENV=production before any test runs. The environment-derived egress
// defaults (see env.go / Options.IsNetworkEnabled) turn the SDK OFF by default
// outside production; without this the whole suite would run in localMode (no
// fetch, no Track, no telemetry) and every test that exercises a network path
// would break. Tests that specifically assert the dev/prod branching override
// SHIPEASY_ENV locally with t.Setenv (see env_test.go).
func TestMain(m *testing.M) {
	internalIngestKey = internalPlaceholderKey
	_ = os.Setenv("SHIPEASY_ENV", "production")
	os.Exit(m.Run())
}
