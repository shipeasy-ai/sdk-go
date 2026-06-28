Evaluate the `{{EXPERIMENT_KEY}}` experiment and track the `{{SUCCESS_EVENT}}` conversion. Assumes `Configure()` ran at startup — see Installation.

### Evaluate and render the assigned group

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

// GetExperiment(name, defaultParams) — name is the experiment key;
// defaultParams is returned as r.Params when the user isn't enrolled.
r := c.GetExperiment("{{EXPERIMENT_KEY}}", map[string]any{"color": "blue"})
if r.InExperiment {
    p := r.Params.(map[string]any)
    render(p["color"])
}
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
