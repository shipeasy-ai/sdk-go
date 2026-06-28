# Shipeasy Go SDK — Overview

`github.com/shipeasy-ai/sdk-go` is the **server-side** Go SDK for
[Shipeasy](https://shipeasy.dev): feature flags ("gates"), dynamic configs,
kill switches, A/B experiments, metric tracking, and structured error reporting.
Evaluation is **local** against a cached copy of the edge blobs — there is no
network call on the hot path.

## Quickstart

```go
package main

import (
    "os"

    shipeasy "github.com/shipeasy-ai/sdk-go"
)

func main() {
    // 1) Once, at process start. The api key lives here.
    shipeasy.Configure(shipeasy.Options{
        APIKey: os.Getenv("SHIPEASY_SERVER_KEY"),
    })

    // 2) Per request: bind the user once, then call with NO user argument.
    c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123", "plan": "pro"})
    if c.GetFlag("new_checkout") {
        // new behaviour
    }
}
```

## Mental model: configure once, bind per request

The whole SDK is exactly two calls:

1. **`Configure(Options{...})`** — call it **once** at process start. The api key
   lives here, and it kicks off a background fetch so the first read resolves
   against real rules. It is first-config-wins (idempotent).
2. **`NewClient(user)`** — a cheap, user-bound handle you build per request. It
   carries no api key and opens no connection; every read is local. Its methods
   take **no** user argument (the user is already bound):
   `GetFlag`, `GetFlagOr`, `GetFlagDetail`, `GetConfig`, `GetConfigOr`,
   `GetExperiment`, `GetKillswitch`, plus `Track(event, props)` and
   `LogExposure(experiment)`.

So an experiment is end-to-end Client-only: bind → `GetExperiment` → `Track`.

```go
c := shipeasy.NewClient(acct)            // acct is your own *Account
if c.GetFlag("new_checkout") { /* ... */ }
```

If you don't supply an `Attributes` transform (see [Installation](installation.md)),
the value you pass to `NewClient` is assumed to already BE the attribute map, so
`shipeasy.NewClient(shipeasy.User{"user_id": "u_123", "plan": "pro"})` works
as-is. `NewClient` **panics** if `Configure` was not called first (the api key
lives in the global config — failing loudly surfaces the misconfiguration).

## Feature pages

- [Installation](installation.md) — `go get`, per-framework wiring, the global `Configure()` call + options table.
- [Configuration](configuration.md) — `Configure` options, env vars, init/poll vs one-shot, change listeners.
- [Flags](flags.md) — `GetFlag`, `GetFlagOr`, `GetFlagDetail`.
- [Configs](configs.md) — `GetConfig`, `GetConfigOr`.
- [Kill switches](killswitches.md) — `GetKillswitch`, named switches.
- [Experiments](experiments.md) — `GetExperiment`, `ExperimentResult`, `Track`, `LogExposure`.
- [i18n](i18n.md) — server SSR loader tag + the browser SDK's `t()`.
- [Error reporting](error-reporting.md) — the `See()` surface.
- [Testing](testing.md) — `ConfigureForTesting`, `ConfigureForOffline`, the `Override*` helpers.
- [OpenFeature](openfeature.md) — `NewGlobalProvider()`.
- [Advanced](advanced.md) — manual exposure, private attributes, bucketBy, sticky bucketing, anon-id middleware.
