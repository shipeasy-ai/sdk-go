# Dynamic configs

A dynamic config is a typed JSON value managed in the dashboard. Configs are
**not** user-scoped — the bound `Client` exposes them for one-stop ergonomics but
forwards straight to the engine.

## `GetConfig` — (value, ok)

```go
c := shipeasy.NewClient(acct)

cfg, ok := c.GetConfig("billing_copy")
if ok {
    m := cfg.(map[string]any)  // configs are arbitrary JSON; type-assert
    _ = m["cta"]
}
```

`GetConfig` returns `(value any, ok bool)`. `ok` is `false` when the key is
absent (or the engine isn't initialized). The value is whatever JSON the config
holds — assert to the concrete type you stored (`string`, `float64`,
`map[string]any`, `[]any`, …).

## `GetConfigOr` — explicit default

Returns the config value, or `def` when the key is absent:

```go
copy := c.GetConfigOr("billing_copy", map[string]any{"cta": "Buy"})
```

## Engine form

```go
eng := shipeasy.ConfiguredEngine()
cfg, ok := eng.GetConfig("billing_copy")
copy := eng.GetConfigOr("billing_copy", map[string]any{"cta": "Buy"})
```
