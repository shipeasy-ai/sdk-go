# OpenFeature provider

The Go SDK ships a first-class [OpenFeature](https://openfeature.dev) provider so
apps standardized on the CNCF OpenFeature API can plug Shipeasy in as the backing
flag provider. It is a thin, pure adapter over the configured engine — no change
to evaluation, resolution is local against the cached blob.

It lives in its own nested Go module so the base SDK doesn't pull in
`github.com/open-feature/go-sdk` for consumers that don't use it:

```bash
go get github.com/shipeasy-ai/sdk-go/openfeature
```

## Wiring

`Configure` Shipeasy as usual, then register `NewGlobalProvider()` — it resolves
the engine `Configure` already built, so OpenFeature is wired without ever naming
the engine:

```go
import (
    "github.com/open-feature/go-sdk/openfeature"
    shipeasy "github.com/shipeasy-ai/sdk-go"
    shipeasyof "github.com/shipeasy-ai/sdk-go/openfeature"
)

// Once, at process start.
shipeasy.Configure(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})

// Register Shipeasy as OpenFeature's backing provider.
_ = openfeature.SetProviderAndWait(shipeasyof.NewGlobalProvider())

of := openfeature.NewClient("app")
on, _ := of.BooleanValue(ctx, "new_checkout", false,
    openfeature.NewEvaluationContext("u1", nil))
```

`NewGlobalProvider()` returns a `*Provider` whose `Metadata().Name` is
`"shipeasy"`. It **panics** if `Configure` has not been called first.

## Type mapping

- **Boolean flags** resolve through gate evaluation (`GetFlagDetail`).
- **String / Float / Int / Object flags** resolve through dynamic **configs**
  (`GetConfig`). A present-but-wrong-type config yields a `TYPE_MISMATCH` error;
  an absent key yields the default with reason `DEFAULT`.

## Evaluation context → user

`TargetingKey` becomes `user_id`; every other context entry is carried through
verbatim as a targeting attribute.

## Reason mapping (boolean flags)

| Shipeasy reason | OpenFeature reason |
| --- | --- |
| `RULE_MATCH` | `TARGETING_MATCH` |
| `DEFAULT` | `DEFAULT` |
| `OFF` | `DISABLED` |
| `OVERRIDE` | `STATIC` |
| `FLAG_NOT_FOUND` | `ERROR` + `FlagNotFound` |
| `CLIENT_NOT_READY` | `ERROR` + `ProviderNotReady` |

On any error-mapped reason the default value is returned with the resolution
error set.
