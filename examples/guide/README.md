# Shipeasy · Go Entity Guide

A single-page Go + [Gin](https://github.com/gin-gonic/gin) web app that renders
like a guide document: **one styled card per Shipeasy entity** — feature flag,
dynamic config, A/B experiment, kill switch, event/metric, i18n label, and
error reporting (`see()`). Each card shows the entity's key, a one-line
explanation, the real SDK call, and a sample resolved value.

It runs standalone with **zero external services** and makes **no network
calls** — the only dependency is Gin.

## SDK wiring

This example now imports `github.com/shipeasy-ai/sdk-go` and configures it once
at startup with `SHIPEASY_SERVER_KEY`. The flag, config, experiment, and kill
switch cards are evaluated through the installed SDK version, while the event
and `see()` cards still show the call shape in the guide.

## Run it

```bash
go run .
```

Then open <http://localhost:8080>.

(The first run downloads Gin via `go mod tidy` / the build — that network access
is for the module cache only; the app itself calls nothing.)

## Next step — make the values live

1. Install the SDK:

   ```bash
   go get github.com/shipeasy-ai/sdk-go
   ```

2. In `main.go`, construct a client once at startup:

   ```go
   import shipeasy "github.com/shipeasy-ai/sdk-go"

   shipeasy.Configure(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})
   c := shipeasy.NewClient(shipeasy.User{"user_id": "u_123"})
   ```

3. Follow the README that ships with the installed SDK version for the exact
   method names and startup shape. In v0.8.0 the bound client exposes
   `GetFlag`, `GetConfig`, `GetExperiment`, `GetKillswitch`, and the package
   exposes `ConfiguredEngine().Track(...)` plus `shipeasy.See(...)`.

## Files

```
examples/guide/
├── go.mod              # module "guide" — its own module, requires Gin only
├── main.go             # builds the entity slice, loads the template, serves GET /
├── templates/
│   └── guide.html      # the single page + inline dark-brand CSS
├── README.md
└── .gitignore
```

Docs: <https://docs.shipeasy.ai>
