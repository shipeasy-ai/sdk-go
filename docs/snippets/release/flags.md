Read the `{{RESOURCE_NAME}}` gate for the bound user. Assumes `Configure()` ran at startup — see Installation.

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

if c.GetFlag("{{RESOURCE_NAME}}") { // gate name; returns false if absent/disabled
    // new behaviour
}

// Or with an explicit fallback when the gate can't be evaluated
// (engine not ready, or the gate is absent — NOT when it evaluates false):
on := c.GetFlagOr("{{RESOURCE_NAME}}", true) // name; def returned only on can't-evaluate
_ = on
```
