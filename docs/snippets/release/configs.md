Read the `{{CONFIG_KEY}}` dynamic config (typed JSON value). Assumes `Configure()` ran at startup — see Installation.

### Read a config

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

// GetConfig(name) — name is the config key; returns (value any, ok bool).
// ok is false when the key is absent. Type-assert value to what you stored.
if cfg, ok := c.GetConfig("{{CONFIG_KEY}}"); ok {
    m := cfg.(map[string]any) // configs are arbitrary JSON
    _ = m["cta"]
}
```

### Config with an explicit fallback

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

// GetConfigOr(name, def) — def is returned when the key is absent.
v := c.GetConfigOr("{{CONFIG_KEY}}", map[string]any{"cta": "Buy"}) // name; def
_ = v
```
