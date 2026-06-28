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

## Env-var convention

The SDK authenticates with your project's **server** key. Read it from the
environment — never hard-code it:

```bash
export SHIPEASY_SERVER_KEY="sk_server_..."
```
