# Installation

The SDK is a standard Go module. Minimum runtime: **Go 1.21**.

```bash
go get github.com/shipeasy-ai/sdk-go
```

Import it (the package name is `shipeasy`; alias the import to match):

```go
import shipeasy "github.com/shipeasy-ai/sdk-go"
```

## Configure once, bind per request

`Configure` is the front door — call it **once** at process start. It builds one
process-wide `Engine` (which owns the api key, blob cache, poll timer and
telemetry) and kicks off a fire-and-forget one-shot fetch, so the first
`NewClient(user).GetFlag()` resolves against real rules without an explicit init.
Then build a cheap user-bound `Client` per request and call with **no** user
argument.

```go
shipeasy.Configure(shipeasy.Options{
    // Server key. Keep it server-side — never embed it in the browser.
    APIKey: os.Getenv("SHIPEASY_SERVER_KEY"),

    // Optional: maps YOUR user value (any shape) to the Shipeasy attribute
    // map used for every evaluation. Applied ONCE in NewClient. Omit it and
    // the value passed to NewClient is used as-is (a shipeasy.User / map).
    Attributes: func(u any) shipeasy.User {
        acct := u.(*Account)
        return shipeasy.User{"user_id": acct.ID, "plan": acct.Plan}
    },
})

// Per request — bind the user once, then call with NO user argument:
c := shipeasy.NewClient(acct)        // acct is your own *Account
if c.GetFlag("new_checkout") { /* ... */ }
```

`Configure` is **first-config-wins** (idempotent) and returns the global
`*Engine`; `ConfiguredEngine()` fetches it later. `NewClient` **panics** if
`Configure` has not run. The full `Options` table (`BaseURL`, `Env`,
`DisableTelemetry`, `PrivateAttributes`, `StickyStore`, …) and the init/poll vs
one-shot semantics live on the [Configuration](configuration.md) page.

### Server key from the environment

The SDK authenticates with your project's **server** key. Read it from the
environment — never hard-code it:

```bash
export SHIPEASY_SERVER_KEY="sk_server_..."
```

## Anonymous visitors — `Middleware` + `AnonID`

For logged-out traffic you need a *stable* unit so a fractional rollout buckets
the same on the server and in the browser. `shipeasy.Middleware` mints a
first-party `__se_anon_id` cookie (a cross-SDK contract — the browser SDK reads
the same cookie) for any request that lacks one and exposes it via
`shipeasy.AnonID(r)`. Wrap your router once; in each handler pass the id as the
bucketing unit when the visitor is logged out:

```go
user := shipeasy.User{"anonymous_id": shipeasy.AnonID(r)} // or {"user_id": ...} once logged in
```

The framework sections below show exactly where `Middleware` and `Configure` go.

## net/http

```go
package main

import (
    "net/http"
    "os"

    shipeasy "github.com/shipeasy-ai/sdk-go"
)

func main() {
    // Once, at process start.
    shipeasy.Configure(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})

    mux := http.NewServeMux()
    mux.HandleFunc("/", handler)

    // Wrap the mux so every request gets the __se_anon_id cookie.
    http.ListenAndServe(":8080", shipeasy.Middleware(mux))
}

func handler(w http.ResponseWriter, r *http.Request) {
    // Bind the user once per request (cheap).
    c := shipeasy.NewClient(shipeasy.User{"anonymous_id": shipeasy.AnonID(r)})
    if c.GetFlag("new_checkout") {
        // new behaviour
    }
}
```

## Gin

`shipeasy.Middleware` is a standard `func(http.Handler) http.Handler`, so adapt
it with `gin.WrapH`, or read the id directly from the request inside a handler.

```go
package main

import (
    "net/http"
    "os"

    "github.com/gin-gonic/gin"
    shipeasy "github.com/shipeasy-ai/sdk-go"
)

func main() {
    shipeasy.Configure(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})

    r := gin.Default()

    // Run the anon-id middleware as a Gin middleware: mint/read the cookie,
    // then hand the (mutated) request to the next handler in the chain.
    r.Use(func(ctx *gin.Context) {
        shipeasy.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
            ctx.Request = req // carries the resolved AnonID in its context
        })).ServeHTTP(ctx.Writer, ctx.Request)
        ctx.Next()
    })

    r.GET("/", func(ctx *gin.Context) {
        c := shipeasy.NewClient(shipeasy.User{"anonymous_id": shipeasy.AnonID(ctx.Request)})
        if c.GetFlag("new_checkout") {
            // new behaviour
        }
        ctx.String(http.StatusOK, "ok")
    })

    r.Run(":8080")
}
```

## Echo

```go
package main

import (
    "net/http"
    "os"

    "github.com/labstack/echo/v4"
    shipeasy "github.com/shipeasy-ai/sdk-go"
)

func main() {
    shipeasy.Configure(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})

    e := echo.New()

    // Echo accepts a standard net/http middleware via echo.WrapMiddleware.
    e.Use(echo.WrapMiddleware(shipeasy.Middleware))

    e.GET("/", func(ctx echo.Context) error {
        c := shipeasy.NewClient(shipeasy.User{"anonymous_id": shipeasy.AnonID(ctx.Request())})
        if c.GetFlag("new_checkout") {
            // new behaviour
        }
        return ctx.String(http.StatusOK, "ok")
    })

    e.Start(":8080")
}
```

## Chi

Chi's middleware signature *is* `func(http.Handler) http.Handler`, so
`shipeasy.Middleware` plugs straight in:

```go
package main

import (
    "net/http"
    "os"

    "github.com/go-chi/chi/v5"
    shipeasy "github.com/shipeasy-ai/sdk-go"
)

func main() {
    shipeasy.Configure(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})

    r := chi.NewRouter()
    r.Use(shipeasy.Middleware) // plugs in directly — no adapter needed

    r.Get("/", func(w http.ResponseWriter, req *http.Request) {
        c := shipeasy.NewClient(shipeasy.User{"anonymous_id": shipeasy.AnonID(req)})
        if c.GetFlag("new_checkout") {
            // new behaviour
        }
        w.Write([]byte("ok"))
    })

    http.ListenAndServe(":8080", r)
}
```

## init / poll vs one-shot

`Configure` does a one-shot background fetch — no polling. A long-running server
that wants live updates calls `Init` on the configured engine to start the
background poll loop:

```go
eng := shipeasy.ConfiguredEngine()
if err := eng.Init(context.Background()); err != nil { panic(err) } // start polling
defer eng.Destroy()
```

See [Configuration](configuration.md) for `Init` / `InitOnce` / `Destroy` and the
full `Options` reference.

## OpenFeature provider (optional, separate module)

The OpenFeature provider lives in its own nested module so the base SDK does not
pull in `github.com/open-feature/go-sdk` for consumers that don't use it:

```bash
go get github.com/shipeasy-ai/sdk-go/openfeature
```

```go
import shipeasyof "github.com/shipeasy-ai/sdk-go/openfeature"
```

See the [OpenFeature](openfeature.md) page for wiring.
