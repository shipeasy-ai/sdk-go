package shipeasy

// Anonymous bucketing identity — the cross-SDK `__se_anon_id` cookie.
//
// Gates and experiments bucket a unit with murmur3(salt:unit). For a logged-out
// visitor the unit is a stable anonymous id carried in a single first-party
// cookie that EVERY Shipeasy SDK (server + browser) reads and writes, so a
// server render and the browser bucket a fractional rollout identically — no
// flash, no disagreement. The cookie name and format are frozen across every
// language; see experiment-platform/18-identity-bucketing.md.

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// AnonIDCookie is the first-party cookie carrying the stable anonymous
// bucketing unit. Do not change it — it is a cross-SDK contract.
const AnonIDCookie = "__se_anon_id"

// One year. Refresh-on-read is intentionally not done (the value is stable).
const anonIDMaxAge = 60 * 60 * 24 * 365

// The cookie value is client-controllable and feeds bucketing, so a tampered
// value is treated as absent and a fresh id is minted. UUIDs satisfy this.
var anonIDRx = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type anonIDKey struct{}

// MintAnonID returns a fresh opaque bucketing id (a UUIDv4). Exposed for callers
// that mint outside the HTTP path; Middleware uses it internally.
func MintAnonID() string {
	var b [16]byte
	// crypto/rand.Read does not fail on supported platforms; on the pathological
	// error path b stays zero, which still yields a well-formed (fixed) UUID
	// rather than panicking inside a request.
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ReadOrMintAnonID returns the request's stable bucketing id — the existing
// __se_anon_id cookie when present and well-formed, otherwise a freshly minted
// one. minted is true when there was no valid cookie, in which case the caller
// must persist the id with SetAnonIDCookie. Most callers should use Middleware
// rather than calling this directly.
func ReadOrMintAnonID(r *http.Request) (id string, minted bool) {
	if c, err := r.Cookie(AnonIDCookie); err == nil && anonIDRx.MatchString(c.Value) {
		return c.Value, false
	}
	return MintAnonID(), true
}

// SetAnonIDCookie writes the anon id as a first-party cookie on the response.
// It is deliberately NOT HttpOnly — the browser SDK reads it via document.cookie
// to bucket identically to the server. Secure is set for HTTPS requests.
func SetAnonIDCookie(w http.ResponseWriter, r *http.Request, id string) {
	http.SetCookie(w, &http.Cookie{
		Name:     AnonIDCookie,
		Value:    id,
		Path:     "/",
		MaxAge:   anonIDMaxAge,
		HttpOnly: false,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// requestIsHTTPS reports whether the original request was HTTPS, honouring a
// terminating proxy's X-Forwarded-Proto (so Secure is set correctly behind a
// load balancer where r.TLS is nil).
func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if i := strings.IndexByte(proto, ','); i >= 0 {
		proto = proto[:i]
	}
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

// Middleware mints the shared __se_anon_id bucketing cookie for any request that
// lacks a valid one, exposes the resolved id on the request context, and
// persists a freshly minted id on the response. Wrap your handler once and
// anonymous visitors bucket consistently across server renders and the browser
// from the very first request, with no per-call wiring.
//
//	mux := http.NewServeMux()
//	// ... register handlers ...
//	http.ListenAndServe(":8080", shipeasy.Middleware(mux))
//
// Inside a handler, read the id with AnonID(r) and pass it as the bucketing
// unit when the visitor is anonymous:
//
//	on := client.GetFlag("new_checkout", shipeasy.User{"anonymous_id": shipeasy.AnonID(r)})
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, minted := ReadOrMintAnonID(r)
		if minted {
			SetAnonIDCookie(w, r, id)
		}
		ctx := context.WithValue(r.Context(), anonIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AnonID returns the stable anonymous bucketing id resolved by Middleware for
// this request, or "" if Middleware did not run. Pass it as anonymous_id for
// logged-out visitors.
func AnonID(r *http.Request) string {
	if id, ok := r.Context().Value(anonIDKey{}).(string); ok {
		return id
	}
	return ""
}
