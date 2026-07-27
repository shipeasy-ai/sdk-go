package admin

import "testing"

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
	// The four groups of the pruned admin surface. Deliberately exhaustive: if a
	// keep-set change adds or drops a group, this list must move with it.
	if c.FlagsAPI == nil || c.KillswitchAPI == nil ||
		c.OpsAPI == nil || c.CommentsAPI == nil {
		t.Fatal("one or more resource-group services is nil")
	}
}
