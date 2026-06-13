# shipeasy-go

Server SDK for [Shipeasy](https://shipeasy.dev). Feature flags, configs, A/B experiments, metric tracking.

```bash
go get github.com/shipeasy-ai/sdk-go
```

```go
import (
    "context"
    shipeasy "github.com/shipeasy-ai/sdk-go"
)

c := shipeasy.NewClient(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})
if err := c.Init(context.Background()); err != nil { panic(err) }
defer c.Destroy()

if c.GetFlag("new_checkout", shipeasy.User{"user_id": "u_123"}) { ... }

cfg, _ := c.GetConfig("billing_copy")

r := c.GetExperiment("checkout_button", shipeasy.User{"user_id": "u_123"}, map[string]any{"color": "blue"})
_ = r.Group; _ = r.Params

c.Track("u_123", "purchase", map[string]any{"amount": 49})
```

## Anonymous visitors (zero-config bucketing)

For logged-out traffic you need a *stable* unit so a fractional rollout buckets
the same on the server and in the browser. `shipeasy.Middleware` mints a
first-party `__se_anon_id` cookie (shared with every Shipeasy SDK) for any
request that lacks one, and exposes it via `shipeasy.AnonID(r)`:

```go
mux := http.NewServeMux()
// ... register handlers ...
http.ListenAndServe(":8080", shipeasy.Middleware(mux))
```

```go
func handler(w http.ResponseWriter, r *http.Request) {
    user := shipeasy.User{"anonymous_id": shipeasy.AnonID(r)} // or {"user_id": ...} once logged in
    if c.GetFlag("new_checkout", user) { /* ... */ }
}
```

The cookie is non-`HttpOnly` by design — the browser SDK reads it so the client
buckets identically to the server. A request with **no** unit still resolves a
fully-rolled (100%) gate as on; only fractional gates need the id. The cookie
name and format are a cross-SDK contract; see
[`18-identity-bucketing.md`](https://github.com/shipeasy-ai/experiment-platform/blob/main/18-identity-bucketing.md).

