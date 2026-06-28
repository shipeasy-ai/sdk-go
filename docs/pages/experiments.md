# A/B experiments

`GetExperiment` evaluates an experiment for a user and returns an
`ExperimentResult` describing whether they were enrolled, which group they
landed in, and the group's params.

## `ExperimentResult`

```go
type ExperimentResult struct {
    InExperiment bool   // enrolled in (not held out / allocated into) the experiment
    Group        string // the assigned group name (e.g. "control", "treatment")
    Params       any    // the group's params, or your defaultParams when not enrolled
}
```

## Evaluating an experiment

```go
c := shipeasy.NewClient(acct)            // bind the user once

// defaultParams is returned in r.Params when the user is NOT enrolled
// (held out, outside allocation, or the experiment isn't running).
r := c.GetExperiment("checkout_button", map[string]any{"color": "blue"})

if r.InExperiment {
    p := r.Params.(map[string]any)
    renderButton(p["color"])
} else {
    renderButton("blue") // control / default
}
```

When the user is not enrolled, `Group` is `"control"` and `Params` falls back to
the `defaultParams` you passed.

## Tracking conversions with `Client.Track`

To measure an experiment you log a conversion event. You already have a bound
`Client` (the same one you called `GetExperiment` on), so call `Track` on it —
the unit id is derived from the bound attribute map (`user_id`, else
`anonymous_id`), so there's no user argument. The event is fire-and-forget
POSTed to `/collect`:

```go
c := shipeasy.NewClient(acct)
// ... present the treatment from c.GetExperiment(...) ...
c.Track("{{SUCCESS_EVENT}}", map[string]any{"amount": 49})
```

`Client.Track(event, props)` takes the event name and an optional property bag.
This makes an experiment end-to-end Client-only: `NewClient(user)` →
`GetExperiment` → `Track`. `Track` is fire-and-forget (never blocks your
response) and a no-op under `ConfigureForTesting` / `ConfigureForOffline`.

## Manual exposure

The server is stateless and never auto-logs an exposure. When you actually
present the treatment, call `LogExposure` on the bound `Client` — the experiment
is re-evaluated against the bound attributes (so `bucketBy` / anonymous traffic
resolve correctly) and one exposure is logged if the user is enrolled:

```go
c := shipeasy.NewClient(acct)
c.LogExposure("checkout_button")
```

See [Advanced](advanced.md) for `bucketBy` and sticky bucketing.
