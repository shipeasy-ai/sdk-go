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
// so the resource groups are reached directly: client.FlagsAPI, client.ConfigsAPI,
// client.KillswitchAPI, client.ExperimentsAPI, client.UniversesAPI,
// client.MetricsAPI, client.EventsAPI, client.AlertsAPI, client.AttributesAPI,
// client.OpsAPI, client.ProjectsAPI, client.ConnectorsAPI, client.ErrorsAPI,
// client.KeysAPI, client.DraftsAPI, client.ProfilesAPI, client.APIKeysAPI.
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

// WithConfiguration applies an arbitrary tweak to the generated Configuration
// (custom HTTP client, extra default headers, …) for advanced use.
func WithConfiguration(fn func(*Configuration)) Option { return Option(fn) }

// NewClient builds an Admin API client authenticated with an admin SDK key
// (sent as Authorization: Bearer <apiKey>). Pass WithProjectID to scope requests
// and WithBaseURL to target a non-production host.
//
//	client := admin.NewClient(os.Getenv("SHIPEASY_ADMIN_KEY"),
//	    admin.WithProjectID(os.Getenv("SHIPEASY_PROJECT_ID")))
//	flags, _, err := client.FlagsAPI.ListGates(context.Background()).Execute()
func NewClient(apiKey string, opts ...Option) *Client {
	cfg := NewConfiguration()
	cfg.AddDefaultHeader("Authorization", "Bearer "+apiKey)
	for _, opt := range opts {
		opt(cfg)
	}
	return &Client{APIClient: NewAPIClient(cfg)}
}
