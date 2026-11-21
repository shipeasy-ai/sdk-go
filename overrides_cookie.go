package shipeasy

// Signed DevTools override cookie (`se_ov`) — verify only. This is a Go port of
// the canonical implementation in the shipeasy monorepo
// (packages/core/src/overrides/cookie.ts) and MUST stay byte-compatible with it:
// the derivation formulas and the body signing are a cross-SDK contract guarded
// by the shared fixture test (overrides_cookie_test.go).
//
//	Kp = project verify key, served as `overrideKey` in the /sdk/flags payload
//	Ks = HMAC-SHA256(Kp, "se-ov-sess:v1:<nonce>:<exp>")
//	sig = HMAC-SHA256(Ks, <body, the base64url text as-is>)
//	cookie = body + "." + sig, body = base64url(JSON payload), no padding

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// OverridesHeader is the request header carrying the override cookies in
// cookie-header syntax (`se_ov=<v>; se_ov_dev=<v>`), for APIs that cannot see
// the browser's cookies (a sibling subdomain, token auth). The browser SDK's
// getOverrideHeaders() builds it.
const OverridesHeader = "X-Shipeasy-Overrides"

// OverrideCookieName is the cookie carrying the signed override payload.
const OverrideCookieName = "se_ov"

// overrideFormatVersion must match core's OVERRIDE_FORMAT_VERSION.
const overrideFormatVersion = 1

// requestOverrides is the verified override set from a signed se_ov cookie.
// gates covers feature gates AND kill switches.
type requestOverrides struct {
	gates   map[string]bool
	configs map[string]any
	exps    map[string]string
}

func b64uDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

func hmacSHA256(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}

// verifyOverrideCookie verifies a bare se_ov cookie value against the project
// key (base64url, as served in the flags payload). It fails closed: any
// malformation, key-id mismatch, expiry or bad signature returns an error.
func verifyOverrideCookie(cookieValue, encodedProjectKey, keyID string, now time.Time) (*requestOverrides, error) {
	projectKey, err := b64uDecode(encodedProjectKey)
	if err != nil {
		return nil, fmt.Errorf("bad-key")
	}
	dot := strings.Index(cookieValue, ".")
	if dot <= 0 || dot != strings.LastIndex(cookieValue, ".") {
		return nil, fmt.Errorf("malformed")
	}
	body, sig := cookieValue[:dot], cookieValue[dot+1:]

	raw, err := b64uDecode(body)
	if err != nil {
		return nil, fmt.Errorf("bad-json")
	}
	var p struct {
		V     *float64 `json:"v"`
		Pid   *string  `json:"pid"`
		KeyID *string  `json:"keyId"`
		Nonce *string  `json:"nonce"`
		Exp   *float64 `json:"exp"`
		Ov    *struct {
			Gates   map[string]bool   `json:"gates"`
			Configs map[string]any    `json:"configs"`
			Exps    map[string]string `json:"exps"`
		} `json:"ov"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("bad-json")
	}
	if p.V == nil || *p.V != overrideFormatVersion || p.Pid == nil || p.KeyID == nil ||
		p.Nonce == nil || p.Exp == nil || p.Ov == nil {
		return nil, fmt.Errorf("bad-shape")
	}
	if *p.KeyID != keyID {
		return nil, fmt.Errorf("wrong-key-id")
	}
	if *p.Exp <= float64(now.Unix()) {
		return nil, fmt.Errorf("expired")
	}
	sessionKey := hmacSHA256(projectKey, fmt.Sprintf("se-ov-sess:v1:%s:%d", *p.Nonce, int64(*p.Exp)))
	got, err := b64uDecode(sig)
	if err != nil || !hmac.Equal(hmacSHA256(sessionKey, body), got) {
		return nil, fmt.Errorf("bad-sig")
	}
	ov := &requestOverrides{gates: p.Ov.Gates, configs: p.Ov.Configs, exps: p.Ov.Exps}
	if ov.gates == nil {
		ov.gates = map[string]bool{}
	}
	if ov.configs == nil {
		ov.configs = map[string]any{}
	}
	if ov.exps == nil {
		ov.exps = map[string]string{}
	}
	return ov, nil
}

// extractOverrideCookie accepts either a bare se_ov value or a full `Cookie:`
// header (`a=1; se_ov=...; b=2`) and returns the URL-decoded value, or "" when
// absent.
func extractOverrideCookie(input string) string {
	if input == "" {
		return ""
	}
	if !strings.Contains(input, ";") && !strings.Contains(input, "se_ov=") {
		if strings.Contains(input, ".") {
			return input
		}
		return ""
	}
	for _, part := range strings.Split(input, ";") {
		eq := strings.Index(part, "=")
		if eq < 0 {
			continue
		}
		if strings.TrimSpace(part[:eq]) == OverrideCookieName {
			v := strings.TrimSpace(part[eq+1:])
			if dec, err := url.QueryUnescape(v); err == nil {
				return dec
			}
			return v
		}
	}
	return ""
}

// DevOverrideCookieName is the UNSIGNED dev-only cookie the browser SDK writes
// for ?se_gate_x=true outside production. Honored only with Options.DevOverrides
// in a non-production process.
const DevOverrideCookieName = "se_ov_dev"

// extractDevOverrideCookie returns the raw se_ov_dev value from a Cookie header
// (or a bare value, which has no '.', ';' or '='), URL-decoded, or "".
func extractDevOverrideCookie(input string) string {
	if input == "" {
		return ""
	}
	if !strings.ContainsAny(input, ";=.") {
		return input
	}
	for _, part := range strings.Split(input, ";") {
		eq := strings.Index(part, "=")
		if eq < 0 {
			continue
		}
		if strings.TrimSpace(part[:eq]) == DevOverrideCookieName {
			v := strings.TrimSpace(part[eq+1:])
			if dec, err := url.QueryUnescape(v); err == nil {
				return dec
			}
			return v
		}
	}
	return ""
}

// decodeDevOverrideCookie decodes base64url(JSON {gates,configs,exps}). Any
// malformation yields nil (ignored, never an error).
func decodeDevOverrideCookie(val string) *requestOverrides {
	if val == "" {
		return nil
	}
	raw, err := b64uDecode(val)
	if err != nil {
		return nil
	}
	var p struct {
		Gates   map[string]bool   `json:"gates"`
		Configs map[string]any    `json:"configs"`
		Exps    map[string]string `json:"exps"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return nil
	}
	ov := &requestOverrides{gates: p.Gates, configs: p.Configs, exps: p.Exps}
	if ov.gates == nil {
		ov.gates = map[string]bool{}
	}
	if ov.configs == nil {
		ov.configs = map[string]any{}
	}
	if ov.exps == nil {
		ov.exps = map[string]string{}
	}
	return ov
}
