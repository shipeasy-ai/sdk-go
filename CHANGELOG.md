# Changelog

## 0.11.0

- **Optional Admin API client** — a new opt-in `admin` module
  (`github.com/shipeasy-ai/sdk-go/admin`) for *administering* resources (create
  gates, start experiments, manage configs/killswitches/universes/metrics/events,
  …) from server code. It is a raw client **generated from the Shipeasy OpenAPI
  spec** (1:1 with the REST API — id-based, basis-points, snake_case; no name→id
  or percent→bp ergonomics, which stay in the CLI/MCP).
  - A **separate Go module**, so the base SDK never depends on it. Opt in with
    `go get github.com/shipeasy-ai/sdk-go/admin` (mirrors the nested `openfeature`
    module).
  - `admin.NewClient(apiKey, admin.WithProjectID(...))` wires bearer auth +
    `X-Project-Id` scoping (base URL defaults to `https://shipeasy.ai`); the
    resource groups are reached as `client.GatesAPI`, `client.ExperimentsAPI`, …
    (gates, configs, killswitches, experiments, universes, metrics, events,
    alertRules, attributes, projects, ops, i18n).
  - Regenerate after a contract change: refresh `admin/openapi.json` then run
    `bash scripts/gen_admin.sh` (only generated files are rewritten; the
    `NewClient` shim is preserved). Generator pinned via `openapitools.json`.

## 0.10.0

The uniform SDK DX standard (experiment-platform doc 23). The documented surface
is now exactly `Configure()` (+ the test/offline siblings) and the bound
`NewClient(user)`; the `Engine` stays public (`NewEngine`) but undocumented.

### Added

- **`ConfigureForTesting(TestOptions{...})`** — no api key, zero network; seeds
  flags/configs/experiments overrides and registers the global engine so the
  bound `NewClient(user)` reads them. **Replaces** prior config (unlike
  `Configure`'s first-config-wins) so a test suite can reconfigure between cases.
- **`ConfigureForOffline(OfflineOptions{...})`** — evaluates the **real** rules
  from an in-memory `Snapshot` or a JSON `Path`, with overrides layered on top;
  also replaces prior config.
- **`Options{Poll, NoInitialFetch}`** — `Poll: true` starts the background poll
  internally (you never call `Init` yourself); the default is a one-shot
  fire-and-forget fetch; `NoInitialFetch` is the init=false escape hatch.
- **Package-level helpers** so the docs never name the `Engine`: `OverrideFlag`,
  `OverrideConfig`, `OverrideExperiment`, `ClearOverrides`, `OnChange`,
  `BootstrapScriptTag`, `I18nScriptTag` — delegating to the configured global.
- **`openfeature.NewGlobalProvider()`** — resolves the engine built by
  `Configure()`, so OpenFeature is wired without naming the `Engine`.
- **`cmd/shipeasy-skill`** — the opt-in installer
  (`go install …/cmd/shipeasy-skill@latest && shipeasy-skill install` / `print`)
  that copies the bundled agent skill into a consumer's project.

### Changed

- `README.md` is now **generated** from `docs/` by `internal/genreadme` (which
  also keeps the embedded `cmd/shipeasy-skill/SKILL.md` in sync); CI enforces it.
  The docs were rewritten Engine-free around `Configure()` + `NewClient`, with new
  `metrics/track` + `ops/see` snippet groups and specific placeholders.

## 0.9.0

- Add `Track()`/`LogExposure()` to the bound `Client` (experiments are now
  end-to-end Client-only; the Engine forms remain for advanced use). The bound
  `Client` already holds the resolved attribute map, so:
  - `Client.Track(event string, props map[string]any)` derives the unit id from
    the bound attributes (`user_id`, else `anonymous_id`) and forwards to
    `Engine.Track`. No user argument.
  - `Client.LogExposure(experiment string)` re-evaluates the experiment against
    the bound attributes and forwards to `Engine.LogExposureUser` (so `bucketBy`
    and `anonymous_id` traffic resolve correctly). No user argument.

## 0.8.0

- **BREAKING — `configure()` + user-bound `Client(user)` front door.** The
  heavyweight type formerly named `Client` is **renamed to `Engine`**, and its
  constructor `NewClient(Options)` is now `NewEngine(Options)`. The name `Client`
  is now a **lightweight, user-bound handle** built with `NewClient(user any)`.
  - New `shipeasy.Configure(Options) *Engine`: builds one process-wide `Engine`
    from the api key + options (first-config-wins, idempotent), stores the
    optional `Options.Attributes` transform, and fires a background one-shot
    fetch so the first bound call resolves against real rules without an explicit
    init. Also registers the engine as the default backing package-level `See()`.
  - New `Options.Attributes func(any) shipeasy.User`: maps the caller's own user
    value (any shape) to the Shipeasy attribute map. Default = identity (the
    value passed to `NewClient` is assumed to already be a `User`/`map[string]any`).
  - New bound `Client` (built via `NewClient(user)`): runs the `Attributes`
    transform once at construction and exposes `GetFlag(name)`,
    `GetFlagOr(name, def)`, `GetFlagDetail(name)`, `GetConfig(name)`,
    `GetConfigOr(name, def)`, `GetExperiment(name, defaultParams)` and
    `GetKillswitch(name [, switchKey])` with **no user argument**. It opens no
    connection and runs no poll timer — it delegates to the configured `Engine`.
    `NewClient(user)` **panics** if `Configure` was not called first.
  - New `shipeasy.ConfiguredEngine() *Engine` accessor for the global engine.
  - New `Engine.GetKillswitch(name, switchKey string) bool` (parity with the
    Python/Ruby engine): reads the flags blob's `killswitches` map, honouring a
    named per-key override switch. `flagsBlob` now decodes a `killswitches` map.
  - The `see()` default wiring is renamed `SetDefaultEngine` /
    `defaultEngine` (was `SetDefaultClient`); both `NewEngine` and `Configure`
    register the last-constructed engine as the default.
  - `NewTestClient`, `NewOfflineClient`, `NewOfflineClientFromSnapshot` keep
    their names but now return `*Engine`. The OpenFeature provider's
    `NewProvider` now takes `*shipeasy.Engine`.

  **Migration:** `shipeasy.NewClient(shipeasy.Options{...})` →
  `shipeasy.NewEngine(shipeasy.Options{...})`; `shipeasy.SetDefaultClient` →
  `shipeasy.SetDefaultEngine`. Prefer the new
  `shipeasy.Configure(...)` + `shipeasy.NewClient(user).GetFlag("name")` flow.

## 0.7.0

- **SSR bootstrap script-tag helpers.** New `Evaluate(user)` batch-evaluate
  (every gate/config/experiment → a `{Flags, Configs, Experiments,
  Killswitches}` payload) plus `BootstrapScriptTag` and `I18nScriptTag`, which
  emit the cross-platform declarative `<script>` tags carrying the SSR payload as
  `data-*` attributes. The static `se-bootstrap.js` loader hydrates
  `window.__SE_BOOTSTRAP` and writes the `__se_anon_id` cookie so the browser
  buckets identically to the server. **No SDK key is embedded** in the bootstrap
  tag.

## 0.6.0

- **`see()` structured error reporting.** Added the `see()` grammar (parity with
  `@shipeasy/sdk` and the Python SDK) for documenting the product *consequence*
  of a handled error, not just its stack. Both an instance API
  (`client.See(err)`, `client.SeeViolation(name)`,
  `client.ControlFlowException(err)`) and package-level functions (`shipeasy.See`,
  `shipeasy.SeeViolation`, `shipeasy.ControlFlowException`) backed by a default
  client registered on `NewClient` (last constructed wins; override with
  `shipeasy.SetDefaultClient`). A global `See()` before any client logs a warning
  and returns a no-op chain (never panics). Grammar:
  `See(err).CausesThe("checkout").Extras(map[string]any{...}).To("use cached prices")`
  — `.To(outcome)` is the terminal that builds the `type:"error"` event and
  fire-and-forgets a POST to `/collect` (idempotent; a second `.To()` is a no-op).
  `SeeViolation` reports a `kind:"violation"` event (no stack). For a Go `error`,
  `error_type` is the concrete type name (`%T`) and a stack is captured via
  `runtime/debug.Stack()`. `ControlFlowException(err).Because(reason).Extras(...)`
  marks the error as expected control flow (queryable via `shipeasy.IsExpected`)
  and reports nothing. Extras are sanitized (≤20 keys, 200-char string values,
  nil/unsupported types dropped) and the client's `PrivateAttributes` are
  stripped. A per-process limiter dedups identical events within 30s and caps at
  25 sends. The new `sdk_version` field (from the embedded `VERSION` file, exposed
  as `shipeasy.SDKVersion`) and the client `env` are included on every event.
  No-op in test/offline mode. NEW client field: `env` is now stored on the client.

## 0.5.0

- **OpenFeature provider.** Added a `ShipeasyProvider` (constructed with
  `shipeasyopenfeature.NewProvider(client)`) implementing the CNCF OpenFeature
  `github.com/open-feature/go-sdk/openfeature.FeatureProvider` interface, so apps
  standardized on OpenFeature can plug Shipeasy in as the backing provider.
  `Metadata().Name` is `"shipeasy"`. Boolean flags resolve through the gate
  evaluator (`GetFlagDetail`), mapping the Shipeasy reason to the OpenFeature
  reason/error: `RULE_MATCH→TARGETING_MATCH`, `DEFAULT→DEFAULT`, `OFF→DISABLED`,
  `OVERRIDE→STATIC`, `FLAG_NOT_FOUND→ERROR/FlagNotFound`,
  `CLIENT_NOT_READY→ERROR/ProviderNotReady`. String/Float/Int/Object flags route
  to `GetConfig` (absent → default with reason `DEFAULT`; wrong type → default
  with `TYPE_MISMATCH`; present → value with `TARGETING_MATCH`). The
  `targetingKey` becomes `user_id`; all other context entries become targeting
  attributes. `Hooks()` returns `nil`. It lives in its own nested Go module
  (`github.com/shipeasy-ai/sdk-go/openfeature`) so the base SDK does **not** pull
  in `go-sdk` for consumers that don't use OpenFeature.
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
