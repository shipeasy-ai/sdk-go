Evaluate the `{{RESOURCE_NAME}}` experiment and track the `{{SUCCESS_EVENT}}` conversion. Assumes `Configure()` ran at startup — see Installation.

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

// name; defaultParams returned as r.Params when the user isn't enrolled.
r := c.GetExperiment("{{RESOURCE_NAME}}", map[string]any{"color": "blue"})
if r.InExperiment {
    p := r.Params.(map[string]any)
    render(p["color"])
}

// On conversion — Track is Client-only (derives the user id from the bound
// attributes). event name; props are optional metric properties.
c.Track("{{SUCCESS_EVENT}}", map[string]any{"amount": 49}) // event; props
```
