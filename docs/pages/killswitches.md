# Kill switches

A kill switch is an operational on/off you can flip from the dashboard to
instantly disable a feature. Kill switches ride the **flags blob** alongside
gates, so reading one is local with no extra fetch. They are not user-scoped.

## `GetKillswitch`

```go
c := shipeasy.NewClient(acct)

paused := c.GetKillswitch("payments_paused")  // bool — true means the switch is engaged
if paused {
    // feature is killed
}
```

`true` means the switch is **engaged** (the feature is killed). It returns
`false` when the engine isn't initialized or the switch is absent.

## Named per-key "switches"

The dashboard "switches" feature lets one kill switch carry named per-key
overrides. Pass an optional `switchKey` to read a specific one; it falls back to
the kill switch's top-level value when that key has no override:

```go
killedForEU := c.GetKillswitch("payments_paused", "eu")
```

## Engine form

```go
eng := shipeasy.ConfiguredEngine()
paused := eng.GetKillswitch("payments_paused", "")     // "" = no named switch
killedForEU := eng.GetKillswitch("payments_paused", "eu")
```

> Note: a gate's own kill state is also folded into gate evaluation — a
> killswitched gate reads `false` from `GetFlag` with reason `OFF`. `GetKillswitch`
> is for standalone kill-switch resources.
