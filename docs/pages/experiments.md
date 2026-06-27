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

## Bound `Client` form

```go
c := shipeasy.NewClient(acct)

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

## Engine form

The engine method takes an explicit `user`:

```go
eng := shipeasy.ConfiguredEngine()
user := shipeasy.User{"user_id": "u_123"}
r := eng.GetExperiment("checkout_button", user, map[string]any{"color": "blue"})
```

When the user is not enrolled, `Group` is `"control"` and `Params` falls back to
the `defaultParams` you passed.

## Tracking conversions with `Track`

To measure an experiment you log a conversion event with `Engine.Track`. The
event is fire-and-forget POSTed to `/collect`:

```go
eng := shipeasy.ConfiguredEngine()
eng.Track("u_123", "{{SUCCESS_EVENT}}", map[string]any{"amount": 49})
```

`Track(userID, eventName, properties)` takes a bare user id, the event name, and
an optional property bag. (`Track` is a no-op on test/offline clients and when
the engine is in local mode.)

## Manual exposure

The server is stateless and never auto-logs an exposure. When you actually
present the treatment, call `LogExposure` (or `LogExposureUser` for `bucketBy` /
anonymous traffic) — see [Advanced](advanced.md).
