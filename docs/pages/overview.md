# Shipeasy Go SDK — Overview

`github.com/shipeasy-ai/sdk-go` is the **server-side** Go SDK for
[Shipeasy](https://shipeasy.dev): feature flags ("gates"), dynamic configs,
kill switches, A/B experiments, metric tracking, and structured error reporting.
Evaluation is **local** against a cached copy of the edge blobs — there is no
network call on the hot path.

## Mental model: configure once, bind per request

```go
import shipeasy "github.com/shipeasy-ai/sdk-go"

// 1) Once, at process start. The api key lives here.
shipeasy.Configure(shipeasy.Options{
    APIKey: os.Getenv("SHIPEASY_SERVER_KEY"),
    // Optional: map YOUR user type → the Shipeasy attribute map.
    Attributes: func(u any) shipeasy.User {
        acct := u.(*Account)
        return shipeasy.User{"user_id": acct.ID, "plan": acct.Plan}
    },
})

// 2) Per request: bind the user once, call with NO user argument.
c := shipeasy.NewClient(acct)            // acct is your own *Account
if c.GetFlag("new_checkout") { /* ... */ }
```

If you don't supply an `Attributes` transform, the value you pass to
`NewClient` is assumed to already BE the attribute map, so
`shipeasy.NewClient(shipeasy.User{"user_id": "u_123", "plan": "pro"})` works
as-is. `NewClient` **panics** if `Configure` was not called first (the api key
lives in the global config — failing loudly surfaces the misconfiguration).

## Engine vs Client

There are two layers:

- **`Engine`** — the heavyweight type that owns the api key, the blob cache, the
  poll timer, telemetry, and the `See()` error surface. `Configure` builds one
  shared `Engine` for the process. Its methods take an explicit `user` argument
  (e.g. `eng.GetFlag("new_checkout", user)`). Reach for it directly when you need
  `Init`, `Track`, `OnChange`, the `Override*` setters, or multiple engines in
  one process.
- **`Client`** — the lightweight, user-bound handle returned by
  `NewClient(user)`. It carries no api key, opens no connection, runs no poll
  timer; it delegates every evaluation to the single `Engine` with the bound
  attribute map. Build one per request — it is cheap. Its methods take **no**
  `user` argument (the user is already bound): `GetFlag`, `GetFlagOr`,
  `GetFlagDetail`, `GetConfig`, `GetConfigOr`, `GetExperiment`, `GetKillswitch`,
  plus `Track(event, props)` and `LogExposure(experiment)` — so an experiment is
  end-to-end Client-only (bind → `GetExperiment` → `Track`).

> Breaking change in 0.8.0: the heavyweight type formerly named `Client` is now
> `Engine`, and `NewClient(Options)` is now `NewEngine(Options)`. The name
> `Client` is now the lightweight user-bound handle built with `NewClient(user)`.

## Feature pages

- [Installation](installation.md) — `go get`, min Go version, import line.
- [Configuration](configuration.md) — `Configure` / `NewEngine`, options, env vars, init/poll.
- [Flags](flags.md) — `GetFlag`, `GetFlagOr`, `GetFlagDetail`.
- [Configs](configs.md) — `GetConfig`, `GetConfigOr`.
- [Kill switches](killswitches.md) — `GetKillswitch`.
- [Experiments](experiments.md) — `GetExperiment`, `ExperimentResult`, `Client.Track`, `Client.LogExposure`.
- [i18n](i18n.md) — server SSR loader tag + the browser SDK's `t()`.
- [Error reporting](error-reporting.md) — the `See()` surface.
- [Testing](testing.md) — `NewTestClient`, offline clients, `Override*`.
- [OpenFeature](openfeature.md) — the `shipeasyopenfeature.Provider`.
- [Advanced](advanced.md) — manual exposure, private attributes, bucketBy, sticky bucketing, anon-id middleware.
