---
name: shipeasy-go
description: Use Shipeasy (feature flags, configs, kill switches, A/B experiments, i18n) from Go. Covers Configure() + NewClient(user), GetFlag/GetConfig/Universe().Assign()/GetKillswitch, Track, testing, OpenFeature.
---

# Shipeasy Go SDK

Server-side Go SDK for Shipeasy. Evaluation is local against a cached blob — no
network on the hot path. Min Go 1.21.

> The documented surface is exactly **`Configure()`** (setup) and the bound
> **`NewClient(user)`** (use), plus the package-level helpers below. For deeper
> docs, fetch any page/snippet from the manifest at
> <https://shipeasy-ai.github.io/sdk-go/manifest.json> (raw URLs below).

## Install

```bash
go get github.com/shipeasy-ai/sdk-go
```

```go
import shipeasy "github.com/shipeasy-ai/sdk-go"
```

## Configure once, bind per request

```go
// Once at process start. The api key lives here.
shipeasy.Configure(shipeasy.Options{
    APIKey: os.Getenv("SHIPEASY_SERVER_KEY"),
    // Optional: map YOUR user type → the Shipeasy attribute map.
    Attributes: func(u any) shipeasy.User {
        acct := u.(*Account)
        return shipeasy.User{"user_id": acct.ID, "plan": acct.Plan}
    },
    // Poll: true,  // long-running server: keep flags fresh with a background poll
})

// Per request: bind the user once, call with NO user argument.
c := shipeasy.NewClient(acct)            // or NewClient(shipeasy.User{"user_id": "u_123"})
```

`Configure` is first-config-wins and owns the fetch lifecycle (one-shot by
default; `Poll: true` for a background refresh — you never call `Init` yourself).
`NewClient` panics if called before `Configure`.

**Egress is quiet outside production.** `IsNetworkEnabled`/`IsTrackingEnabled`
(both `*bool`) default ON in production and OFF everywhere else, so dev/CI runs
make no outbound request. "Production" = `SHIPEASY_ENV`/`APP_ENV`/`GO_ENV`/`ENV`
being `production`/`prod`, else the `Env` option (defaults `"prod"`). Force it with
`x := true; Options{IsNetworkEnabled: &x}`. Reference:
<https://shipeasy-ai.github.io/sdk-go/pages/configuration.md>

## Evaluate

```go
c := shipeasy.NewClient(acct)                        // construct once per callsite
on := c.GetFlag("new_checkout")                      // bool
on = c.GetFlagOr("new_checkout", true)               // fallback only when UNEVALUATABLE
d := c.GetFlagDetail("new_checkout")                 // d.Value, d.Reason

cfg, ok := c.GetConfig("billing_copy")               // (any, bool)
fallback := c.GetConfigOr("billing_copy", map[string]any{"cta": "Buy"})

paused := c.GetKillswitch("payments_paused")         // true = killed
// Named switch: GetKillswitch(name, switchKey) — an unconfigured key falls back
// to the kill switch's top-level value.
```

`GetFlagOr` returns the fallback only on reason `CLIENT_NOT_READY` /
`FLAG_NOT_FOUND` — a gate that evaluates to `false` returns `false`. Reference:
<https://shipeasy-ai.github.io/sdk-go/pages/flags.md> ·
<https://shipeasy-ai.github.io/sdk-go/pages/killswitches.md>

## Experiments + track (Client-only, end to end)

Experiments are read by **universe** — a mutual-exclusion pool; a unit lands in
<=1 experiment. `Assign()` is side-effect free; the single (deduped) exposure
fires on the first enrolled `Get(...)`. Use `Peek(field, fallback)` to read a
param without logging one.

```go
c := shipeasy.NewClient(acct)                        // construct once per callsite
a := c.Universe("checkout").Assign()                 // Assignment (no getExperiment); no exposure yet
// a.Enrolled bool, a.Name/a.Group string ("" when not enrolled)
color := a.Get("color", "blue")                      // variant ?? universe default ?? fallback; first enrolled read logs the exposure
_ = a.Peek("color", "blue")                          // same lookup, logs NO exposure

c.Track("purchase", map[string]any{"amount": 49})    // conversion for the bound user
```

Reference: <https://shipeasy-ai.github.io/sdk-go/pages/experiments.md> · track
snippet <https://shipeasy-ai.github.io/sdk-go/snippets/metrics/track.md>

## Anonymous traffic

```go
http.ListenAndServe(":8080", shipeasy.Middleware(mux)) // mints __se_anon_id cookie
// in a handler:
c := shipeasy.NewClient(shipeasy.User{"anonymous_id": shipeasy.AnonID(r)})
```

## Error reporting — See()

```go
if err := chargeCard(o); err != nil {
    shipeasy.See(err).CausesThe("checkout").
        Extras(map[string]any{"order_id": o.ID}).
        To("use the backup processor")   // .To(...) is the terminal — sends the report
}
// Or pass extras inline (merged like a final .Extras — no ordering to remember):
shipeasy.See(err).CausesThe("checkout").
    To("use the backup processor", map[string]any{"order_id": o.ID})
// Expected control flow reports NOTHING:
shipeasy.ControlFlowException(err).Because("because empty-state path")
```

Reference: <https://shipeasy-ai.github.io/sdk-go/pages/error-reporting.md> · snippet
<https://shipeasy-ai.github.io/sdk-go/snippets/ops/see.md>

## Testing (zero network)

```go
// Seed values up front; reads go through the ordinary NewClient(user). Replaces
// prior config, so each test can reconfigure freely.
shipeasy.ConfigureForTesting(shipeasy.TestOptions{
    Flags:       map[string]bool{"new_checkout": true},
    Configs:     map[string]any{"billing_copy": map[string]any{"cta": "Buy now"}},
    Experiments: map[string]shipeasy.ExperimentOverride{"checkout_button": {Group: "treatment", Params: map[string]any{"color": "green"}}},
})
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_1"})
c.GetFlag("new_checkout") // true

shipeasy.OverrideFlag("new_checkout", false) // flip on the spot
shipeasy.ClearOverrides()                    // drop every override (incl. the seed)

// Offline: evaluate the REAL rules from a snapshot or JSON file, no network.
_, _ = shipeasy.ConfigureForOffline(shipeasy.OfflineOptions{Path: "shipeasy-snapshot.json"})
```

Reference: <https://shipeasy-ai.github.io/sdk-go/pages/testing.md>

## OpenFeature (separate nested module)

```bash
go get github.com/shipeasy-ai/sdk-go/openfeature
```

```go
import shipeasyof "github.com/shipeasy-ai/sdk-go/openfeature"

// Assumes shipeasy.Configure(...) ran — the global provider resolves it.
_ = openfeature.SetProviderAndWait(shipeasyof.NewGlobalProvider())
// boolean flags → gates; string/float/int/object → dynamic configs.
```

Reference: <https://shipeasy-ai.github.io/sdk-go/pages/openfeature.md>

## i18n

Server-side only: emit `shipeasy.I18nScriptTag()` in the page `<head>` — every
argument is optional (the public client key, profile and CDN origin come from
`Configure`); pass one `shipeasy.TagOptions` to override a single tag. Same for
`shipeasy.BootstrapScriptTag(user)` and `shipeasy.DevtoolsScriptTag()` (devtools
overlay: Shift+Alt+S or `?se=1`). The browser client SDK's `t()` renders the
labels. There is no server-side `t()` in Go. Reference:
<https://shipeasy-ai.github.io/sdk-go/pages/i18n.md>

## Change listeners

`shipeasy.OnChange(func(){ /* reloaded */ })` fires after a background poll
fetches new data — requires `Configure(shipeasy.Options{Poll: true})`. Reference:
<https://shipeasy-ai.github.io/sdk-go/pages/advanced.md>
