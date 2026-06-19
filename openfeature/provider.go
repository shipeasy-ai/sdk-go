// Package shipeasyopenfeature provides an OpenFeature provider for Shipeasy.
//
// It lets apps standardized on the CNCF OpenFeature API plug Shipeasy in as the
// backing flag provider. It is a thin, pure adapter over the SDK's existing
// *shipeasy.Client — no change to evaluation:
//
//	import (
//		"github.com/open-feature/go-sdk/openfeature"
//		shipeasy "github.com/shipeasy-ai/sdk-go"
//		shipeasyof "github.com/shipeasy-ai/sdk-go/openfeature"
//	)
//
//	client := shipeasy.NewClient(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})
//	_ = client.Init(ctx)
//	_ = openfeature.SetProviderAndWait(shipeasyof.NewProvider(client))
//
//	of := openfeature.NewClient("app")
//	on, _ := of.BooleanValue(ctx, "new_checkout", false, openfeature.NewEvaluationContext("u1", nil))
//
// This lives in its own nested Go module so the base SDK module does not pull in
// github.com/open-feature/go-sdk for consumers that don't use OpenFeature.
package shipeasyopenfeature

import (
	"context"

	"github.com/open-feature/go-sdk/openfeature"
	shipeasy "github.com/shipeasy-ai/sdk-go"
)

// providerName is reported by Metadata().Name (OpenFeature provider identity).
const providerName = "shipeasy"

// Provider implements the OpenFeature openfeature.FeatureProvider interface by
// wrapping a *shipeasy.Client. Boolean flags resolve through the SDK's gate
// evaluation (GetFlagDetail); string/float/int/object flags resolve through the
// SDK's dynamic configs (GetConfig). Resolution is local against the cached
// blob, so there is no network on the evaluation path.
type Provider struct {
	client *shipeasy.Client
}

// NewProvider wraps a *shipeasy.Client as an OpenFeature provider.
func NewProvider(client *shipeasy.Client) *Provider {
	return &Provider{client: client}
}

// compile-time assertion that *Provider satisfies the OpenFeature contract.
var _ openfeature.FeatureProvider = (*Provider)(nil)

// Metadata returns the provider identity ("shipeasy").
func (p *Provider) Metadata() openfeature.Metadata {
	return openfeature.Metadata{Name: providerName}
}

// Hooks returns no provider hooks.
func (p *Provider) Hooks() []openfeature.Hook { return nil }

// toUser turns a flattened OpenFeature evaluation context into a shipeasy.User.
// TargetingKey becomes user_id; every other entry is carried through verbatim as
// a targeting attribute. A user_id/anonymous_id already present in the context
// is preserved when no targetingKey is supplied.
func toUser(flatCtx openfeature.FlattenedContext) shipeasy.User {
	user := shipeasy.User{}
	for k, v := range flatCtx {
		if k == openfeature.TargetingKey {
			continue
		}
		user[k] = v
	}
	if tk, ok := flatCtx[openfeature.TargetingKey]; ok {
		if s, ok := tk.(string); ok && s != "" {
			user["user_id"] = s
		}
	}
	return user
}

// mapReason maps a Shipeasy flag reason onto an OpenFeature reason and, when the
// flag could not be evaluated, a ResolutionError. The mapping (per doc 20):
//
//	RULE_MATCH       → TARGETING_MATCH
//	DEFAULT          → DEFAULT
//	OFF              → DISABLED
//	OVERRIDE         → STATIC
//	FLAG_NOT_FOUND   → ERROR + FlagNotFound
//	CLIENT_NOT_READY → ERROR + ProviderNotReady
func mapReason(reason string) (openfeature.Reason, *openfeature.ResolutionError) {
	switch reason {
	case shipeasy.ReasonRuleMatch:
		return openfeature.TargetingMatchReason, nil
	case shipeasy.ReasonDefault:
		return openfeature.DefaultReason, nil
	case shipeasy.ReasonOff:
		return openfeature.DisabledReason, nil
	case shipeasy.ReasonOverride:
		return openfeature.StaticReason, nil
	case shipeasy.ReasonFlagNotFound:
		e := openfeature.NewFlagNotFoundResolutionError("flag not found")
		return openfeature.ErrorReason, &e
	case shipeasy.ReasonClientNotReady:
		e := openfeature.NewProviderNotReadyResolutionError("provider not ready")
		return openfeature.ErrorReason, &e
	default:
		return openfeature.UnknownReason, nil
	}
}

// BooleanEvaluation resolves a gate. On any reason that maps to an error the
// default value is returned with the resolution error set.
func (p *Provider) BooleanEvaluation(_ context.Context, flag string, defaultValue bool, flatCtx openfeature.FlattenedContext) openfeature.BoolResolutionDetail {
	detail := p.client.GetFlagDetail(flag, toUser(flatCtx))
	reason, resErr := mapReason(detail.Reason)
	if resErr != nil {
		return openfeature.BoolResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
				ResolutionError: *resErr,
				Reason:          reason,
			},
		}
	}
	return openfeature.BoolResolutionDetail{
		Value: detail.Value,
		ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
			Reason: reason,
		},
	}
}

// StringEvaluation resolves a string config.
func (p *Provider) StringEvaluation(_ context.Context, flag string, defaultValue string, _ openfeature.FlattenedContext) openfeature.StringResolutionDetail {
	raw, ok := p.client.GetConfig(flag)
	if !ok {
		return openfeature.StringResolutionDetail{
			Value:                    defaultValue,
			ProviderResolutionDetail: openfeature.ProviderResolutionDetail{Reason: openfeature.DefaultReason},
		}
	}
	v, isType := raw.(string)
	if !isType {
		return openfeature.StringResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
				ResolutionError: openfeature.NewTypeMismatchResolutionError("config value is not a string"),
				Reason:          openfeature.ErrorReason,
			},
		}
	}
	return openfeature.StringResolutionDetail{
		Value:                    v,
		ProviderResolutionDetail: openfeature.ProviderResolutionDetail{Reason: openfeature.TargetingMatchReason},
	}
}

// FloatEvaluation resolves a numeric config as float64. JSON numbers decode to
// float64, but ints from a test override or a typed value are accepted too.
func (p *Provider) FloatEvaluation(_ context.Context, flag string, defaultValue float64, _ openfeature.FlattenedContext) openfeature.FloatResolutionDetail {
	raw, ok := p.client.GetConfig(flag)
	if !ok {
		return openfeature.FloatResolutionDetail{
			Value:                    defaultValue,
			ProviderResolutionDetail: openfeature.ProviderResolutionDetail{Reason: openfeature.DefaultReason},
		}
	}
	v, isType := toFloat(raw)
	if !isType {
		return openfeature.FloatResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
				ResolutionError: openfeature.NewTypeMismatchResolutionError("config value is not a number"),
				Reason:          openfeature.ErrorReason,
			},
		}
	}
	return openfeature.FloatResolutionDetail{
		Value:                    v,
		ProviderResolutionDetail: openfeature.ProviderResolutionDetail{Reason: openfeature.TargetingMatchReason},
	}
}

// IntEvaluation resolves a numeric config as int64.
func (p *Provider) IntEvaluation(_ context.Context, flag string, defaultValue int64, _ openfeature.FlattenedContext) openfeature.IntResolutionDetail {
	raw, ok := p.client.GetConfig(flag)
	if !ok {
		return openfeature.IntResolutionDetail{
			Value:                    defaultValue,
			ProviderResolutionDetail: openfeature.ProviderResolutionDetail{Reason: openfeature.DefaultReason},
		}
	}
	f, isType := toFloat(raw)
	if !isType {
		return openfeature.IntResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
				ResolutionError: openfeature.NewTypeMismatchResolutionError("config value is not a number"),
				Reason:          openfeature.ErrorReason,
			},
		}
	}
	return openfeature.IntResolutionDetail{
		Value:                    int64(f),
		ProviderResolutionDetail: openfeature.ProviderResolutionDetail{Reason: openfeature.TargetingMatchReason},
	}
}

// ObjectEvaluation resolves an object/array config. Any non-nil config value is
// returned as-is; an absent key falls back to the default with reason Default.
func (p *Provider) ObjectEvaluation(_ context.Context, flag string, defaultValue any, _ openfeature.FlattenedContext) openfeature.InterfaceResolutionDetail {
	raw, ok := p.client.GetConfig(flag)
	if !ok || raw == nil {
		return openfeature.InterfaceResolutionDetail{
			Value:                    defaultValue,
			ProviderResolutionDetail: openfeature.ProviderResolutionDetail{Reason: openfeature.DefaultReason},
		}
	}
	return openfeature.InterfaceResolutionDetail{
		Value:                    raw,
		ProviderResolutionDetail: openfeature.ProviderResolutionDetail{Reason: openfeature.TargetingMatchReason},
	}
}

// toFloat coerces a config value (float64 from JSON, or a native int/float) into
// a float64. Returns ok=false for non-numeric values (so the caller emits a
// TypeMismatch). Strings are intentionally NOT coerced — a stringly-typed config
// is a type mismatch for a numeric flag.
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	default:
		return 0, false
	}
}
