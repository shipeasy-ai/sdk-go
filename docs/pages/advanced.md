# Advanced

## Anonymous-visitor bucketing & middleware

For logged-out traffic you need a *stable* unit so a fractional rollout buckets
identically on the server and in the browser. `shipeasy.Middleware` mints a
first-party `__se_anon_id` cookie (shared with every Shipeasy SDK) for any
request lacking one, and exposes it via `shipeasy.AnonID(r)`:

```go
mux := http.NewServeMux()
// ... register handlers ...
http.ListenAndServe(":8080", shipeasy.Middleware(mux))

func handler(w http.ResponseWriter, r *http.Request) {
    user := shipeasy.User{"anonymous_id": shipeasy.AnonID(r)} // or {"user_id": ...}
    if c.GetFlag("new_checkout", user) { /* ... */ }
}
```

The cookie is non-`HttpOnly` by design (the browser SDK reads it). A request with
**no** unit still resolves a fully-rolled (100%) gate as on; only fractional
gates need the id. Lower-level helpers: `MintAnonID`, `ReadOrMintAnonID`,
`SetAnonIDCookie`. The cookie name/format is a cross-SDK contract
([18-identity-bucketing.md](https://github.com/shipeasy-ai/experiment-platform/blob/main/18-identity-bucketing.md)).

## SSR bootstrap

Emit the request's evaluated flags as a declarative `<script>` tag so the browser
SDK has them on first paint (no key embedded):

```go
user := shipeasy.User{"user_id": "u_123"}
head := c.BootstrapScriptTag(user, shipeasy.BootstrapTagOptions{AnonID: anonID}) +
    c.I18nScriptTag(clientKey, "en:prod", shipeasy.BootstrapTagOptions{})

// …or get the raw payload (Flags / Configs / Experiments / Killswitches):
boot := c.Evaluate(user)
```

`BootstrapTagOptions` accepts `AnonID`, `I18nProfile`, and `BaseURL` (defaults to
`https://cdn.shipeasy.ai`).

## Manual exposure

The server is stateless and never auto-logs exposures. When you actually present
a treatment, log a single exposure event. From a bound `Client` use
`c.LogExposure("checkout_button")` (it re-evaluates against the bound
attributes); the `Engine` forms below are the explicit-user, advanced path:

```go
eng := shipeasy.ConfiguredEngine()

// Bare user id:
eng.LogExposure("u_123", "checkout_button")

// Full User (needed for bucketBy experiments or anonymous_id-only traffic):
eng.LogExposureUser(shipeasy.User{"anonymous_id": anonID}, "checkout_button")
```

It re-evaluates the experiment; if the user is enrolled, one
`{type:"exposure", experiment, group, user_id/anonymous_id, ts}` event is POSTed
to `/collect`. No-op in local mode or when the user isn't enrolled.

## Private attributes

`Options.PrivateAttributes` lists event-property keys stripped from every
outbound `/collect` payload (`Track`, `LogExposure`, `See` extras). Server
evaluation is local, so private attrs never egress for evaluation either —
only Track/exposure/error events ever leave the process.

```go
shipeasy.Configure(shipeasy.Options{
    APIKey:            os.Getenv("SHIPEASY_SERVER_KEY"),
    PrivateAttributes: []string{"email", "ssn"},
})
```

## bucketBy

An experiment can bucket on an attribute other than the individual (e.g.
`company_id` to keep a whole org on one variant). It's a property of the
experiment definition; supply the attribute on the `User` and the SDK uses it as
the bucketing unit (falling back to `user_id ?? anonymous_id`):

```go
r := eng.GetExperiment("new_dashboard",
    shipeasy.User{"user_id": "u_123", "company_id": "acme"}, nil)
```

## Sticky bucketing

A `StickyBucketStore` locks in experiment assignments per bucketing unit so a
later weight/allocation change can't reshuffle an enrolled user (a salt change
still reshuffles). Supply one via `Options.StickyStore`:

```go
store := shipeasy.NewInMemoryStickyStore() // process-local; or implement StickyBucketStore
shipeasy.Configure(shipeasy.Options{
    APIKey:      os.Getenv("SHIPEASY_SERVER_KEY"),
    StickyStore: store,
})
```

`NewInMemoryStickyStore` is process-local (handy for tests and single-process
servers). Implement the `StickyBucketStore` interface (`Get`/`Set`, keyed by
unit) for a shared/persistent store. Implementations must be safe for concurrent
use. Absent ⇒ purely deterministic bucketing.

## Change listeners

Register a callback fired after a background poll loads **new** data (a 200, not
a 304). It returns a `cancel` func:

```go
cancel := eng.OnChange(func() {
    log.Println("flags/experiments changed; re-render or warm caches")
})
defer cancel()
```

A panicking listener is recovered and logged so it can't take down the poll loop.
Test/offline clients never poll, so they never fire listeners.
