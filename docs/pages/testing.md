# Testing

In tests you don't want a live edge or a real API key. Configure Shipeasy in
test mode with `ConfigureForTesting` (or `ConfigureForOffline`), then read the
seeded values through the ordinary `NewClient(user)`. Both are drop-in siblings
of `Configure` that do **zero network, ever** — no api key needed — and they
**replace** any previous configuration, so a test suite can reconfigure freely
between cases.

## `ConfigureForTesting` — seed the values by hand

```go
func TestCheckout(t *testing.T) {
    shipeasy.ConfigureForTesting(shipeasy.TestOptions{
        Flags:       map[string]bool{"new_checkout": true},
        Configs:     map[string]any{"billing_copy": map[string]any{"cta": "Buy now"}},
        Experiments: map[string]shipeasy.ExperimentOverride{
            "checkout_button": {Group: "treatment", Params: map[string]any{"color": "green"}},
        },
    })

    c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"}) // bind once

    if !c.GetFlag("new_checkout") {
        t.Fatal("expected new_checkout on")
    }
    if r := c.GetExperiment("checkout_button", nil); r.Group != "treatment" {
        t.Fatalf("got group %q", r.Group)
    }
}
```

`TestOptions` fields are all optional:

| Field | Type | Effect |
| --- | --- | --- |
| `Flags` | `map[string]bool` | forced `GetFlag` results |
| `Configs` | `map[string]any` | forced `GetConfig` results |
| `Experiments` | `map[string]ExperimentOverride` | forced enrolments (`Group` + `Params`) |
| `Attributes` | `func(any) User` | same transform as `Configure` (default identity) |

`Track` and `LogExposure` are no-ops in test mode — they never hit the network.

## On-the-spot overrides

The package-level `Override*` helpers force a single value on top of whatever the
current configuration set up. They win over everything until `ClearOverrides`:

```go
shipeasy.OverrideFlag("new_checkout", true)         // force GetFlag → true
shipeasy.OverrideConfig("billing_copy", "Buy now")  // force GetConfig → ("Buy now", true)
shipeasy.OverrideExperiment("checkout_button", "treatment", map[string]any{"color": "green"})

// ... assertions ...

shipeasy.ClearOverrides() // reset between cases
```

| Helper | Effect |
| --- | --- |
| `OverrideFlag(name, bool)` | force `GetFlag(name)` to that value |
| `OverrideConfig(name, value)` | force `GetConfig(name)` → `(value, true)` |
| `OverrideExperiment(name, group, params)` | force enrolment with `group` / `params` |
| `ClearOverrides()` | drop every flag/config/experiment override |

Under `ConfigureForTesting` there is no blob beneath, so `ClearOverrides` reverts
everything (including the `TestOptions` seed) to empty-blob defaults. Under
`ConfigureForOffline` the snapshot remains and evaluations revert to it.

## `ConfigureForOffline` — evaluate the REAL rules from a snapshot

`ConfigureForOffline` evaluates the **real** evaluator against a captured snapshot
of the edge blobs — still zero network. Provide exactly one source: an in-memory
`Snapshot`, or a `Path` to a JSON file. The `Flags` / `Configs` / `Experiments`
overrides layer on top.

```go
// From a JSON file:
shipeasy.ConfigureForOffline(shipeasy.OfflineOptions{Path: "shipeasy-snapshot.json"})

// Or from in-memory parsed blobs:
shipeasy.ConfigureForOffline(shipeasy.OfflineOptions{
    Snapshot: &shipeasy.Snapshot{Flags: flagsBody, Experiments: experimentsBody},
})

c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})
on := c.GetFlag("new_checkout") // runs the real rollout/targeting evaluator
_ = on
```

`ConfigureForOffline` returns an error only when reading/parsing a `Path` snapshot
fails.

### Snapshot file format

The file is JSON of the shape
`{ "flags": <body of /sdk/flags>, "experiments": <body of /sdk/experiments> }`.
A minimal but complete, valid snapshot — `new_checkout` rolled out to 10% of
users:

```json
{
  "flags": {
    "version": 1,
    "plan": "pro",
    "gates": {
      "new_checkout": { "rules": [], "rolloutPct": 1000, "salt": "s", "enabled": 1 }
    },
    "configs": {},
    "killswitches": {}
  },
  "experiments": {
    "version": 1,
    "universes": {},
    "experiments": {}
  }
}
```

`rolloutPct` is in **basis points**: `10000` = 100%, `1000` = 10%, `0` = off. A
gate object is `{ "rules": [...], "rolloutPct": <bp>, "salt": "<str>", "enabled": 0|1 }`.
