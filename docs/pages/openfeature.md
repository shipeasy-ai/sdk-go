# OpenFeature provider

The Go SDK ships a first-class [OpenFeature](https://openfeature.dev) provider so
apps standardized on the CNCF OpenFeature API can plug Shipeasy in as the backing
flag provider. It is a thin, pure adapter over `*shipeasy.Engine` — no change to
evaluation, resolution is local against the cached blob.

It lives in its own nested Go module so the base SDK doesn't pull in
`github.com/open-feature/go-sdk` for consumers that don't use it:

```bash
go get github.com/shipeasy-ai/sdk-go/openfeature
```

## Wiring

```go
import (
    "github.com/open-feature/go-sdk/openfeature"
    shipeasy "github.com/shipeasy-ai/sdk-go"
    shipeasyof "github.com/shipeasy-ai/sdk-go/openfeature"
)

client := shipeasy.NewEngine(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})
_ = client.Init(ctx)

_ = openfeature.SetProviderAndWait(shipeasyof.NewProvider(client))

of := openfeature.NewClient("app")
on, _ := of.BooleanValue(ctx, "new_checkout", false,
    openfeature.NewEvaluationContext("u1", nil))
```

`shipeasyof.NewProvider(*shipeasy.Engine)` returns a `*Provider` whose
`Metadata().Name` is `"shipeasy"`.

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
