Assign a unit within the `{{EXPERIMENT_KEY}}` universe (a mutual-exclusion pool — the unit lands in <=1 experiment), read the assigned params, then record the `{{SUCCESS_EVENT}}` conversion on the same bound `Client`. Assumes `Configure()` ran at startup — see Installation.

### Assign within a universe and render the assigned group

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

// Universe(name).Assign() → Assignment
//   name  — the UNIVERSE name (not an experiment); the unit lands in <=1 experiment
//   a.Name     — the experiment the unit landed in, or "" when not enrolled
//   a.Group    — the assigned variant, or "" when not enrolled
//   a.Enrolled — true iff enrolled (Group != "")
//   a.Get(field, fallback) — variant override ?? universe default ?? fallback
a := c.Universe("{{EXPERIMENT_KEY}}").Assign()
render(a.Get("color", "blue")) // always safe — falls back when not enrolled
```

### Track the conversion

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

// Track(event, props) — Client-only; the unit id is derived from the bound
// attributes (user_id, else anonymous_id). event is the metric event name;
// props are optional numeric/string fields. Fire-and-forget.
c.Track("{{SUCCESS_EVENT}}", map[string]any{"amount": 49}) // event; props
```
