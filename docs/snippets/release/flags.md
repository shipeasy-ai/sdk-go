Read the `{{FLAG_KEY}}` gate for the bound user. Assumes `Configure()` ran at startup — see Installation.

### Read a flag

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

// GetFlag(name) — name is the gate key; returns false if the gate is
// absent, disabled, or killswitched (never a user argument — it's bound).
if c.GetFlag("{{FLAG_KEY}}") {
    // new behaviour
}
```

### Flag with an explicit fallback

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

// GetFlagOr(name, def) — def is returned ONLY when the flag can't be
// evaluated (engine not ready, or the gate is absent); a gate that
// evaluates false returns false.
on := c.GetFlagOr("{{FLAG_KEY}}", true) // name; def returned only on can't-evaluate
_ = on
```
