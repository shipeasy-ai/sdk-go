package shipeasy

import (
	"os"
	"strings"
)

// Native runtime-environment detection. Mirrors the TypeScript reference SDK's
// src/env.ts.
//
// Used ONLY to pick the DEFAULT for outbound egress when the caller does not set
// it explicitly:
//   - is the SDK allowed to make network requests at all (Options.IsNetworkEnabled)?
//   - is per-evaluation usage telemetry / logging allowed (Options.IsTrackingEnabled)?
//
// Both default to ON in production and OFF everywhere else, so a local/dev/CI run
// of an app that embeds the SDK never phones home unless it explicitly opts in.
//
// Precedence for the production decision:
//  1. A native runtime env var, checked in order: SHIPEASY_ENV, then APP_ENV,
//     then GO_ENV, then ENV. A value of "production"/"prod" (case-insensitive) ⇒
//     prod; any other present value ("development"/"staging"/"test"/…) ⇒ not prod.
//  2. When NO native env var is set (common on serverless / containers that don't
//     export one), fall back to the SDK's own configured Env option (which the
//     caller sets and which itself defaults to "prod"). This keeps a real
//     production deploy "on" by default while an Env: "dev" config stays quiet.
//
// The Env option is always present (it defaults to "prod"), so the production
// decision is always inferable — the SDK never has to make the field required.

// nativeEnvVars is the ordered list of runtime env vars consulted before falling
// back to the SDK's own Env option. Go has no framework-wide env convention, so
// SHIPEASY_ENV wins, then the common APP_ENV / GO_ENV / ENV names.
var nativeEnvVars = []string{"SHIPEASY_ENV", "APP_ENV", "GO_ENV", "ENV"}

// isProductionEnv reports whether the host runtime looks like a production
// deployment. configuredEnv is the SDK's own Env option (dev/staging/prod); it is
// consulted only when no native runtime env var is set.
func isProductionEnv(configuredEnv string) bool {
	if native, ok := readNativeEnv(); ok {
		return native == "production" || native == "prod"
	}
	v := strings.ToLower(strings.TrimSpace(configuredEnv))
	if v == "" {
		v = "prod"
	}
	return v == "prod" || v == "production"
}

// readNativeEnv returns the first set-and-non-empty native env var (lowercased,
// trimmed) and true, or ("", false) when none of them are set.
func readNativeEnv() (string, bool) {
	for _, name := range nativeEnvVars {
		raw := os.Getenv(name)
		v := strings.ToLower(strings.TrimSpace(raw))
		if v != "" {
			return v, true
		}
	}
	return "", false
}
