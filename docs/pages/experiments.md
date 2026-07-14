# A/B experiments (`Universe().Assign()` + `Track`)

Experiments are read by **universe**. A universe is a mutual-exclusion pool: a
unit lands in **at most one** experiment in it. `Assign()` picks that experiment
(if any) and returns the assigned group plus its resolved parameters — it is
side-effect free. The single (deduped) exposure fires **on read**: the first time
you read a param with `Assign(...).Get(field, fallback)`. Use `Peek(field,
fallback)` to read without logging one. Record a conversion with `Track`.

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
// The first enrolled Get logs the single (deduped) exposure.
func (a Assignment) Get(field string, fallback any) any

// Peek resolves a param the same way, but WITHOUT logging an exposure.
func (a Assignment) Peek(field string, fallback any) any
```

When the unit isn't enrolled (targeting/holdout/allocation), `Enrolled` is
`false`, `Group` and `Name` are `""`, and `Get(field, fallback)` returns the
universe default if there is one, else your `fallback` — so reading a param is
always safe.

Reading `Enrolled` / `Name` / `Group` never logs an exposure — only `Get` does.
Reach for `Peek` when you need a param but don't want to count the unit as
exposed (e.g. logging, debugging, or a pre-render peek).

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

`Assign()` is side-effect free; the single exposure fires **on read** — the first
enrolled `Get(...)` logs it. It is deduped per process (unit + experiment +
group) so repeated reads in a long-running server don't spam `/collect`, and
durably deduped server-side per `(unit, experiment, group)`. Read a param with
`Peek(field, fallback)` to opt out — same lookup, no exposure. Exposures are
fire-and-forget and a no-op under `ConfigureForTesting` / `ConfigureForOffline`.

See [Advanced](advanced.md) for `bucketBy` and sticky bucketing.
