Evaluate the `{{RESOURCE_NAME}}` experiment and track the `{{SUCCESS_EVENT}}` conversion.

```go
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

r := c.GetExperiment("{{RESOURCE_NAME}}", map[string]any{"color": "blue"})
if r.InExperiment {
    p := r.Params.(map[string]any)
    render(p["color"])
}

// On conversion — the bound Client derives the user id from its attributes:
c.Track("{{SUCCESS_EVENT}}", map[string]any{"amount": 49})
```
