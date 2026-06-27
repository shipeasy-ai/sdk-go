# Testing

In tests you usually don't want a live edge or a real API key. The SDK ships two
zero-network factories plus `Override*` setters.

## `NewTestClient` — seed everything by hand

`NewTestClient()` returns an `*Engine` that does **zero network**: telemetry is
disabled, `Init`/`InitOnce` are no-ops (they never fetch), `Track` is a no-op,
and no API key is required. Seed each entity with the `Override*` setters — an
override always wins over fetched data.

```go
func TestCheckout(t *testing.T) {
    c := shipeasy.NewTestClient()

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

    c.ClearOverrides() // reset between cases
    _, _, _ = cfg, ok, r
}
```

The `Override*` setters also work on a normal engine built with `NewEngine` — an
override always wins over fetched data in `GetFlag`/`GetConfig`/`GetExperiment`.

| Setter | Effect |
| --- | --- |
| `OverrideFlag(name, bool)` | force `GetFlag(name, _)` to that value |
| `OverrideConfig(name, value)` | force `GetConfig(name)` → `(value, true)` |
| `OverrideExperiment(name, group, params)` | force `InExperiment=true` with `group`/`params` |
| `ClearOverrides()` | remove all flag, config, and experiment overrides |

## Offline snapshot clients

Run evaluations against a captured snapshot of the edge blobs with **zero
network** — no key, no polling, no telemetry. The snapshot is JSON of the shape
`{ "flags": <body of /sdk/flags>, "experiments": <body of /sdk/experiments> }`:

```go
// From a file:
c, err := shipeasy.NewOfflineClient("shipeasy-snapshot.json")

// Or from in-memory blobs (parsed bodies of /sdk/flags and /sdk/experiments):
c := shipeasy.NewOfflineClientFromSnapshot(flagsBody, experimentsBody)
```

`Init`/`InitOnce`/`Track` are no-ops; evaluations run the **real** evaluator
against the snapshot, and `Override*` setters apply on top:

```go
on := c.GetFlag("new_checkout", shipeasy.User{"user_id": "u_123"})
```
