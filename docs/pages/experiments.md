# A/B experiments (`Universe().Assign()` + `Track`)

Experiments are read by **universe**. A universe is a mutual-exclusion pool: a
unit lands in **at most one** experiment in it. `Assign()` picks that experiment
(if any), returns the assigned group plus its resolved parameters, and auto-logs
a single (deduped) exposure. You read parameters with
`Assign(...).Get(field, fallback)` and record a conversion with `Track`.

## Read an experiment

```go
c := shipeasy.NewClient(acct)            // bind the user once (cheap)

// Ask the UNIVERSE, not the experiment: the unit lands in <=1 experiment in it.
a := c.Universe("hero_cta").Assign()

// Read a param: variant override ?? universe default ?? your fallback.
render(a.Get("primary_label", "Sign up"))
```

On the bound `Client` the user is bound at construction, so `Assign()` takes no
argument. (The heavyweight `Engine` form is `engine.Universe(name).Assign(user)`,
for advanced use.)

## `Assignment`

```go
type Assignment struct {
    Name     string // the experiment the unit landed in, or "" when not enrolled
    Group    string // the assigned variant, or "" when not enrolled
    Enrolled bool   // true iff enrolled (Group != "")
}

// Get resolves a param: variant override, else the universe default, else fallback.
func (a Assignment) Get(field string, fallback any) any
```

When the unit isn't enrolled (targeting/holdout/allocation), `Enrolled` is
`false`, `Group` and `Name` are `""`, and `Get(field, fallback)` returns the
universe default if there is one, else your `fallback` — so reading a param is
always safe.

```go
a := c.Universe("hero_cta").Assign()
if a.Enrolled {
    // a.Group is the variant, e.g. "treatment"
}
label := a.Get("primary_label", "Sign up") // never panics
```

## Track conversions

Record the success event so the analysis pipeline can compute lift. Conversion
events are attributed to the bound user. You already have a `Client` — call
`Track` on the **same handle**, so an experiment is end-to-end Client-only:

```go
c := shipeasy.NewClient(acct)
// ... present the treatment from c.Universe(...).Assign() ...
c.Track("{{SUCCESS_EVENT}}", map[string]any{"amount": 49})
```

`Client.Track(event, props)` takes the event name and an optional property bag;
the unit id is derived from the bound attribute map (`user_id`, else
`anonymous_id`). `Track` is fire-and-forget (never blocks your response) and a
no-op under `ConfigureForTesting` / `ConfigureForOffline`.

## Exposure logging

By default `Assign()` auto-logs a single exposure when the unit is enrolled,
deduped per process (unit + experiment + group) so repeated calls in a
long-running server don't spam `/collect`. Exposures are fire-and-forget and a
no-op under `ConfigureForTesting` / `ConfigureForOffline`.

See [Advanced](advanced.md) for `bucketBy` and sticky bucketing.
