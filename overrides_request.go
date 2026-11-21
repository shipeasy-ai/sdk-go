package shipeasy

import (
	"context"
	"time"
)

// Request-scoped DevTools overrides.
//
// When a signed-in DevTools overlay overrides a gate, config or experiment, it
// mirrors the override into the signed `se_ov` cookie, which the browser sends
// on every same-origin request. Middleware (or WithRequestOverrides, for apps
// that don't use it) verifies that cookie once per request and parks the
// verified set on the request's context; a Client bound to that context with
// WithContext applies it on every read.
//
// Fail closed: overrides apply only when Shipeasy served the project's override
// key (overrideKey / overrideKeyId in the flags payload), the signature is
// valid and the cookie is unexpired. Precedence on a read is programmatic
// Override* > signed cookie > normal evaluation. Outside a request scope, or
// with no key, behaviour is unchanged.

type requestOverridesKey struct{}

// overridesFromCookie verifies the Cookie header (or bare se_ov value) against
// the engine's override key. Returns nil when overrides are disabled or invalid.
func (c *Engine) overridesFromCookie(cookieHeader string) *requestOverrides {
	if c == nil {
		return nil
	}
	val := extractOverrideCookie(cookieHeader)
	if val == "" {
		return nil
	}
	c.mu.RLock()
	flags := c.flags
	c.mu.RUnlock()
	if flags == nil || flags.OverrideKey == "" || flags.OverrideKeyID == "" {
		return nil
	}
	ov, err := verifyOverrideCookie(val, flags.OverrideKey, flags.OverrideKeyID, time.Now())
	if err != nil {
		return nil
	}
	return ov
}

// WithRequestOverrides returns a context carrying the verified DevTools
// overrides from cookieHeader (a full Cookie header or the bare se_ov value),
// for apps that don't use Middleware. Verification happens once, here. An
// absent, invalid, expired or unverifiable cookie yields a context with no
// overrides (and clears any inherited from ctx). Pass the result to
// Client.WithContext:
//
//	ctx := shipeasy.WithRequestOverrides(r.Context(), r.Header.Get("Cookie"))
//	c := shipeasy.NewClient(user).WithContext(ctx)
func WithRequestOverrides(ctx context.Context, cookieHeader string) context.Context {
	var ov *requestOverrides
	if eng := resolveGlobalEngine(); eng != nil {
		ov = eng.overridesFromCookie(cookieHeader)
	}
	return context.WithValue(ctx, requestOverridesKey{}, ov)
}

func requestOverridesFrom(ctx context.Context) *requestOverrides {
	if ctx == nil {
		return nil
	}
	ov, _ := ctx.Value(requestOverridesKey{}).(*requestOverrides)
	return ov
}
