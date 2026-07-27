// Package admin is the OPTIONAL, generated Admin API client for the Shipeasy Go
// SDK. It is a SEPARATE Go module (github.com/shipeasy-ai/sdk-go/admin) so the
// base SDK never pulls it in — users opt in explicitly:
//
//	import "github.com/shipeasy-ai/sdk-go/admin"
//
// Everything except this file (client.go / client_test.go) is generated 1:1 from
// the Shipeasy OpenAPI spec by scripts/gen_admin.sh and must not be edited by
// hand. This file is the thin AdminClient entry point: it wires bearer auth and
// project scoping onto the generated APIClient. It does NOT add name->id or
// percent->basis-point ergonomics (that facade lives in the Shipeasy CLI/MCP);
// the surface here is the raw, id/basis-point/snake_case REST API.
package admin

// Client is a configured Admin API client. It embeds the generated *APIClient,
// so the resource groups are reached directly: client.FlagsAPI,
// client.KillswitchAPI, client.OpsAPI.
//
// Those three are the whole surface: the vendored spec is the dedicated
// server-SDK contract (marketplace/openapi/spec/openapi-sdk.yaml in the
// monorepo), seven operations across three capabilities — file a public ticket,
// toggle a kill switch, manage a flag's whitelist. The rest of the admin API
// (experiments, metrics, events, configs, i18n, projects, …) is deliberately
// absent — reach it through the Shipeasy CLI or MCP, which use the full spec.
type Client struct {
	*APIClient
}

// Option customizes the underlying Configuration.
type Option func(*Configuration)

// WithProjectID scopes every request to a project via the X-Project-Id header.
func WithProjectID(projectID string) Option {
	return func(c *Configuration) {
		if projectID != "" {
			c.AddDefaultHeader("X-Project-Id", projectID)
		}
	}
}

// WithBaseURL overrides the API base URL (defaults to https://shipeasy.ai; use
// http://localhost:3000 for local dev).
func WithBaseURL(url string) Option {
	return func(c *Configuration) {
		if url != "" {
			c.Servers = ServerConfigurations{{URL: url}}
		}
	}
}

// WithClientKey supplies a CLIENT SDK key (sdk_client_…) for the two public
// ticket operations — OpsAPI.CreatePublicBug and CreatePublicFeatureRequest.
// Those live on the Shipeasy edge worker and authenticate with X-SDK-Key rather
// than the admin bearer token, and the generated client already routes them to
// the edge host. The key must carry the `tickets:public_create` scope.
//
// Sent as a default header rather than threaded through a request context: the
// generated operations only read ContextAPIKeys when it is present, so a
// default header is the simpler equivalent and needs no ctx plumbing.
func WithClientKey(clientKey string) Option {
	return func(c *Configuration) {
		if clientKey != "" {
			c.AddDefaultHeader("X-SDK-Key", clientKey)
		}
	}
}

// WithConfiguration applies an arbitrary tweak to the generated Configuration
// (custom HTTP client, extra default headers, …) for advanced use.
func WithConfiguration(fn func(*Configuration)) Option { return Option(fn) }

// NewClient builds an Admin API client authenticated with an admin SDK key
// (sent as Authorization: Bearer <apiKey>). Pass WithProjectID to scope requests
// and WithBaseURL to target a non-production host.
//
//	client := admin.NewClient(os.Getenv("SHIPEASY_ADMIN_KEY"),
//	    admin.WithProjectID(os.Getenv("SHIPEASY_PROJECT_ID")))
//	wl, _, err := client.FlagsAPI.GetGateWhitelist(context.Background(), "new_checkout").Execute()
func NewClient(apiKey string, opts ...Option) *Client {
	cfg := NewConfiguration()
	cfg.AddDefaultHeader("Authorization", "Bearer "+apiKey)
	for _, opt := range opts {
		opt(cfg)
	}
	return &Client{APIClient: NewAPIClient(cfg)}
}
