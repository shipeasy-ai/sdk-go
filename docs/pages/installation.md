# Installation

The SDK is a standard Go module. Minimum runtime: **Go 1.21**.

```bash
go get github.com/shipeasy-ai/sdk-go
```

Import it (the package name is `shipeasy`; alias the import to match):

```go
import shipeasy "github.com/shipeasy-ai/sdk-go"
```

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

## Server key

The SDK authenticates with your project's **server** key. Keep it server-side —
never embed it in the browser. Read it from the environment:

```go
shipeasy.Configure(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})
```
