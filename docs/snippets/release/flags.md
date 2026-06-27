Configure once, then bind the user and read the `{{RESOURCE_NAME}}` gate.

```go
shipeasy.Configure(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})

c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})
if c.GetFlag("{{RESOURCE_NAME}}") {
    // new behaviour
}
```
