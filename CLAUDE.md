# CLAUDE.md — shipeasy-go

Guidance for AI agents (and humans) working in this repository.

## What this is

`github.com/shipeasy-ai/sdk-go` — the **server** SDK for
[Shipeasy](https://shipeasy.ai): feature flags, dynamic configs, kill switches,
A/B experiments, metric tracking, `See()` error reporting, and SSR/i18n helpers.
Server-key only; never embed in a browser. Evaluation is local against a cached
blob. The OpenFeature provider lives in the nested module `./openfeature`.

## The documented public surface (this is a contract)

Users are taught exactly **two** things, and the docs must never drift from them:

1. **`Configure()`** — and its siblings `ConfigureForTesting()` /
   `ConfigureForOffline()` — for setup.
2. **`NewClient(user)`** — the cheap, user-bound handle for *all* reads
   (`GetFlag` / `GetFlagOr` / `GetFlagDetail` / `GetConfig` / `GetConfigOr` /
   `GetKillswitch` / `GetExperiment` / `LogExposure` / `Track`).

Plus the package-level helpers that let users avoid the heavyweight object:
`OverrideFlag` / `OverrideConfig` / `OverrideExperiment` / `ClearOverrides`,
`OnChange`, `BootstrapScriptTag` / `I18nScriptTag`, the global-form
`openfeature.NewGlobalProvider()`, and the `See()` family.

**The `Engine` type is an internal detail. Do NOT document it.** It stays public
(`NewEngine`) for advanced/back-compat use, but no page, snippet, skill, or the
README should tell a user to construct or call an `Engine`. New user-facing
capability should get a `Configure`-style or package-level affordance, then be
documented through that.

## HARD RULE: change the SDK → update the docs in the SAME change

`docs/` is the published, user-facing source of truth (rendered at
<https://shipeasy-ai.github.io/sdk-go/> and ingested by the Shipeasy CLI/MCP
`docs` tooling and the central docs portal). If you change the SDK's **public API
or behaviour**, you MUST update the docs in the same commit:

- New/changed/removed public function, method, field, default, or return shape →
  update the relevant `docs/pages/*.md`, the matching `docs/snippets/**`, and
  `docs/skill/SKILL.md`.
- New page / snippet / placeholder → also update `docs/manifest.json`.
- See [`docs/CLAUDE.md`](docs/CLAUDE.md) for the docs structure and conventions.

**`README.md` is generated — do not hand-edit it.** It is assembled from the docs
by `internal/genreadme` (which also syncs the embedded `cmd/shipeasy-skill/SKILL.md`).
After editing `docs/`, run:

```bash
go run ./internal/genreadme
```

CI (`.github/workflows/tests.yml`) re-runs it and fails if `README.md` or the
embedded skill is out of date, so commit the regenerated files.

## Versioning & release

- Bump the `VERSION` file and add a `CHANGELOG.md` entry.
- Publishing is **push-to-`main`**: the publish workflow self-tags `v$VERSION` and
  the Go module proxy serves it. A version-bumped push to `main` IS the release.

## Checks before you commit

- `go vet ./... && go test ./...` (base module) and the same in `openfeature/`
  (separate module). The suite is hermetic — no network. CI runs Go 1.21–1.23.
- New public behaviour ships with a test.
- Docs updated per the hard rule; `docs/manifest.json` stays valid JSON and every
  path it lists exists.
- `go run ./internal/genreadme` and commit the result (CI checks it's in sync).
