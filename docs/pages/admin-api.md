# Admin API client (optional) — `admin`

The base SDK *evaluates* flags, configs, and experiments
([`Configure()`](configuration.md) + `NewClient(user)`). The **Admin API client**
is a separate, optional surface for *administering* a small, deliberate slice of
those resources from server code.

It is **intentionally lean** — three capabilities, seven operations, not the
whole admin API:

| Capability           | Operations                                                                             |
| -------------------- | -------------------------------------------------------------------------------------- |
| File a public ticket (client key) | `CreatePublicBug`, `CreatePublicFeatureRequest`                                          |
| Toggle a kill switch | `ToggleKillswitch`                                                                       |
| Manage a whitelist   | `GetGateWhitelist`, `SetGateWhitelist`, `AddToGateWhitelist`, `RemoveFromGateWhitelist`  |

Everything else in the admin API — listing, generic CRUD, experiments, metrics,
events, configs, i18n, projects, connectors, keys — is deliberately **not** here.
Reach for the Shipeasy CLI or MCP for those; they speak the complete spec.
Keeping the vendored contract small is what keeps the generated client small.

It is **off by default**: `admin` is a **separate Go module**, so the base SDK
never pulls it in. Opt in explicitly:

```bash
go get github.com/shipeasy-ai/sdk-go/admin
```

```go
import "github.com/shipeasy-ai/sdk-go/admin"
```

The client is **generated from the Shipeasy OpenAPI spec**, so it is a raw, 1:1
projection of the REST API: id-based paths and typed request models. It does
*not* add the percent→basis-point conveniences you get from the Shipeasy
CLI/MCP — reach for those tools when you want the ergonomic surface, and for this
client when you want a typed, programmatic mirror of the API.

## Authenticate and scope

Mint an **admin** SDK key (`sdk_admin_…`) and scope every call to a project.

```go
client := admin.NewClient(
    os.Getenv("SHIPEASY_ADMIN_KEY"),                       // Authorization: Bearer <key>
    admin.WithProjectID(os.Getenv("SHIPEASY_PROJECT_ID")), // sent as X-Project-Id on every call
    // Only needed for the two public ticket operations — see "File a public
    // ticket" below. They send it as X-SDK-Key, on the edge host.
    admin.WithClientKey(os.Getenv("SHIPEASY_CLIENT_KEY")),
    // admin.WithBaseURL("http://localhost:3000") for local dev
)
```

Every operation is a fluent request builder: chain the body/params, then
`Execute()`.

## File a public ticket

Two dedicated endpoints, so there is no discriminator to set and no fields from
the other kind in the request. Both accept just a title.

These two are the **public** intake, and they differ from the other five
operations in three ways worth knowing before you call them:

- They authenticate with a **client** key (`sdk_client_…`) carrying the
  `tickets:public_create` scope — not the admin key. Client keys are meant to be
  embedded in shipped code, which is the point: a CLI, an installer, or a
  browser bundle can file a ticket without holding an admin credential. Pass
  yours with `admin.WithClientKey(...)`.
- They are served by the Shipeasy **edge worker** (`api.shipeasy.ai`), not the
  admin API host. The generated client already routes them there.
- Every item is filed as `pending_approval`, parked out of the work queue until
  a human promotes it in the dashboard, and repeat submissions of the same title
  dedupe against the open ticket already tracking them (HTTP 200 with
  `deduped: true` instead of a second ticket).

The project is the key's own project — there is no project id to pass and no way
to file into someone else's queue. The project must have public ticket creation
enabled.

```go
ctx := context.Background()

body := admin.NewCreatePublicBugRequest("Checkout 500s on Safari")
body.SetStepsToReproduce("Open the cart on iOS Safari and tap the price row.")
body.SetActualResult("The primary CTA overlaps the price.")
body.SetPriority(admin.OPSITEMPRIORITY_HIGH)

filed, _, err := client.OpsAPI.CreatePublicBug(ctx).CreatePublicBugRequest(*body).Execute()
if err != nil {
    log.Fatal(err)
}
fmt.Println(filed.GetNumber())

feature := admin.NewCreatePublicFeatureRequestRequest("Dark mode")
feature.SetUseCase("Reduce eye strain at night")
_, _, _ = client.OpsAPI.CreatePublicFeatureRequest(ctx).
    CreatePublicFeatureRequestRequest(*feature).Execute()
```

## Toggle a kill switch

`ToggleKillswitch` reads the current value and publishes its opposite in one
call. Every body field is optional, so it widens from "flip it" to "set exactly
this, on this environment":

```go
ks := "payments.checkout" // id (ksw_…) or name

// Flip the kill switch itself on prod — no body at all.
_, _, _ = client.KillswitchAPI.ToggleKillswitch(ctx, ks).Execute()

// Flip one nested sub-switch on prod (created off→on if it doesn't exist yet).
flip := admin.NewToggleKillswitchRequest()
flip.SetSwitchKey("eu_region")
_, _, _ = client.KillswitchAPI.ToggleKillswitch(ctx, ks).ToggleKillswitchRequest(*flip).Execute()

// Set it idempotently, on a chosen environment — a retry can't undo the first call.
set := admin.NewToggleKillswitchRequest()
set.SetSwitchKey("eu_region")
set.SetValue(true)
set.SetEnv(admin.ENV_STAGING)

res, _, err := client.KillswitchAPI.ToggleKillswitch(ctx, ks).
    ToggleKillswitchRequest(*set).Execute()
if err != nil {
    log.Fatal(err)
}
fmt.Printf("%v -> %v\n", res.GetPrevious(), res.GetValue()) // false -> true
```

Omitting `Value` means **flip**; setting it explicitly means **set**. Omitting
`Env` targets `prod`.

## Manage a flag's whitelist

A gate's whitelist is the always-first allowlist that admits named identities
before any targeting rule or percentage rollout runs — the same list the
dashboard's Whitelist block edits.

```go
gate := "new_checkout" // id (gate_…) or name

wl, _, err := client.FlagsAPI.GetGateWhitelist(ctx, gate).Execute()
if err != nil {
    log.Fatal(err)
}
fmt.Println(wl.GetAttr(), wl.GetEntries()) // email [alice@acme.dev]

// Let one more person in (idempotent — already-listed entries are skipped).
add := admin.NewAddToGateWhitelistRequest([]string{"bob@acme.dev"})
_, _, _ = client.FlagsAPI.AddToGateWhitelist(ctx, gate).AddToGateWhitelistRequest(*add).Execute()

// Revoke one.
rm := admin.NewRemoveFromGateWhitelistRequest([]string{"bob@acme.dev"})
_, _, _ = client.FlagsAPI.RemoveFromGateWhitelist(ctx, gate).
    RemoveFromGateWhitelistRequest(*rm).Execute()

// Pin an exact list, re-key onto user ids, or clear it entirely.
reKey := admin.NewSetGateWhitelistRequest([]string{"usr_123"})
reKey.SetAttr(admin.GATEWHITELISTATTR_USER_ID)
_, _, _ = client.FlagsAPI.SetGateWhitelist(ctx, gate).SetGateWhitelistRequest(*reKey).Execute()

clear := admin.NewSetGateWhitelistRequest([]string{})
_, _, _ = client.FlagsAPI.SetGateWhitelist(ctx, gate).SetGateWhitelistRequest(*clear).Execute()
```

`SetGateWhitelist` is the only call that can switch the attribute or drop the
block — an empty `Entries` removes the whitelist from the gate. Adding to a
whitelist keyed on the other attribute is rejected with a 409 rather than
silently re-keying everyone already listed.

## Resource groups

`Client` embeds the generated `*APIClient`, so the groups are reached directly:
`client.FlagsAPI`, `client.KillswitchAPI`, `client.OpsAPI`. Those three are the
whole surface.

## Escape hatch

`client.GetConfig()` exposes the generated `Configuration` for advanced use
(custom `http.Client`, extra default headers), and `admin.WithConfiguration`
applies an arbitrary tweak at construction.

## Regenerating

The generated code lives under `admin/` and is committed. `admin/openapi.json` is
**not** the full Shipeasy spec — it is the dedicated server-SDK contract,
hand-authored in the monorepo as `marketplace/openapi/spec/openapi-sdk.yaml` and
bundled to `openapi-sdk.json`. Do not hand-edit it, and do not replace it with
the full `openapi.json`: that is what bloats the generated client back to
megabytes.

From the monorepo, re-vendor and regenerate in one step (only the generated
files are rewritten, never `client.go`):

```bash
pnpm sdk:spec:regen sdk-go
```

A monorepo pre-commit hook blocks any commit that changes the admin spec while
this vendored copy is stale, so the two cannot silently drift.

The generator version is pinned in `openapitools.json`, and `gen_admin.sh` passes
`enumClassPrefix=true` — without it two enums that share a value collide and the
module does not compile.
