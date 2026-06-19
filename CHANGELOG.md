# Changelog

## Unreleased

- **Private attributes.** Added the `PrivateAttributes []string` client option.
  Listed keys are stripped from every outbound `/collect` event's `properties`
  in `Track` (and from `LogExposure` payloads, which carry no caller props).
  Server evaluation is local, so private attrs never egress for evaluation
  either — the only egress was `Track`. Matches LD/Statsig `privateAttributes`.
- **Manual exposure logging.** Added `LogExposure(userID, experimentName)` and
  `LogExposureUser(user, experimentName)`. The server is stateless and never
  auto-logs; call these at the decision point. The experiment is re-evaluated
  for the user and, if enrolled, a single `{type:"exposure", experiment, group,
  user_id, ts}` event is POSTed to `/collect`. No-op when not enrolled (or in
  test/offline mode).
- **Sticky bucketing.** Added the `StickyBucketStore` interface
  (`Get(unit) map[experiment]StickyEntry`, `Set(unit, experiment, entry)`),
  the `StickyEntry` value (`G` group, `S` 8-char salt prefix), the built-in
  `NewInMemoryStickyStore(seed…)`, and the `StickyStore` client option. When a
  store is supplied, experiment eval — after the holdout, before allocation —
  honors a stored entry whose salt prefix still matches: it skips the allocation
  gate and returns the stored group (so a shrinking allocation keeps an enrolled
  user in and a weight change can't reshuffle them). A salt change moves the
  prefix and forces a re-bucket + overwrite; a now-missing group falls through.
  Absent a store, assignment is purely deterministic (unchanged). The bucketing
  unit is the `bucketBy`-resolved identifier.
- **Experiment `bucketBy`.** Experiment evaluation now honors a per-experiment
  `bucketBy` attribute (camelCase JSON, matching the KV blob). When set and the
  user carries a non-empty string (or numeric) value for it, that value is the
  bucketing unit — so a whole org keyed on `company_id` lands on one variant —
  driving the holdout, allocation, and group hashes alike. Absent/empty
  `bucketBy` (or a missing attribute) falls back to `user_id ?? anonymous_id`,
  matching gate bucketing. Mirrors the canonical `pickIdentifier` in
  `@shipeasy/core`.
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
