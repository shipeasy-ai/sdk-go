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
    // Bind the anon id as the user, then read as usual.
    c := shipeasy.NewClient(shipeasy.User{"anonymous_id": shipeasy.AnonID(r)}) // or {"user_id": ...}
    if c.GetFlag("new_checkout") { /* ... */ }
}
```

The cookie is non-`HttpOnly` by design (the browser SDK reads it). A request with
**no** unit still resolves a fully-rolled (100%) gate as on; only fractional
gates need the id. Lower-level helpers: `MintAnonID`, `ReadOrMintAnonID`,
`SetAnonIDCookie`. The cookie name/format is a cross-SDK contract
([Identity & bucketing](https://docs.shipeasy.ai/get-started/identity-and-bucketing)).

## SSR bootstrap

Emit the request's evaluated flags as a declarative `<script>` tag so the browser
SDK has them on first paint (no key embedded). Both helpers are package-level and
run off the global configuration:

```go
user := shipeasy.User{"user_id": "u_123"}
head := shipeasy.BootstrapScriptTag(user, shipeasy.TagOptions{AnonID: anonID}) +
    shipeasy.I18nScriptTag()
```

### Every argument is optional

The tag helpers take **variadic** `TagOptions`: pass none and every value comes
from `Configure`, or pass one to override a field for that tag.

| Helper | Signature | Defaults from `Options` |
| --- | --- | --- |
| `shipeasy.I18nScriptTag` | `(opts ...TagOptions)` | `ClientKey`, `Profile`, `CDNBaseURL` |
| `shipeasy.BootstrapScriptTag` | `(user User, opts ...TagOptions)` | `nil` user ⇒ anonymous; `Profile`, `CDNBaseURL` |
| `shipeasy.DevtoolsScriptTag` | `(opts ...TagOptions)` | `ProjectID`, `ClientKey`, `CDNBaseURL` |

```go
shipeasy.Configure(shipeasy.Options{
    APIKey:    os.Getenv("SHIPEASY_SERVER_KEY"),
    ClientKey: os.Getenv("SHIPEASY_CLIENT_KEY"), // PUBLIC key, for the tags
    ProjectID: os.Getenv("SHIPEASY_PROJECT_ID"), // for the devtools tag
    Profile:   "en:prod",
})
```

`TagOptions` fields: `AnonID`, `I18nProfile`, `ClientKey`, `ProjectID`,
`BaseURL`, `NoDefer` (`BootstrapTagOptions` is kept as an alias). A tag still
renders when a value is missing — the browser bundle reports what it needs — but
the SDK logs a warning naming the `Options` field to fill in, once per field
rather than once per render.

### Devtools overlay tag

`shipeasy.DevtoolsScriptTag()` emits the hosted devtools overlay bundle —
nothing to install, no overlay code in your binary or bundle. It reads the
project id and public client key off the tag and opens with **Shift+Alt+S** or on
any page loaded with `?se=1`. It is `defer`red unless you set
`TagOptions{NoDefer: true}`: a developer tool never belongs on the critical
rendering path.

```go
head += shipeasy.DevtoolsScriptTag()
```

Adding it unconditionally is fine: the overlay only opens for someone with a
signed-in Shipeasy session, so on a page where nobody has authenticated it
renders nothing and says nothing. Gating it on your own staff or environment
check is **optional** — worth it only if you'd rather the bundle not load for
end users at all:

```go
if user.IsStaff {
    head += shipeasy.DevtoolsScriptTag()
}
```

### Identity coherence — no anon→identified flip

When the `user` you evaluate is identified (any attribute other than
`anonymous_id`), the tag also carries the identity as a `data-user` attribute
(the JSON of the user's attributes, minus `anonymous_id` — that already rides
`data-anon-id`). The browser SDK adopts that identity on first paint, so the
client buckets as the **same** user the server did — no anonymous→identified
flip after hydration. An anonymous request (only `anonymous_id`, or an empty
user) emits **no** `data-user`, so no PII rides the tag. See
[Identity & bucketing](https://docs.shipeasy.ai/get-started/identity-and-bucketing).

## Exposure logging

`Universe(name).Assign()` auto-logs a single exposure when the unit is enrolled —
you don't call anything separately. The exposure is deduped per process (unit +
experiment + group) so repeated `Assign()` calls in a long-running server don't
spam `/collect`; it re-evaluates against the bound attributes (so `bucketBy` and
`anonymous_id`-only traffic resolve correctly) and POSTs one
`{type:"exposure", experiment, group, user_id/anonymous_id, ts}` event:

```go
c := shipeasy.NewClient(shipeasy.User{"anonymous_id": anonID}) // bind once
a := c.Universe("hero_cta").Assign()                            // logs the exposure if enrolled
_ = a
```

No-op in local mode (test/offline) or when the unit isn't enrolled.

## Private attributes

`Options.PrivateAttributes` lists event-property keys stripped from every
outbound `/collect` payload (`Track`, exposure, `See` extras). Server
evaluation is local, so private attrs never egress for evaluation either —
only Track/exposure/error events ever leave the process.

```go
shipeasy.Configure(shipeasy.Options{
    APIKey:            os.Getenv("SHIPEASY_SERVER_KEY"),
    PrivateAttributes: []string{"email", "ssn"},
})
```

## DevTools overrides on server reads

A signed-in Shipeasy DevTools overlay mirrors its overrides (gates, kill switches,
configs, experiment groups, including the `?se_gate_<name>=true` URL state) into
a signed `se_ov` cookie. The browser sends it on every same-origin request, so
`shipeasy.Middleware` can verify it once per request and put the result on the
request context. Bind your `Client` to that context:

```go
http.ListenAndServe(":8080", shipeasy.Middleware(mux))

// in a handler
c := shipeasy.NewClient(user).WithContext(r.Context())
c.GetFlag("new_checkout") // honors the signed-in DevTools override
```

Without `Middleware`, scope it yourself with `WithRequestOverrides`:

```go
ctx := shipeasy.WithRequestOverrides(r.Context(), r.Header.Get("Cookie"))
c := shipeasy.NewClientContext(ctx, user)
```

Precedence on a read is programmatic `Override*` > signed cookie > normal
evaluation. A cookie override reports `shipeasy.ReasonOverride`. The cookie's
`gates` map covers kill switches too (whole-switch `GetKillswitch` reads only),
`configs` replace `GetConfig` values, and `exps` force a universe assignment to
that group.

It fails closed: overrides apply only when Shipeasy serves your project's
override key, the cookie signature is valid, and it has not expired. A client
with no request context, or an unsigned `?se_gate_` URL parameter alone, is
unaffected, so those stay client-only by design.

### Local dev: `?se_gate_x=true` without signing in

On a local, dev or test stack you can skip DevTools sign-in. The browser SDK
mirrors the URL overrides into an unsigned `se_ov_dev` cookie outside
production builds, and the server reads it when you opt in:

```go
shipeasy.Configure(shipeasy.Options{
    APIKey:       os.Getenv("SHIPEASY_SERVER_KEY"),
    DevOverrides: true, // default false
})
```

Two guards apply: `DevOverrides` must be true AND the process must be
non-production (`SHIPEASY_ENV`, `APP_ENV`, `GO_ENV` or `ENV`; `ConfigureForTesting`
counts as non-production via `TestOptions.DevOverrides`). Set in a production
process it is ignored with one warning, and the cookie is never even read. The
cookie is unsigned, so it is **never active in production**. Per name the
precedence is programmatic `Override*` > signed `se_ov` > `se_ov_dev` >
normal evaluation.

### Frontend and API on different subdomains

When the frontend is `app.example.com` and the API is `api.example.com`, the
browser SDK scopes the override cookies to the registrable domain (auto-detected,
or set `overrideCookieDomain` in the browser `configure()`), so the API sees them
and `Middleware` works unchanged. For token-auth APIs (or when you do not want
cookies sent), the browser SDK's `getOverrideHeaders()` returns
`{"X-Shipeasy-Overrides": "se_ov=...; se_ov_dev=..."}`: attach it to your API
requests (the API's CORS must allow that header). `Middleware` reads it
automatically; for manual scoping pass it as the optional third argument:

```go
ctx := shipeasy.WithRequestOverrides(r.Context(), r.Header.Get("Cookie"),
    r.Header.Get(shipeasy.OverridesHeader))
```

Per cookie name the `Cookie` header wins and the custom header fills in when it
is absent. The same verification and guards apply (a signed value must verify;
`se_ov_dev` is not even parsed unless `DevOverrides` is on in a non-production
process).

## bucketBy

An experiment can bucket on an attribute other than the individual (e.g.
`company_id` to keep a whole org on one variant). It's a property of the
experiment definition; supply the attribute on the user you bind and the SDK uses
it as the bucketing unit (falling back to `user_id ?? anonymous_id`):

```go
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123", "company_id": "acme"})
a := c.Universe("dashboards").Assign()
_ = a
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
a 304). It is package-level and returns a `cancel` func. It requires
`Configure(Options{Poll: true})` — no poll runs otherwise:

```go
cancel := shipeasy.OnChange(func() {
    log.Println("flags/experiments changed; re-render or warm caches")
})
defer cancel()
```

A panicking listener is recovered and logged so it can't take down the poll loop.
Test/offline configurations never poll, so they never fire listeners.
