Check whether the `{{RESOURCE_NAME}}` kill switch is engaged. Assumes `Configure()` ran at startup — see Installation.

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

// name; optional switchKey selects a named per-key override switch
// (the dashboard "switches" feature). Omit it for the whole kill switch.
if c.GetKillswitch("{{RESOURCE_NAME}}") {
    // feature is killed — short-circuit
}
```
