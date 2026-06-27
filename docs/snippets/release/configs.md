Read the `{{RESOURCE_NAME}}` dynamic config (typed JSON value).

```go
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

if cfg, ok := c.GetConfig("{{RESOURCE_NAME}}"); ok {
    m := cfg.(map[string]any)
    _ = m["cta"]
}
```
