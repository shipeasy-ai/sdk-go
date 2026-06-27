Read the `{{RESOURCE_NAME}}` dynamic config (typed JSON value). Assumes `Configure()` ran at startup — see Installation.

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

// name; returns (value any, ok bool) — ok is false when the key is absent.
if cfg, ok := c.GetConfig("{{RESOURCE_NAME}}"); ok {
    m := cfg.(map[string]any)
    _ = m["cta"]
}

// Or with an explicit fallback returned when the key is absent:
v := c.GetConfigOr("{{RESOURCE_NAME}}", map[string]any{"cta": "Buy"}) // name; def
_ = v
```
