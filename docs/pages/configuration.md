# Configuration

## `Configure` — the front door

Call `Configure` **once** at process start. It stores the api key and the
optional `Attributes` transform as a package global, and kicks off a background
fetch so a later `NewClient(user).GetFlag()` resolves against real rules without
any explicit init.

```go
shipeasy.Configure(shipeasy.Options{
    APIKey: os.Getenv("SHIPEASY_SERVER_KEY"),
    Attributes: func(u any) shipeasy.User {
        acct := u.(*Account)
        return shipeasy.User{"user_id": acct.ID, "plan": acct.Plan}
    },
})
```

`Configure` is **first-config-wins** (idempotent): the first call registers the
configuration and starts the fetch; subsequent calls are no-ops. After it runs,
build a cheap user-bound `Client` per request with `NewClient(user)`.

For the full `Options` field table see [Installation](installation.md).

## The `Attributes` transform & identity default

`Attributes func(any) shipeasy.User` maps **your** user value (any shape) to the
Shipeasy attribute map used for every evaluation. It is applied **once** in
`NewClient(user)` and the result is cached on the bound `Client`.

If you omit it, the identity transform is used: a `shipeasy.User` (or a
`map[string]any`) passed to `NewClient` is used as-is; `nil` becomes an empty
map; any other type degrades to an empty map (unidentified user) with a warning.

## init / poll vs one-shot

You never start the fetch yourself — `Configure` owns the fetch lifecycle, and
two `Options` fields choose its shape:

- **default** — `Configure` does a one-shot fire-and-forget fetch in the
  background, then never refreshes. Ideal for short-lived / serverless processes.
- **`Poll: true`** — `Configure` does an initial fetch plus a periodic background
  refresh (default interval 30s, re-tuned from the edge's `X-Poll-Interval`
  header), so flags stay fresh without a redeploy. Use this for long-running
  servers.
- **`NoInitialFetch: true`** — suppresses even the one-shot fetch (the
  `init=false` escape hatch). Ignored when `Poll` is true.

```go
// Long-running server that wants live updates:
shipeasy.Configure(shipeasy.Options{
    APIKey: os.Getenv("SHIPEASY_SERVER_KEY"),
    Poll:   true,
})
```

## Change listeners — `OnChange`

When polling is on (`Poll: true`), register a callback fired after a background
poll loads **new** data (a 200, not a 304). It returns a `cancel` func:

```go
cancel := shipeasy.OnChange(func() {
    log.Println("flags/experiments changed; re-render or warm caches")
})
defer cancel()
```

`OnChange` requires `Configure(Options{Poll: true})` — no poll runs otherwise, so
the listener never fires. A panicking listener is recovered and logged so it
can't take down the poll loop.

## Fail-safe reads & the `LogLevel` option

Reads never panic into your request path. Every runtime read on the bound
`Client` — `GetFlag`, `GetFlagOr`, `GetFlagDetail`, `GetConfig`, `GetConfigOr`,
`GetKillswitch`, `GetExperiment` — is wrapped so that even an unexpected panic is
caught, logged, and the read returns its documented safe default (`GetFlag` →
`false`, `GetConfig` → `(nil, false)`, `GetExperiment` → the control result with
your `defaultParams`, `GetKillswitch` → `false`). You never need a `recover()`
around a read. (Setup mistakes still fail loudly — `NewClient(user)` before
`Configure` panics on purpose.)

`Options.LogLevel` controls the SDK's own log verbosity. The SDK logs its
fire-and-forget failures (a failed `Track`/`LogExposure`/`see()` POST, a poll
error, or a recovered panic) with a `[shipeasy] ` prefix. Levels, from quietest
to loudest, are `silent`, `error`, `warn`, `info`, `debug`; a message at level L
is printed only when your configured level is at least L. The default is `warn`;
an empty or unrecognized value also resolves to `warn`.

```go
shipeasy.Configure(shipeasy.Options{
    APIKey:   os.Getenv("SHIPEASY_SERVER_KEY"),
    LogLevel: "silent", // fully quiet the SDK's background chatter in prod
})
```

### SDK self-monitoring

When one of those last-resort guards actually fires — a bug on Shipeasy's side,
not yours — the SDK also reports that internal error to **Shipeasy's own
project** so we can track and fix SDK bugs across every app the SDK runs in. It
is a dedicated, baked-in destination (a public client-key ingest credential),
entirely separate from your `See()` reporting: internal errors never land in your
project or Errors tab. The report carries only the error itself plus a stable,
deduped consequence (subject = the guarded operation, e.g. `GetFlag`), is
rate-limited, and is fire-and-forget — it can never slow down or break a read. It
is on by default and always off in test/offline mode. Opt out entirely with
`DisableInternalErrorReporting: true`:

```go
shipeasy.Configure(shipeasy.Options{
    APIKey:                        os.Getenv("SHIPEASY_SERVER_KEY"),
    DisableInternalErrorReporting: true, // suppress the SDK self-monitoring channel
})
```

## Env-var convention

The SDK authenticates with your project's **server** key. Read it from the
environment — never hard-code it:

```bash
export SHIPEASY_SERVER_KEY="sk_server_..."
```
