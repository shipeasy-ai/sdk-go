# Changelog

## Unreleased

- **Default values.** Added `GetFlagOr(name, user, def) bool` and
  `GetConfigOr(name, def) any`. The fallback is returned only when the
  flag/config *cannot* be evaluated (client not ready, or the gate/config is
  absent) — never when a gate evaluates to `false`. `GetConfig` is unchanged
  (`(any, bool)`).
- **Evaluation detail.** Added `GetFlagDetail(name, user) FlagDetail` (`Value`,
  `Reason`) and the exported reason constants `ReasonOverride`,
  `ReasonClientNotReady`, `ReasonFlagNotFound`, `ReasonOff`, `ReasonRuleMatch`,
  `ReasonDefault`. Reasons are computed at the boundary without touching the
  canonical evaluator; the per-evaluation "gate" telemetry beacon fires exactly
  once (never on an override). `GetFlag` now delegates to `GetFlagDetail`.
- **Change listeners.** Added `OnChange(fn) (cancel func())`. Registered
  listeners fire after a background poll loads new data (a `200`, not a `304`);
  a panicking listener is recovered and logged. Test/offline clients never
  poll, so they never fire.
- **Offline file data source.** Added `NewOfflineClient(path)` and
  `NewOfflineClientFromSnapshot(flags, experiments)`. Both build a no-network
  client preloaded from a `{ "flags": …, "experiments": … }` snapshot;
  `Init`/`InitOnce`/`Track` are no-ops, evaluations run the real evaluator
  against the snapshot, and `Override*` setters apply on top.
- **Local-override test utility.** Added `shipeasy.NewTestClient()`, a
  no-network, immediately-usable client (telemetry disabled, `Init`/`InitOnce`
  no-op, `Track` no-op, no API key required) for unit tests. New override
  setters `OverrideFlag`, `OverrideConfig`, `OverrideExperiment`, and
  `ClearOverrides` (also usable on a normal client) let tests seed every entity;
  an override always wins over fetched data in `GetFlag`/`GetConfig`/
  `GetExperiment`. See the README "Testing" section.

## 0.3.0

- **Anonymous bucketing (`__se_anon_id`).** Added `shipeasy.Middleware`, a
  zero-dependency `net/http` middleware that mints the shared `__se_anon_id`
  first-party cookie for any request without one and exposes the resolved id via
  `shipeasy.AnonID(r)`. Anonymous visitors now bucket consistently across server
  renders and the browser. Lower-level primitives `MintAnonID`,
  `ReadOrMintAnonID`, and `SetAnonIDCookie` are exported for custom HTTP stacks.
  Implements the cross-SDK contract in `18-identity-bucketing.md`.
- **Eval fix (no-unit gate rule).** A request with no `user_id`/`anonymous_id`
  now resolves a fully-rolled (100%) gate as **on** instead of always off; a
  fractional gate is still off until a stable unit exists. Brings Go in line
  with the TypeScript reference SDK. Targeting rules are still evaluated first.

## 0.2.0

- Per-evaluation usage telemetry (fire-and-forget, on by default).

## 0.1.0

- Initial release: feature flags, configs, experiments, metric tracking.
