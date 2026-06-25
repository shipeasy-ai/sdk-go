# shipeasy-go

Server SDK for [Shipeasy](https://shipeasy.dev). Feature flags, configs, A/B experiments, metric tracking.

```bash
go get github.com/shipeasy-ai/sdk-go
```

Configure once at startup, then build a cheap user-bound `Client` per request:

```go
import (
    shipeasy "github.com/shipeasy-ai/sdk-go"
)

// Once, at process start. The api key lives here; the optional Attributes
// transform maps YOUR user type to the Shipeasy attribute map.
shipeasy.Configure(shipeasy.Options{
    APIKey: os.Getenv("SHIPEASY_SERVER_KEY"),
    Attributes: func(u any) shipeasy.User {
        acct := u.(*Account)
        return shipeasy.User{"user_id": acct.ID, "plan": acct.Plan}
    },
})

// Per request: bind the user once, then call with NO user argument.
c := shipeasy.NewClient(acct)            // acct is your own *Account
if c.GetFlag("new_checkout") { /* ... */ }

cfg, _ := c.GetConfig("billing_copy")

r := c.GetExperiment("checkout_button", map[string]any{"color": "blue"})
_ = r.Group
_ = r.Params

paused := c.GetKillswitch("payments_paused")
_ = paused
```

If you don't configure an `Attributes` transform, the value you pass to
`NewClient` is assumed to already BE the attribute map, so
`shipeasy.NewClient(shipeasy.User{"user_id": "u_123", "plan": "pro"})` works
as-is. `NewClient` **panics** if `Configure` was not called first (the api key
lives in the global config — failing loudly surfaces the misconfiguration).

`Configure` builds one shared **`Engine`** (the heavyweight type that owns the
api key, blob cache, poll timer, telemetry and the `see()` surface) and kicks
off a one-shot fetch in the background, so the first `NewClient(user).GetFlag()`
resolves against real rules without an explicit init.

### Managing the engine directly (advanced)

Most apps only need `Configure` + `NewClient`. When you need full control —
multiple engines in one process, an explicit `Init` to start background polling,
`Track`, `OnChange`, or the `Override*` setters — build an `Engine` yourself:

```go
import "context"

eng := shipeasy.NewEngine(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})
if err := eng.Init(context.Background()); err != nil { panic(err) }   // starts polling
defer eng.Destroy()

if eng.GetFlag("new_checkout", shipeasy.User{"user_id": "u_123"}) { /* ... */ }
eng.Track("u_123", "purchase", map[string]any{"amount": 49})
```

> **Breaking change in 0.8.0:** the heavyweight type formerly named `Client` is
> now `Engine`, and `NewClient(Options)` is now `NewEngine(Options)`. The name
> `Client` is now the lightweight user-bound handle built with
> `NewClient(user)`. See the [CHANGELOG](CHANGELOG.md).

> The sections below show the **`Engine`** API (methods take an explicit `user`
> argument). The bound `Client` from `NewClient(user)` exposes the same methods
> with the user already bound (no `user` argument) — pick whichever fits.

## Anonymous visitors (zero-config bucketing)

For logged-out traffic you need a *stable* unit so a fractional rollout buckets
the same on the server and in the browser. `shipeasy.Middleware` mints a
first-party `__se_anon_id` cookie (shared with every Shipeasy SDK) for any
request that lacks one, and exposes it via `shipeasy.AnonID(r)`:

```go
mux := http.NewServeMux()
// ... register handlers ...
http.ListenAndServe(":8080", shipeasy.Middleware(mux))
```

```go
func handler(w http.ResponseWriter, r *http.Request) {
    user := shipeasy.User{"anonymous_id": shipeasy.AnonID(r)} // or {"user_id": ...} once logged in
    if c.GetFlag("new_checkout", user) { /* ... */ }
}
```

The cookie is non-`HttpOnly` by design — the browser SDK reads it so the client
buckets identically to the server. A request with **no** unit still resolves a
fully-rolled (100%) gate as on; only fractional gates need the id. The cookie
name and format are a cross-SDK contract; see
[`18-identity-bucketing.md`](https://github.com/shipeasy-ai/experiment-platform/blob/main/18-identity-bucketing.md).

## Server-side rendering (SSR)

Emit the request's evaluated flags as a declarative `<script>` tag so the
browser SDK has them on first paint. `BootstrapScriptTag` carries the payload in
`data-*` attributes (**no key**); the static `se-bootstrap.js` loader hydrates
`window.__SE_BOOTSTRAP` and writes the `__se_anon_id` cookie so the browser
buckets identically to the server.

```go
user := shipeasy.User{"user_id": "u_123"}

// Two tags for the document <head>. The PUBLIC client key (not the server
// key) goes on the i18n loader tag.
head := c.BootstrapScriptTag(user, shipeasy.BootstrapTagOptions{AnonID: anonID}) +
    c.I18nScriptTag(clientKey, "en:prod", shipeasy.BootstrapTagOptions{})

// …or get the raw payload (Flags / Configs / Experiments / Killswitches):
boot := c.Evaluate(user)
```

`BootstrapTagOptions` also accepts `I18nProfile` and `BaseURL` (defaults to
`https://cdn.shipeasy.ai`).

## Default values

Go has no default arguments, so the SDK ships `…Or` variants that take an
explicit fallback. The fallback is returned only when the flag/config **cannot
be evaluated** — never when it evaluates to `false`:

```go
// def is returned ONLY when the flag can't be evaluated (client not ready, or
// the gate is absent). A gate that evaluates to false returns false, not def.
on := c.GetFlagOr("new_checkout", user, true)

// def is returned when the config key is absent. GetConfig stays (value, ok).
copy := c.GetConfigOr("billing_copy", map[string]any{"cta": "Buy"})
```

## Evaluation detail

`GetFlagDetail` returns the value plus a stable, exported reason explaining how
it was reached:

```go
d := c.GetFlagDetail("new_checkout", user)
// d.Value  bool
// d.Reason one of:
//   shipeasy.ReasonOverride       "OVERRIDE"          (a local Override* won)
//   shipeasy.ReasonClientNotReady "CLIENT_NOT_READY"  (Init not done; value=false)
//   shipeasy.ReasonFlagNotFound   "FLAG_NOT_FOUND"    (no such gate; value=false)
//   shipeasy.ReasonOff            "OFF"               (gate disabled/killswitched)
//   shipeasy.ReasonRuleMatch      "RULE_MATCH"        (evaluated true)
//   shipeasy.ReasonDefault        "DEFAULT"           (evaluated false)
```

`GetFlag` is `GetFlagDetail(...).Value`, and `GetFlagOr` returns `def` exactly
when the reason is `CLIENT_NOT_READY` or `FLAG_NOT_FOUND`.

## Change listeners

Register a callback that fires after a background poll loads **new** data (a
`200`, not a `304`). It returns a `cancel` func that deregisters the listener:

```go
cancel := c.OnChange(func() {
    log.Println("flags/experiments changed; re-render or warm caches")
})
defer cancel()
```

Listeners run after the poll updates the blobs; a panicking listener is
recovered and logged so it can't take down the poll loop. Test clients
(`NewTestClient`, offline clients) never poll, so they never fire listeners.

## Offline snapshot

Run evaluations against a captured snapshot of the edge blobs with **zero
network** — no key, no polling, no telemetry. The snapshot is JSON of the shape
`{ "flags": <body of /sdk/flags>, "experiments": <body of /sdk/experiments> }`:

```go
// From a file:
c, err := shipeasy.NewOfflineClient("shipeasy-snapshot.json")

// Or from in-memory blobs (e.g. fetched earlier and cached):
c := shipeasy.NewOfflineClientFromSnapshot(flagsBody, experimentsBody)

// Init/InitOnce/Track are no-ops; evaluations run the real evaluator against the
// snapshot, and Override* setters apply on top:
on := c.GetFlag("new_checkout", shipeasy.User{"user_id": "u_123"})
```

## Testing

In tests you usually don't want a live edge or a real API key. `NewTestClient`
returns a client that does **zero network** — telemetry is disabled,
`Init`/`InitOnce` are no-ops (they never fetch), `Track` is a no-op, and no API
key is required. Seed each entity with the `Override*` setters; an override
always wins over fetched data, so the setters work on a normal client too.

```go
func TestCheckout(t *testing.T) {
    c := shipeasy.NewTestClient()
    // No Init() needed, but it's safe to call (no-op):
    // _ = c.Init(context.Background())

    // Flags
    c.OverrideFlag("new_checkout", true)
    if !c.GetFlag("new_checkout", shipeasy.User{"user_id": "u_123"}) {
        t.Fatal("expected new_checkout on")
    }

    // Configs — GetConfig returns (value, true) for an overridden config:
    c.OverrideConfig("billing_copy", map[string]any{"cta": "Buy now"})
    cfg, ok := c.GetConfig("billing_copy") // cfg == map[...]; ok == true

    // Experiments — forces InExperiment=true with the given group/params:
    c.OverrideExperiment("checkout_button", "treatment", map[string]any{"color": "green"})
    r := c.GetExperiment("checkout_button", shipeasy.User{"user_id": "u_123"}, nil)
    // r.InExperiment == true; r.Group == "treatment"; r.Params == {"color":"green"}

    // Track is a no-op on a test client — never panics, never hits the network:
    c.Track("u_123", "purchase", map[string]any{"amount": 49})

    // Reset between cases:
    c.ClearOverrides()
    _ = cfg
    _ = r
    _ = ok
}
```

