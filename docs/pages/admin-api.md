# Admin API client (optional) — `admin` module

The base SDK *evaluates* flags, configs, and experiments (`Configure()` +
`NewClient(user)`). The **Admin API client** is a separate, optional surface for
*administering* those resources from server code — creating gates, starting
experiments, managing configs, kill switches, universes, metrics, events, and
more.

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
client.FlagsAPI.CreateGate(ctx).Execute()
client.ExperimentsAPI.CreateExperiment(ctx).Execute()
```

Available groups: `FlagsAPI`, `ConfigsAPI`, `KillswitchAPI`, `ExperimentsAPI`,
`UniversesAPI`, `AttributesAPI`, `MetricsAPI`, `EventsAPI`, `OpsAPI`, `AlertsAPI`,
`ProjectsAPI`, `ProfilesAPI`, `KeysAPI`, `DraftsAPI`, `ErrorsAPI`, `ConnectorsAPI`,
`APIKeysAPI`. The exact method names, request models, and response shapes come
straight from the spec — explore them with your editor's autocomplete.

## Regenerating

The generated code lives in `admin/` (everything except `client.go`) and is
committed. When the API contract changes, refresh the vendored spec and
regenerate — only the generated files are rewritten, never the `NewClient` shim:

```bash
cp <monorepo>/marketplace/openapi/openapi.json admin/openapi.json
bash scripts/gen_admin.sh
```

The generator version is pinned in `openapitools.json`.
