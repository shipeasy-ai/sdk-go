# Admin API client (optional) — `admin` module

The base SDK *evaluates* flags, configs, and experiments (`Configure()` +
`NewClient(user)`). The **Admin API client** is a separate, optional surface for
*administering* a small, deliberate slice of those resources from server code.

It is **intentionally lean** — three groups of operations, not the whole admin
API:

| Group                    | What it covers                                                     |
| ------------------------ | ------------------------------------------------------------------ |
| Public ticket queue      | File a bug or feature request, list the queue, read and update one item, and hold its comment thread |
| Kill-switch sub-switches | Add, edit, and delete the named nested switches on a kill switch    |
| Flag whitelists          | Read a gate and manage the whitelist on its targeting stack         |

Everything else in the admin API — experiments, metrics, events, configs, i18n,
projects, connectors, keys — is deliberately **not** here. Reach for the Shipeasy
CLI or MCP for those; they speak the complete spec. Keeping the vendored contract
small is what keeps the generated client small.

It is a **separate Go module** (`github.com/shipeasy-ai/sdk-go/admin`), so the
base SDK never pulls it in. You opt in by importing it:

```go
import "github.com/shipeasy-ai/sdk-go/admin"
```

```bash
go get github.com/shipeasy-ai/sdk-go/admin
```

The client is **generated from the Shipeasy OpenAPI spec**, so it is a raw, 1:1
projection of the REST API: id-based, basis-points, snake_case. It does *not* add
the name→id resolution or percent→basis-point conveniences of the Shipeasy
CLI/MCP — reach for those tools when you want the ergonomic surface, and for this
module when you want a typed, programmatic mirror of the API.

## Authenticate and scope

Mint an **admin** SDK key (`sdk_admin_…`) and scope every call to a project.

```go
package main

import (
	"context"
	"os"

	"github.com/shipeasy-ai/sdk-go/admin"
)

func main() {
	client := admin.NewClient(
		os.Getenv("SHIPEASY_ADMIN_KEY"),                  // Authorization: Bearer <key>
		admin.WithProjectID(os.Getenv("SHIPEASY_PROJECT_ID")), // X-Project-Id on every call
		// admin.WithBaseURL("http://localhost:3000"),    // defaults to https://shipeasy.ai
	)

	flags, _, err := client.FlagsAPI.ListGates(context.Background()).Execute()
	_ = flags
	_ = err
}
```

## Resource groups

Each resource group is a field on the embedded client whose methods map 1:1 to
the OpenAPI operations:

```go
// file a bug on the public ticket queue, then comment on it
client.OpsAPI.CreateOpsItem(ctx).Execute()
client.CommentsAPI.CreateOpsComment(ctx, "42").Execute()

// manage a gate's whitelist (it lives on the targeting stack)
client.FlagsAPI.UpdateGate(ctx, "g_123").Execute()

// add or remove a kill switch's nested sub-switch
client.KillswitchAPI.SetKillswitchSwitch(ctx, "k_123").Execute()
client.KillswitchAPI.UnsetKillswitchSwitch(ctx, "k_123").Execute()
```

Available groups: `FlagsAPI`, `KillswitchAPI`, `OpsAPI`, `CommentsAPI`. The exact
method names, request models, and response shapes come straight from the spec —
explore them with your editor's autocomplete.

## Regenerating

The generated code lives in `admin/` (everything except `client.go`) and is committed.
`admin/openapi.json` is **not** the full Shipeasy spec — it is the pruned subset
described above, produced in the monorepo by `scripts/sdk-spec/prune.mjs` from
`scripts/sdk-spec/keep-set.json`. Do not hand-edit it, and do not replace it with
the full `openapi.json`: that is what bloats the generated client back to
megabytes.

From the monorepo, re-vendor and regenerate in one step (only the generated code
is rewritten, never the hand-written shim):

```bash
pnpm sdk:spec:regen sdk-go
```

A monorepo pre-commit hook blocks any commit that changes the admin spec while
this vendored copy is stale, so the two cannot silently drift.

The generator version is pinned in `openapitools.json`.
