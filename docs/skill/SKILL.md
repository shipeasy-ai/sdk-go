---
name: shipeasy-go
description: Use Shipeasy (feature flags, configs, kill switches, A/B experiments, i18n) from Go. Covers Configure() + NewClient(user), GetFlag/GetConfig/GetExperiment/GetKillswitch, Track, testing, OpenFeature.
---

# Shipeasy Go SDK

Server-side Go SDK for Shipeasy. Evaluation is local against a cached blob — no
network on the hot path. Min Go 1.21.

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
})

// Per request: bind the user once, call with NO user argument.
c := shipeasy.NewClient(acct)            // or NewClient(shipeasy.User{"user_id": "u_123"})
```

`NewClient` panics if `Configure` was not called first. Long-running servers that
want live updates call `shipeasy.ConfiguredEngine().Init(ctx)` to start polling.

## Evaluate

```go
on := c.GetFlag("new_checkout")                      // bool
on = c.GetFlagOr("new_checkout", true)               // fallback only when UNEVALUATABLE
d := c.GetFlagDetail("new_checkout")                 // d.Value, d.Reason

cfg, ok := c.GetConfig("billing_copy")               // (any, bool)
copy := c.GetConfigOr("billing_copy", map[string]any{"cta": "Buy"})

paused := c.GetKillswitch("payments_paused")         // true = killed
```

`GetFlagOr` returns the fallback only on reason `CLIENT_NOT_READY` or
`FLAG_NOT_FOUND` — a gate that evaluates to `false` returns `false`.

## Experiments + track

```go
r := c.GetExperiment("checkout_button", map[string]any{"color": "blue"})
// r.InExperiment bool, r.Group string, r.Params any (defaultParams when not enrolled)

// Conversion event (engine-level; bare user id):
shipeasy.ConfiguredEngine().Track("u_123", "purchase", map[string]any{"amount": 49})

// Manual exposure when you present the treatment:
shipeasy.ConfiguredEngine().LogExposure("u_123", "checkout_button")
```

## Engine form (advanced)

The engine methods take an explicit `user`. Use `NewEngine` for multiple engines
or explicit control:

```go
eng := shipeasy.NewEngine(shipeasy.Options{APIKey: key})
_ = eng.Init(ctx); defer eng.Destroy()
eng.GetFlag("new_checkout", shipeasy.User{"user_id": "u_123"})
cancel := eng.OnChange(func(){ /* reloaded */ }); defer cancel()
```

## Anonymous traffic

```go
http.ListenAndServe(":8080", shipeasy.Middleware(mux)) // mints __se_anon_id cookie
// in a handler:
user := shipeasy.User{"anonymous_id": shipeasy.AnonID(r)}
```

## Error reporting — See()

```go
if err := chargeCard(o); err != nil {
    shipeasy.See(err).CausesThe("checkout").
        Extras(map[string]any{"order_id": o.ID}).
        To("use the backup processor")   // .To(...) is the terminal — sends the report
}
// Expected control flow reports NOTHING:
shipeasy.ControlFlowException(err).Because("because empty-state path")
```

## Testing (zero network)

```go
c := shipeasy.NewTestClient()            // no key, no network
c.OverrideFlag("new_checkout", true)
c.OverrideConfig("billing_copy", map[string]any{"cta": "Buy now"})
c.OverrideExperiment("checkout_button", "treatment", map[string]any{"color": "green"})
c.ClearOverrides()

// Or against a captured snapshot:
oc, _ := shipeasy.NewOfflineClient("shipeasy-snapshot.json")
```

## OpenFeature (separate nested module)

```bash
go get github.com/shipeasy-ai/sdk-go/openfeature
```

```go
import shipeasyof "github.com/shipeasy-ai/sdk-go/openfeature"

eng := shipeasy.NewEngine(shipeasy.Options{APIKey: key}); _ = eng.Init(ctx)
_ = openfeature.SetProviderAndWait(shipeasyof.NewProvider(eng))
// boolean flags → gates; string/float/int/object → dynamic configs.
```

## i18n

Server-side only: emit `eng.I18nScriptTag(clientKey, "en:prod", opts)` in the
page `<head>` (public client key). The browser client SDK's `t()` renders the
labels. There is no server-side `t()` in Go.
