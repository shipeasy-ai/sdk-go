Check whether the `{{RESOURCE_NAME}}` kill switch is engaged.

```go
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

if c.GetKillswitch("{{RESOURCE_NAME}}") {
    // feature is killed — short-circuit
}
```
