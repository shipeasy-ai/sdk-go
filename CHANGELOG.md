# Changelog

## 0.20.0 — 2026-07-26

### The SSR bootstrap tag loads `/sdk/runtime.js`

`bootstrapScriptTag` now points at `/sdk/runtime.js` instead of the retired
`/sdk/bootstrap.js`, and emits both marker attributes (`data-se-bootstrap` for
the npm client SDK, `data-se-boot` for the runtime). The `data-*` payload is
unchanged, so this is a drop-in — but the emitted markup differs, so snapshot
tests asserting the old `src` need updating.

`bootstrap.js` did two things beyond relaying attributes: minting the
`__se_anon_id` cookie and publishing `window.__SE_BOOTSTRAP`. The runtime now
does both, and additionally installs a working `window.shipeasy` from the SSR
payload — so an SSR page gets the vanilla-JS API for free, with one request
fewer than before.

**`/sdk/bootstrap.js` no longer exists.** Pages served by an older version of
this SDK will 404 on that tag and lose the anon cookie, so upgrade before the
next deploy.

## 0.19.0 — 2026-07-26

### Feat!: the SSR tag helpers take every argument from `Configure`, plus a devtools tag

- **Every tag helper is now callable with no arguments.** They take variadic
  `TagOptions`, and each unset field falls back to what `Configure` was given:

  ```go
  head := shipeasy.BootstrapScriptTag(user) + shipeasy.I18nScriptTag()
  ```

- **New `Options` fields feeding those defaults:** `ClientKey` (the PUBLIC client
  key), `Profile`, `ProjectID`, `CDNBaseURL`.
- **New `shipeasy.DevtoolsScriptTag`** emits the hosted devtools overlay bundle
  (`se-devtools.js`) with `data-project-id` + `data-client-api-key`, `defer` by
  default (drop it with `TagOptions{NoDefer: true}`). The overlay opens with
  **Shift+Alt+S** or on any page loaded with `?se=1`.
- **`BootstrapTagOptions` is now `TagOptions`** — the old name is kept as a type
  alias, and it gained `ClientKey`, `ProjectID` and `NoDefer` fields.
- **BREAKING (source-level):** `I18nScriptTag(clientKey, profile string, opts
  BootstrapTagOptions)` became `I18nScriptTag(opts ...TagOptions)`. Migrate
  `I18nScriptTag(k, p, BootstrapTagOptions{})` →
  `I18nScriptTag(TagOptions{ClientKey: k, I18nProfile: p})`, or set `ClientKey`
  and `Profile` on `Options` and call `I18nScriptTag()`. `BootstrapScriptTag`'s
  options argument became variadic, so existing call sites keep compiling (and a
  `nil` user is now an anonymous request).
- A tag built with a missing key / project id still renders, and the SDK logs a
  warning naming the `Options` field to fill in — once per field, not once per
  render. Mirrors the Ruby SDK 3.7.0, Python 0.21.0 and PHP 0.20.0.

## 0.18.0 — 2026-07-19

### Feat: carry the server-identified user on the SSR bootstrap tag as `data-user`

`BootstrapScriptTag` now emits a `data-user` attribute — the HTML-escaped JSON of
the evaluated user's attributes, **minus `anonymous_id`** (that already rides
`data-anon-id`) — whenever the request is identified (any attribute beyond
`anonymous_id`). The browser SDK reads it off the `<script data-se-bootstrap>` tag
and adopts that identity on first paint, so the client buckets as the **same**
user the server did — killing the anonymous→identified flip after hydration for a
Go-backend + JS-frontend app. An anonymous request (only `anonymous_id`, or an
empty user) emits **no** `data-user`, so no PII rides the tag. Keys are sorted
deterministically. No API change — the identity is the same `user` already passed
to `BootstrapScriptTag`. See
[18-identity-bucketing.md](https://github.com/shipeasy-ai/experiment-platform/blob/main/18-identity-bucketing.md).

## 0.17.1 — 2026-07-19

### Fix: honor the gatekeeper `stack` in local gate evaluation

Local gate evaluation now evaluates a gate's ordered gatekeeper **`stack`** when
the flags blob ships one, instead of reading only the flat `rules` + `rolloutPct`
fields. The flat fields are a lossy approximation of a modern gate: a whitelist
condition at 100% followed by a 0% public rollout flattens to
`rules:[project_id in [...]], rolloutPct:0`, which the flat path wrongly read as
"matches the whitelist AND is in the 0% bucket" = never true. The stack is tried
top-to-bottom and the gate passes on the first entry whose rules match AND whose
bucket hits, matching `@shipeasy/core`'s `evalGatekeeper` and the edge worker.
Per-condition rollout %, `pass:"any"`, `bucketBy`, per-entry `salt`, and `ramp`
(time-interpolated rollout) are all honored. A gate **without** a stack keeps the
exact legacy flat behavior. No public API change.

## 0.17.0 — 2026-07-13

### `See()`: inline extras on `.To`, no ordering footgun

- **`.To(outcome, extras ...map[string]any)`** — the terminal now accepts the
  extras inline, e.g.
  `shipeasy.See(err).CausesThe("checkout").To("use cached prices", map[string]any{"order_id": id})`.
  Each map is merged like a final `.Extras(...)` call (later map wins), folding
  under any earlier `.Extras`. So there is no longer an order to remember. Existing
  `.To(outcome)` call sites keep compiling unchanged (the argument is variadic).

## 0.16.0 — 2026-07-08

### Exposure fires on read, with a `Peek` opt-out

`Universe(name).Assign()` is now **side-effect free**. The single experiment
exposure fires **on read** — the first time an enrolled `Assignment` is read via
`Get(field, fallback)` — instead of eagerly at `Assign()` time. Reading
`Enrolled` / `Name` / `Group` never logs.

- **New `Assignment.Peek(field, fallback any) any`** — the read-only counterpart
  to `Get`: identical lookup (variant override ?? universe default ?? fallback),
  but logs **no** exposure. Reach for it when you need a param without counting
  the unit as exposed (logging, debugging, a pre-render peek).
- Exposure is still deduped per process (unit + experiment + group), and is now
  additionally **durably deduped server-side** per `(unit, experiment, group)`.

**BEHAVIOUR CHANGE.** Code that called `Assign()` purely for its exposure
side-effect (never reading a param) will no longer log one — read a param via
`Get` to record the exposure.

### Durable forced-but-gated ID & cohort/gate overrides

The resolver now honours durable **ID overrides** and **cohort/gate overrides**
that are *forced but still gated*: a matched override pins the group only when the
unit passes targeting and isn't held out; ID overrides beat cohort overrides.
This is consumed via the experiments blob — **no new user-facing SDK API**.
Running experiments are byte-identical; the new ordering rides `hash_version: 3`.

## 0.15.0 — 2026-07-08

### Environment-derived network & telemetry (egress) defaults

The SDK is now **quiet by default outside production**: an app that embeds it
makes **no outbound request** from a dev machine or CI unless it opts in. Two new
`Options` fields (both `*bool`, so an explicit `false` is distinguishable from an
unset `nil`) control egress:

- **`IsNetworkEnabled`** — master switch. When off the SDK is fully offline: no
  flag/experiment/config fetch, no `Track`, no exposure logging, no internal error
  reports, and no usage telemetry. Reads return your in-code defaults and any
  `Override*` values. It flows into the existing offline (`localMode`) machinery.
- **`IsTrackingEnabled`** — usage telemetry / "any outside logging" only. Forced
  off whenever the network is off.

Both **default ON in production and OFF in every other environment**. "Production"
is decided by `isProductionEnv`, checking a native env var in order —
`SHIPEASY_ENV`, then `APP_ENV`, then `GO_ENV`, then `ENV` (`production`/`prod`,
case-insensitive) — and, when none is set, falling back to the SDK's own `Env`
option (which already defaults to `"prod"`). An explicit option always overrides
the default; test/offline configs stay offline regardless.

**BEHAVIOUR CHANGE.** Before 0.15.0 the SDK always made network calls and always
sent usage telemetry (unless `DisableTelemetry: true`). Now, in a non-production
environment, it is offline by default. To **restore the old always-on behaviour**,
either declare the runtime production for egress by setting `SHIPEASY_ENV=production`
(or `APP_ENV`/`GO_ENV`/`ENV`), or pass the switches explicitly:

```go
on := true
shipeasy.Configure(shipeasy.Options{
    APIKey:            os.Getenv("SHIPEASY_SERVER_KEY"),
    IsNetworkEnabled:  &on,
    IsTrackingEnabled: &on,
})
```

`DisableTelemetry` is now **deprecated** in favour of `IsTrackingEnabled`:
`true` still forces telemetry off, but `false` (the zero value) no longer forces
it on — it defers to `IsTrackingEnabled` / the environment default.

## 0.14.0 — 2026-07-08

### BREAKING — experiments are now read by universe, not by name

The whole experiment read surface is replaced. A **universe is a
mutual-exclusion pool**: a unit is enrolled in **at most one** experiment in it,
so you ask a universe for an assignment instead of naming an experiment.
`Engine.GetExperiment`, `Engine.LogExposure`/`LogExposureUser`,
`Client.GetExperiment`, and `Client.LogExposure` are **removed**.

```go
// Before (removed):
r := c.GetExperiment("checkout_button", map[string]any{"color": "red"})
if r.InExperiment && r.Params.(map[string]any)["color"] == "green" { … }

// After (bound Client):
a := c.Universe("checkout").Assign()
if a.Get("color", "red") == "green" { … }

// After (heavyweight Engine): engine.Universe("checkout").Assign(user)
```

- **`Universe(name).Assign()`** (bound `Client`) / **`Universe(name).Assign(user)`**
  (`Engine`) returns an `Assignment`:
  - `.Name` — the experiment the unit landed in, or `""` when not enrolled.
  - `.Group` — the assigned variant, or `""` when not enrolled.
  - `.Enrolled` — `bool` (true iff `Group != ""`).
  - `.Get(field, fallback)` — resolves **variant override ?? universe default ??
    fallback**. Works even when not enrolled (you get the universe default),
    because the universe now owns the param schema + defaults.
- **Auto-exposure.** `Assign()` logs a single exposure when the unit is enrolled,
  deduped per process (unit + experiment + group). The manual `LogExposure`
  primitive is gone — reading *is* the exposure. No-op in test/offline mode.
- **Mutual exclusion (pooled assignment), per-experiment holdout gates, reserved
  headroom, and universe-default ⊕ variant param merge** are now honoured by
  local eval, matching the edge. New experiment blob fields (`holdoutGate`,
  `poolOffsetBp`, `poolSizeBp`, `reservedHeadroomBp`, `hashVersion`) and universe
  `param_schema` are parsed. The SSR `Evaluate()` bootstrap now carries a
  top-level `universes` defaults map and a `universe` field per experiment, with
  the enrolled `params` pre-merged.
- The internal `OverrideExperiment` test seam is retained: it *refines* an
  experiment that lives in a universe (forces its variant) and surfaces through
  `Universe().Assign()` when the experiment is a running candidate in the loaded
  blob.

## 0.13.0 — 2026-07-08

- **SDK self-monitoring for internal errors.** When one of the SDK's last-resort
  guards (`recoverRead`, the deferred `recover` wrapping every runtime read)
  swallows an internal failure — a bug on Shipeasy's side, not the caller's — it
  now also reports that error to Shipeasy's own project so we can track and fix
  SDK bugs across every app the SDK runs in. This is a dedicated, baked-in
  destination (a public client-key ingest credential), entirely separate from
  your `See()` reporting: internal errors never land in your project or Errors
  tab. The report carries only the error itself plus a stable, deduped
  consequence (subject = the guarded operation, e.g. `GetFlag`) and is
  fire-and-forget — it can never slow down or break a read. On by default and
  always off in test/offline mode; opt out with
  `Options.DisableInternalErrorReporting: true`.

## 0.12.1 — 2026-07-07

- **Fixed: default API host now resolves.** The default `Options.BaseURL`
  pointed at the unregistered domain `https://edge.shipeasy.dev`, so every
  `Configure` one-shot fetch and every `GetFlag`/`GetConfig`/`GetExperiment`/
  `Track`/`See()` call failed with a DNS error unless `BaseURL` was set
  explicitly. Corrected to the real edge origin `https://api.shipeasy.ai` — the
  host the docs, CLI, and curl snippets already use. Explicit `BaseURL`
  overrides are unaffected.

## 0.12.0 — 2026-07-07

- **Fail-safe runtime reads.** Every runtime read now catches an unexpected
  panic and returns its documented safe default instead of unwinding into the
  caller. This covers `Engine.GetFlag`/`GetFlagOr`/`GetFlagDetail`/`GetConfig`/
  `GetConfigOr`/`GetKillswitch`/`GetExperiment` and the bound `Client`
  equivalents. The read paths were already panic-safe by construction; this is a
  defensive last resort so a future regression can never take down a request.
  Setup/lifecycle panics are unchanged — `NewClient(user)` before `Configure()`,
  the `requireGlobal(...)` package helpers, and `openfeature.NewGlobalProvider()`
  still fail loudly, since those are boot-time misconfiguration.
- **New `Options.LogLevel`.** Sets the SDK's own log verbosity: `"silent"`,
  `"error"`, `"warn"`, `"info"`, or `"debug"` (ordered
  `silent < error < warn < info < debug`; a message at level L is logged iff
  `LogLevel >= L`). Empty or unknown values resolve to the default, `"warn"`. Use
  it to quiet the SDK's fire-and-forget network/decode chatter
  (`Track`/`LogExposure`/`see()`/poll failures) in production. Every internal
  `[shipeasy] …` log now routes through this leveled gate.

## 0.11.1

- **Admin API client regenerated from the canonical OpenAPI spec (2.0.0).** The
  0.11.0 client was generated from a stale 1.0.0 subset; this regenerates it from
  the full spec, adding the connectors, errors, keys/api-keys, drafts, profiles,
  and search endpoints (resource groups now: flags, configs, killswitch,
  experiments, universes, attributes, metrics, events, ops, alerts, projects,
  profiles, keys, drafts, errors, connectors, apiKeys). Tag renames mean the
  generated service fields changed (e.g. `GatesAPI`→`FlagsAPI`,
  `KillswitchesAPI`→`KillswitchAPI`, `AlertRulesAPI`→`AlertsAPI`).

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
