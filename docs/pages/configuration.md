# Configuration

## `Configure` — the front door

Call `Configure` **once** at process start. It builds one process-wide `Engine`,
stores it (plus the optional `Attributes` transform) as a package global, and
kicks off a fire-and-forget one-shot fetch in the background so a later
`NewClient(user).GetFlag()` resolves against real rules without an explicit init.

```go
shipeasy.Configure(shipeasy.Options{
    APIKey: os.Getenv("SHIPEASY_SERVER_KEY"),
    Attributes: func(u any) shipeasy.User {
        acct := u.(*Account)
        return shipeasy.User{"user_id": acct.ID, "plan": acct.Plan}
    },
})
```

`Configure` is **first-config-wins** (idempotent): the first call builds and
registers the engine; subsequent calls are no-ops and return the already-built
engine. It returns the global `*Engine`. `ConfiguredEngine()` fetches it later
(or `nil` if `Configure` was never called).

### The `Attributes` transform & identity default

`Attributes func(any) shipeasy.User` maps **your** user value (any shape) to the
Shipeasy attribute map used for every evaluation. It is applied **once** in
`NewClient(user)` and the result is cached on the bound `Client`.

If you omit it, the identity transform is used: a `shipeasy.User` (or a
`map[string]any`) passed to `NewClient` is used as-is; `nil` becomes an empty
map; any other type degrades to an empty map (unidentified user) with a warning.

## `Options` reference

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `APIKey` | `string` | — | Server key. Authenticates blob fetches and `/collect`. |
| `BaseURL` | `string` | `https://edge.shipeasy.dev` | Edge API origin for the flag/experiment blobs. |
| `HTTP` | `*http.Client` | 10s-timeout client | Custom HTTP client. |
| `Env` | `string` | `"prod"` | Published env reported in usage + `See()` telemetry. |
| `DisableTelemetry` | `bool` | `false` | Turn off per-evaluation usage beacons. |
| `TelemetryURL` | `string` | default beacon host | Override the beacon host. |
| `PrivateAttributes` | `[]string` | — | Event property keys stripped from every outbound `/collect` payload. |
| `StickyStore` | `StickyBucketStore` | `nil` | Lock in experiment assignments per unit. See [Advanced](advanced.md). |
| `Attributes` | `func(any) User` | identity | Transform from your user type to `User`. Consumed by `Configure` + `Client`; `NewEngine` ignores it. |

## init / poll vs one-shot

- **`Configure`** does a one-shot fetch (`InitOnce`-equivalent) in the
  background. No background polling unless you opt in.
- **`Engine.Init(ctx)`** does a blocking initial fetch, then starts a background
  poll loop (default interval 30s, re-tuned from the edge's `X-Poll-Interval`
  header). Long-running servers that want live updates call this on the
  configured engine.
- **`Engine.InitOnce(ctx)`** fetches once, no poll loop. No-op if already
  initialized.
- **`Engine.Destroy()`** stops the poll loop.

```go
eng := shipeasy.ConfiguredEngine()
if err := eng.Init(context.Background()); err != nil { panic(err) } // start polling
defer eng.Destroy()
```

## Managing an engine directly (advanced)

When you need full control — multiple engines, explicit `Init`, `Track`,
`OnChange`, the `Override*` setters — build an `Engine` yourself:

```go
eng := shipeasy.NewEngine(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})
if err := eng.Init(context.Background()); err != nil { panic(err) }
defer eng.Destroy()

if eng.GetFlag("new_checkout", shipeasy.User{"user_id": "u_123"}) { /* ... */ }
eng.Track("u_123", "purchase", map[string]any{"amount": 49})
```

`NewEngine` ignores `Options.Attributes` — only `Configure`/`Client` use it.
