# Error reporting — `See()`

The Go SDK ships a structured error-reporting surface, `See()`, mirroring
`@shipeasy/sdk`'s `see()`. Every handled error documents its product
**consequence**, not just its stack. Reports are fire-and-forget POSTs to
`/collect` (`type:"error"`) — they never block or panic into the request path.

> Rule of thumb: *if you don't know the consequence of an error, don't handle it
> here.*

## Reporting a handled error

The chain is `See(problem).CausesThe(subject).To(outcome, extras)`. The terminal
**`.To(outcome, extras...)`** builds the event and sends it, merging each extras
map like a final `.Extras()` call (later map wins). A chain that never calls
`.To` sends nothing.

```go
if err := chargeCard(order); err != nil {
    shipeasy.See(err).
        CausesThe("checkout").
        To("use the backup processor", map[string]any{"order_id": order.ID})
}
```

### Where extras go in the chain

`CausesThe(subject)` and `.To(outcome)` are two halves of one sentence and must
stay adjacent, so pass the extras inline on the terminal:

```go
// PREFERRED — the consequence reads as one sentence:
shipeasy.See(err).CausesThe("checkout").To("use cached prices", map[string]any{"order_id": order.ID})
```

`.To` returns nothing, so extras cannot trail the terminal in Go. And never
split the sentence with `.Extras(...)` — the standalone setter still exists, but
reach for it only when you genuinely cannot pass the context inline:

```go
// WON'T COMPILE — .To returns nothing:
// shipeasy.See(err).CausesThe("checkout").To("use cached prices").Extras(m)

// WRONG — extras wedged between the subject and the outcome. You read
// "checkout … order_id … use cached prices" and lose the consequence.
// shipeasy.See(err).CausesThe("checkout").Extras(m).To("use cached prices")
```

`See` is package-level — it reports against the configuration from `Configure`,
so there is no client to thread through. Before `Configure` has run it logs a
warning and returns a no-op chain — it never panics.

## Violations (non-exception problems)

A `Violation` is a problem with no Go `error` value. The name is a stable
**fingerprint** — put variable data in `.Extras()`, never the name:

```go
shipeasy.SeeViolation("cart_total_negative").
    To("clamp to zero", map[string]any{"cart_id": cart.ID})
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
