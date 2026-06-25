package shipeasyopenfeature

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/open-feature/go-sdk/openfeature"
	shipeasy "github.com/shipeasy-ai/sdk-go"
)

// newOFClient wires a no-network shipeasy test client through the real go-sdk
// (SetProviderAndWait + NewClient), seeding flags/configs via the SDK's own
// override facilities so no network is touched.
func newOFClient(t *testing.T, seed func(c *shipeasy.Engine)) *openfeature.Client {
	t.Helper()
	sc := shipeasy.NewTestClient()
	if seed != nil {
		seed(sc)
	}
	return ofClientFor(t, sc)
}

// ofClientFor registers sc as the global provider and returns an OpenFeature
// client. SetProviderAndWait replaces any previously-set provider, so tests are
// isolated despite the shared OpenFeature singleton.
func ofClientFor(t *testing.T, sc *shipeasy.Engine) *openfeature.Client {
	t.Helper()
	if err := openfeature.SetProviderAndWait(NewProvider(sc)); err != nil {
		t.Fatalf("SetProviderAndWait: %v", err)
	}
	return openfeature.NewClient("test")
}

// snapshotClient builds a no-network shipeasy client seeded from a raw /sdk/flags
// JSON body, so tests can exercise real gate evaluation (reasons, targeting)
// rather than only Override* short-circuits.
func snapshotClient(t *testing.T, flagsJSON string) *shipeasy.Engine {
	t.Helper()
	var flags any
	if err := json.Unmarshal([]byte(flagsJSON), &flags); err != nil {
		t.Fatalf("seed flags json: %v", err)
	}
	return shipeasy.NewOfflineClientFromSnapshot(flags, nil)
}

func TestMetadata(t *testing.T) {
	p := NewProvider(shipeasy.NewTestClient())
	if got := p.Metadata().Name; got != "shipeasy" {
		t.Fatalf("Metadata().Name = %q, want shipeasy", got)
	}
}

func TestHooksNil(t *testing.T) {
	p := NewProvider(shipeasy.NewTestClient())
	if p.Hooks() != nil {
		t.Fatalf("Hooks() = %v, want nil", p.Hooks())
	}
}

func TestBooleanTargetingMatch(t *testing.T) {
	ctx := context.Background()
	of := newOFClient(t, func(c *shipeasy.Engine) {
		c.OverrideFlag("new_checkout", true)
	})
	d, err := of.BooleanValueDetails(ctx, "new_checkout", false, openfeature.NewEvaluationContext("u1", nil))
	if err != nil {
		t.Fatalf("BooleanValueDetails: %v", err)
	}
	if d.Value != true {
		t.Fatalf("value = %v, want true", d.Value)
	}
	// An override resolves with reason STATIC.
	if d.Reason != openfeature.StaticReason {
		t.Fatalf("reason = %q, want STATIC", d.Reason)
	}
}

func TestBooleanRuleMatchReason(t *testing.T) {
	ctx := context.Background()
	// Seed a real gate (fully rolled out, enabled) so the reason is RULE_MATCH
	// rather than an override's STATIC.
	sc := snapshotClient(t, `{"gates":{"beta":{"enabled":true,"killswitch":false,"salt":"s","rolloutPct":10000,"rules":[]}},"configs":{}}`)
	of := ofClientFor(t, sc)
	d, err := of.BooleanValueDetails(ctx, "beta", false, openfeature.NewEvaluationContext("u1", nil))
	if err != nil {
		t.Fatalf("BooleanValueDetails: %v", err)
	}
	if d.Value != true {
		t.Fatalf("value = %v, want true", d.Value)
	}
	if d.Reason != openfeature.TargetingMatchReason {
		t.Fatalf("reason = %q, want TARGETING_MATCH", d.Reason)
	}
}

func TestBooleanFlagNotFound(t *testing.T) {
	ctx := context.Background()
	// A loaded (non-nil) but empty gates blob → a missing gate is FLAG_NOT_FOUND
	// (a nil blob would instead be CLIENT_NOT_READY).
	sc := snapshotClient(t, `{"gates":{},"configs":{}}`)
	of := ofClientFor(t, sc)
	d, err := of.BooleanValueDetails(ctx, "missing", true, openfeature.NewEvaluationContext("u1", nil))
	if err == nil {
		t.Fatalf("expected error for missing flag")
	}
	if d.Value != true {
		t.Fatalf("value = %v, want default true", d.Value)
	}
	if d.ErrorCode != openfeature.FlagNotFoundCode {
		t.Fatalf("errorCode = %q, want FLAG_NOT_FOUND", d.ErrorCode)
	}
	if d.Reason != openfeature.ErrorReason {
		t.Fatalf("reason = %q, want ERROR", d.Reason)
	}
}

func TestBooleanProviderNotReady(t *testing.T) {
	ctx := context.Background()
	// No flags blob loaded at all → CLIENT_NOT_READY → PROVIDER_NOT_READY.
	of := newOFClient(t, nil)
	d, err := of.BooleanValueDetails(ctx, "anything", true, openfeature.NewEvaluationContext("u1", nil))
	if err == nil {
		t.Fatalf("expected error when provider not ready")
	}
	if d.Value != true {
		t.Fatalf("value = %v, want default true", d.Value)
	}
	if d.ErrorCode != openfeature.ProviderNotReadyCode {
		t.Fatalf("errorCode = %q, want PROVIDER_NOT_READY", d.ErrorCode)
	}
}

func TestStringResolve(t *testing.T) {
	ctx := context.Background()
	of := newOFClient(t, func(c *shipeasy.Engine) {
		c.OverrideConfig("greeting", "hello")
	})
	d, err := of.StringValueDetails(ctx, "greeting", "default", openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("StringValueDetails: %v", err)
	}
	if d.Value != "hello" {
		t.Fatalf("value = %q, want hello", d.Value)
	}
	if d.Reason != openfeature.TargetingMatchReason {
		t.Fatalf("reason = %q, want TARGETING_MATCH", d.Reason)
	}
}

func TestStringDefault(t *testing.T) {
	ctx := context.Background()
	of := newOFClient(t, nil)
	d, err := of.StringValueDetails(ctx, "absent", "fallback", openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("StringValueDetails: %v", err)
	}
	if d.Value != "fallback" {
		t.Fatalf("value = %q, want fallback", d.Value)
	}
	if d.Reason != openfeature.DefaultReason {
		t.Fatalf("reason = %q, want DEFAULT", d.Reason)
	}
}

func TestStringTypeMismatch(t *testing.T) {
	ctx := context.Background()
	of := newOFClient(t, func(c *shipeasy.Engine) {
		c.OverrideConfig("num", float64(42))
	})
	d, err := of.StringValueDetails(ctx, "num", "fallback", openfeature.EvaluationContext{})
	if err == nil {
		t.Fatalf("expected type mismatch error")
	}
	if d.Value != "fallback" {
		t.Fatalf("value = %q, want fallback", d.Value)
	}
	if d.ErrorCode != openfeature.TypeMismatchCode {
		t.Fatalf("errorCode = %q, want TYPE_MISMATCH", d.ErrorCode)
	}
}

func TestFloatResolve(t *testing.T) {
	ctx := context.Background()
	of := newOFClient(t, func(c *shipeasy.Engine) {
		c.OverrideConfig("rate", float64(3.5))
	})
	d, err := of.FloatValueDetails(ctx, "rate", 1.0, openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("FloatValueDetails: %v", err)
	}
	if d.Value != 3.5 {
		t.Fatalf("value = %v, want 3.5", d.Value)
	}
	if d.Reason != openfeature.TargetingMatchReason {
		t.Fatalf("reason = %q, want TARGETING_MATCH", d.Reason)
	}
}

func TestIntResolve(t *testing.T) {
	ctx := context.Background()
	// JSON numbers decode to float64; verify int coercion.
	of := newOFClient(t, func(c *shipeasy.Engine) {
		c.OverrideConfig("limit", float64(7))
	})
	d, err := of.IntValueDetails(ctx, "limit", 1, openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("IntValueDetails: %v", err)
	}
	if d.Value != 7 {
		t.Fatalf("value = %v, want 7", d.Value)
	}
}

func TestFloatTypeMismatch(t *testing.T) {
	ctx := context.Background()
	of := newOFClient(t, func(c *shipeasy.Engine) {
		c.OverrideConfig("notnum", "abc")
	})
	d, err := of.FloatValueDetails(ctx, "notnum", 9.9, openfeature.EvaluationContext{})
	if err == nil {
		t.Fatalf("expected type mismatch error")
	}
	if d.Value != 9.9 {
		t.Fatalf("value = %v, want 9.9", d.Value)
	}
	if d.ErrorCode != openfeature.TypeMismatchCode {
		t.Fatalf("errorCode = %q, want TYPE_MISMATCH", d.ErrorCode)
	}
}

func TestObjectResolve(t *testing.T) {
	ctx := context.Background()
	want := map[string]any{"cta": "Buy now"}
	of := newOFClient(t, func(c *shipeasy.Engine) {
		c.OverrideConfig("billing_copy", want)
	})
	d, err := of.ObjectValueDetails(ctx, "billing_copy", map[string]any{}, openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("ObjectValueDetails: %v", err)
	}
	m, ok := d.Value.(map[string]any)
	if !ok || m["cta"] != "Buy now" {
		t.Fatalf("value = %#v, want %#v", d.Value, want)
	}
	if d.Reason != openfeature.TargetingMatchReason {
		t.Fatalf("reason = %q, want TARGETING_MATCH", d.Reason)
	}
}

func TestObjectDefault(t *testing.T) {
	ctx := context.Background()
	of := newOFClient(t, nil)
	def := map[string]any{"k": "v"}
	d, err := of.ObjectValueDetails(ctx, "absent_obj", def, openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("ObjectValueDetails: %v", err)
	}
	if d.Reason != openfeature.DefaultReason {
		t.Fatalf("reason = %q, want DEFAULT", d.Reason)
	}
}

// TestTargetingKeyToUser verifies the targeting key flows to user_id so the gate
// can bucket/target on it. A gate rule on user_id confirms the mapping.
func TestTargetingKeyToUser(t *testing.T) {
	ctx := context.Background()
	sc := snapshotClient(t, `{"gates":{"vip":{"enabled":true,"killswitch":false,"salt":"s","rolloutPct":10000,"rules":[{"attr":"user_id","op":"eq","value":"alice"}]}},"configs":{}}`)
	of := ofClientFor(t, sc)

	on, err := of.BooleanValue(ctx, "vip", false, openfeature.NewEvaluationContext("alice", nil))
	if err != nil {
		t.Fatalf("BooleanValue(alice): %v", err)
	}
	if !on {
		t.Fatalf("alice should match vip rule")
	}
	off, err := of.BooleanValue(ctx, "vip", false, openfeature.NewEvaluationContext("bob", nil))
	if err != nil {
		t.Fatalf("BooleanValue(bob): %v", err)
	}
	if off {
		t.Fatalf("bob should NOT match vip rule")
	}
}
