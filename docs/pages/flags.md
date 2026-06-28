# Feature flags (gates)

A flag ("gate") evaluates to a `bool` for a user. Evaluation is local against the
cached blob — no network call.

## Reading a flag

After `Configure` + `NewClient(user)`, the user is already bound, so the methods
take no user argument:

```go
c := shipeasy.NewClient(acct)            // bind the user once

on := c.GetFlag("new_checkout")          // bool

// Or with an explicit fallback (see "Defaults" below):
on = c.GetFlagOr("new_checkout", true)

// Or the full detail:
d := c.GetFlagDetail("new_checkout")
_ = d.Value   // bool
_ = d.Reason  // string, see below
```

## Boolean semantics & defaults

Go has no default arguments, so the SDK ships an `…Or` variant taking an explicit
fallback. **The fallback is returned only when the flag CANNOT be evaluated —
never when it evaluates to `false`:**

```go
// def is returned ONLY when the engine isn't ready (CLIENT_NOT_READY) or the
// gate is absent (FLAG_NOT_FOUND). A gate that evaluates to false returns false.
on := c.GetFlagOr("new_checkout", true)
```

`GetFlag` is exactly `GetFlagDetail(...).Value`. When the engine isn't
initialized or the gate is missing, `GetFlag` returns `false`.

## Evaluation detail & reasons

`GetFlagDetail` returns the value plus a stable, exported reason:

```go
d := c.GetFlagDetail("new_checkout")
// d.Value  bool
// d.Reason one of:
//   shipeasy.ReasonOverride       "OVERRIDE"          (a local Override* won)
//   shipeasy.ReasonClientNotReady "CLIENT_NOT_READY"  (Init not done; value=false)
//   shipeasy.ReasonFlagNotFound   "FLAG_NOT_FOUND"    (no such gate; value=false)
//   shipeasy.ReasonOff            "OFF"               (gate disabled/killswitched)
//   shipeasy.ReasonRuleMatch      "RULE_MATCH"        (evaluated true)
//   shipeasy.ReasonDefault        "DEFAULT"           (evaluated false)
```

`GetFlagOr` returns `def` exactly when the reason is `CLIENT_NOT_READY` or
`FLAG_NOT_FOUND`.
