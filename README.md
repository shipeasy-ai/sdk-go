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
