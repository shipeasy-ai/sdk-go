package admin

import (
	"context"
	"testing"
)

// These tests only construct the client (no network) and assert the auth/scoping
// wiring + that the generated resource groups are reachable. Because admin/ is a
// SEPARATE Go module, the base SDK's `go test ./...` never runs these — the
// module boundary is the opt-in guard.

func TestNewClientWiresAuthAndScope(t *testing.T) {
	c := NewClient("sdk_admin_test", WithProjectID("proj_123"))
	cfg := c.GetConfig()
	if got := cfg.DefaultHeader["Authorization"]; got != "Bearer sdk_admin_test" {
		t.Fatalf("Authorization header = %q, want Bearer sdk_admin_test", got)
	}
	if got := cfg.DefaultHeader["X-Project-Id"]; got != "proj_123" {
		t.Fatalf("X-Project-Id header = %q, want proj_123", got)
	}
}

func TestDefaultBaseURLIsProduction(t *testing.T) {
	c := NewClient("sdk_admin_test")
	if got := c.GetConfig().Servers[0].URL; got != "https://shipeasy.ai" {
		t.Fatalf("default server URL = %q, want https://shipeasy.ai", got)
	}
}

func TestWithBaseURLOverrides(t *testing.T) {
	c := NewClient("k", WithBaseURL("http://localhost:3000"))
	if got := c.GetConfig().Servers[0].URL; got != "http://localhost:3000" {
		t.Fatalf("server URL = %q, want override", got)
	}
}

func TestResourceGroupsReachable(t *testing.T) {
	c := NewClient("k")
	// The three groups of the lean admin surface. Deliberately exhaustive: if a
	// change to the SDK spec adds or drops a group, this list must move with it.
	if c.FlagsAPI == nil || c.KillswitchAPI == nil || c.OpsAPI == nil {
		t.Fatal("one or more resource-group services is nil")
	}
}

func TestWithClientKeySetsTheIntakeHeader(t *testing.T) {
	// The two public ticket ops authenticate with a CLIENT key (X-SDK-Key) on
	// the edge worker, not the admin bearer.
	c := NewClient("k", WithClientKey("sdk_client_test"))
	if got := c.GetConfig().DefaultHeader["X-SDK-Key"]; got != "sdk_client_test" {
		t.Fatalf("X-SDK-Key = %q, want the client key", got)
	}
	if got := NewClient("k").GetConfig().DefaultHeader["X-SDK-Key"]; got != "" {
		t.Fatalf("X-SDK-Key = %q, want unset without WithClientKey", got)
	}
}

// The seven operations the SDK contract carries. These are compile-time
// references, so dropping or renaming one in the spec breaks the build here
// rather than silently shrinking the published client.
func TestContractOperationsExist(t *testing.T) {
	c := NewClient("k")
	ctx := context.Background()
	_ = c.OpsAPI.CreatePublicBug(ctx)
	_ = c.OpsAPI.CreatePublicFeatureRequest(ctx)
	_ = c.KillswitchAPI.ToggleKillswitch(ctx, "payments.checkout")
	_ = c.FlagsAPI.GetGateWhitelist(ctx, "new_checkout")
	_ = c.FlagsAPI.SetGateWhitelist(ctx, "new_checkout")
	_ = c.FlagsAPI.AddToGateWhitelist(ctx, "new_checkout")
	_ = c.FlagsAPI.RemoveFromGateWhitelist(ctx, "new_checkout")
}
