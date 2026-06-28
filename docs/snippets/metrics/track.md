Track a metric/conversion event from the bound `Client`. Metrics in the dashboard
are computed from these events. Assumes `Configure()` ran at startup — see
Installation.

### Track an event

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

// Track(event, props)
//   event — the event your metric is built on (required)
//   props — optional payload; numeric/string fields you can sum/filter on in a
//           metric (private attributes are stripped before egress)
c.Track("{{EVENT_NAME}}", map[string]any{"amount": 49, "currency": "usd"})
```

Fire-and-forget (never blocks your response) and a no-op under
`ConfigureForTesting` / `ConfigureForOffline`. The unit is the bound user
(`user_id`, else `anonymous_id`); with no unit the call is a no-op.

### Track without properties

```go
// construct once per callsite (cheap; binds the user)
c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})

c.Track("{{EVENT_NAME}}", nil) // props are optional — pass nil
```
