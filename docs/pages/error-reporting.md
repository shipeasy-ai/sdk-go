# Error reporting — `See()`

The Go SDK ships a structured error-reporting surface, `See()`, mirroring
`@shipeasy/sdk`'s `see()`. Every handled error documents its product
**consequence**, not just its stack. Reports are fire-and-forget POSTs to
`/collect` (`type:"error"`) — they never block or panic into the request path.

> Rule of thumb: *if you don't know the consequence of an error, don't handle it
> here.*

## Reporting a handled error

The chain is `See(problem).CausesThe(subject).Extras(map).To(outcome)`. The
terminal **`.To(outcome)`** builds the event and sends it; `CausesThe` and
`Extras` are chainable setters callable in any order before `.To`. A chain that
never calls `.To` sends nothing.

```go
if err := chargeCard(order); err != nil {
    shipeasy.See(err).
        CausesThe("checkout").
        Extras(map[string]any{"order_id": order.ID}).
        To("use the backup processor")
}
```

`See` is package-level — it reports against the configuration from `Configure`,
so there is no client to thread through. Before `Configure` has run it logs a
warning and returns a no-op chain — it never panics.

## Violations (non-exception problems)

A `Violation` is a problem with no Go `error` value. The name is a stable
**fingerprint** — put variable data in `.Extras()`, never the name:

```go
shipeasy.SeeViolation("cart_total_negative").
    Extras(map[string]any{"cart_id": cart.ID}).
    To("clamp to zero")
```

## Expected control flow — reports NOTHING

Mark an error as deliberate control flow so it is **not** reported. It only
stamps the error value (and optional local-only debug extras):

```go
if errors.Is(err, sql.ErrNoRows) {
    shipeasy.ControlFlowException(err).
        Because("because a missing row is the empty-state path").
        Extras(map[string]any{"key": key}) // local only — never transmitted
}
```

`shipeasy.IsExpected(err)` reports whether an error was marked this way (handy in
tests/assertions).

## Limits & safety

- Per-process spam guard: identical events within a 30s window collapse to one
  send; a hard cap (25/process) bounds total sends.
- Extras are sanitized: only string / finite-number / bool values, string values
  truncated to 200 chars, capped at 20 keys; `PrivateAttributes` keys are
  stripped.
- No-op on test/offline clients (local mode).
