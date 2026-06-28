Check whether the `{{KILLSWITCH_KEY}}` kill switch is engaged. Assumes `Configure()` ran at startup — see Installation.

### Read a kill switch

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

// GetKillswitch(name) — name is the kill-switch key; true means engaged
// (the feature is killed). Returns false if the switch is absent.
if c.GetKillswitch("{{KILLSWITCH_KEY}}") {
    // feature is killed — short-circuit
}
```

### Read a named per-key switch

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

// GetKillswitch(name, switchKey) — the optional switchKey selects a named
// per-key override (the dashboard "switches" feature). When that key has no
// override, it falls back to the kill switch's top-level value.
if c.GetKillswitch("{{KILLSWITCH_KEY}}", "eu") {
    // killed for the "eu" variant
}
```
