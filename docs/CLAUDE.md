# CLAUDE.md — `docs/`

This folder is the SDK's **published documentation**. It follows the cross-SDK
Shipeasy doc standard (experiment-platform `22-*` + `23-sdk-dx-standard.md`) so the
same structure exists in every Shipeasy SDK repo and can be consumed by tooling.

## How it's published

GitHub Pages serves this folder verbatim: **Settings → Pages → Deploy from a
branch → `main` / `/docs`**. Files are then fetchable raw at:

```
https://shipeasy-ai.github.io/sdk-go/manifest.json
https://shipeasy-ai.github.io/sdk-go/pages/<page>.md
https://shipeasy-ai.github.io/sdk-go/snippets/<group>/<leaf>.md
https://shipeasy-ai.github.io/sdk-go/skill/SKILL.md
```

`/.nojekyll` MUST stay — it makes Pages serve raw `.md`/JSON bytes. Three
consumers read this folder: end users (rendered on github.com), the Shipeasy
CLI/MCP `docs` op (raw fetch from Pages), and the central docs portal.

## Structure

```
docs/
├── .nojekyll                 # keep — serve raw bytes
├── manifest.json             # the index of everything (schemaVersion 2)
├── skill/SKILL.md            # installable agent skill (YAML frontmatter + guide)
├── pages/                    # feature-reference pages (FIXED key vocabulary)
└── snippets/<group>/<leaf>.md  # tiny copy-paste blocks, grouped
```

### `manifest.json`

The runtime index. Keep it valid JSON and in sync with the files on disk:

- `sdk` — the registry name (`"go"`).
- `placeholders` — every `{{TOKEN}}` used in snippets: `FLAG_KEY`, `CONFIG_KEY`,
  `KILLSWITCH_KEY`, `EXPERIMENT_KEY`, `EVENT_NAME`, `SUCCESS_EVENT`, `PROFILE`.
- `skill` — path to the installable skill.
- `pages` — map of page key → path. The **keys are a fixed vocabulary shared
  across all SDKs**: `overview, installation, configuration, flags, configs,
  killswitches, experiments, i18n, error-reporting, testing, openfeature,
  advanced`. Don't rename keys.
- `snippets` — nested `{ group: { leaf: path } }`. Groups: `release`, `metrics`
  (track), `i18n`, `ops` (see). Adding a snippet = add the file **and** its
  manifest entry.

### `pages/`

One feature-reference page per fixed key. Each starts with an H1 and documents
that feature for **this SDK's real API**. Written around `Configure()` +
`NewClient(user)` — never the `Engine` (see the repo-root `CLAUDE.md`).

### `snippets/<group>/<leaf>.md`

Minimal copy-paste examples. Conventions (enforced):

- **No `Configure()` call inside snippet code** — a one-line "Assumes
  `Configure()` ran at startup — see Installation." note instead.
- **Construct the bound client on its own line**, with a "construct once per
  callsite" comment — never chain `NewClient(user).GetFlag(...)`.
- **Document every argument** inline.
- Use the manifest's `{{PLACEHOLDER}}` tokens, not hard-coded names.
- A file may hold a few labelled mini-snippets (`### Heading` + a block each).

### `skill/SKILL.md`

An installable Claude-Code-style skill: YAML frontmatter (`name`, `description`)
followed by a tight usage guide. It is bundled into `cmd/shipeasy-skill` via
`go:embed` (kept in sync by `internal/genreadme`) and installed with
`shipeasy-skill install`, so keep the frontmatter valid.

## The README is generated from these docs

`../README.md` is **generated** by `../internal/genreadme` (which also re-syncs
the embedded `../cmd/shipeasy-skill/SKILL.md`). After any doc edit, run
`go run ./internal/genreadme` from the module root and commit the result; CI
(`tests.yml`) fails if it drifts. Never hand-edit the README.

## Working on the docs

- **Keep docs in lockstep with the code.** Any public API/behaviour change updates
  the matching page(s), snippet(s), and the skill in the *same* change, then
  regenerate the README.
- Confirm every `pages`/`snippets`/`skill` path the manifest lists exists.
- Prefer plain Markdown. The central portal compiles these as MDX, so avoid bare
  `<` / unescaped `{` in prose (inside code fences is fine).
- Don't restructure the fixed `pages` keys or drop `.nojekyll`.
