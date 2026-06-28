Report a caught, handled error (or a non-exception "violation") to Shipeasy with
`See()` — fire-and-forget, never panics into the request path. Package-level, so
it reports against the configuration from `Configure`. Assumes `Configure()` ran
at startup — see Installation.

### Report a handled error

```go
if err := charge(order); err != nil {
    // CausesThe(subject) — what the error affects (e.g. "checkout")
    // To(outcome)        — the terminal: what you do about it; builds + fires once
    shipeasy.See(err).CausesThe("checkout").To("use the backup processor")
    fallbackCharge(order)
}
```

### Attach context with `.Extras(...)`

```go
if err := charge(order); err != nil {
    // Extras(map) — structured fields attached to the report (sanitized:
    // string/number/bool only; private attributes stripped before egress).
    shipeasy.See(err).
        CausesThe("checkout").
        Extras(map[string]any{"order_id": order.ID}).
        To("use cached prices")
}
```

### Report a non-exception violation

```go
// A bad state that isn't an error — the name is a STABLE fingerprint; put
// variable data in .Extras, never the name. .To() is the terminal.
shipeasy.SeeViolation("missing_invoice").
    CausesThe("billing").
    To("skip the dunning email")
```

### Mark an expected error — report NOTHING

```go
if errors.Is(err, sql.ErrNoRows) {
    // transmits nothing; .Because(...) / .Extras() are local-debug only
    shipeasy.ControlFlowException(err).Because("a missing row is the empty-state path")
}
```
