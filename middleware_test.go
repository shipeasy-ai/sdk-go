package shipeasy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMintAnonIDFormat(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := MintAnonID()
		if !anonIDRx.MatchString(id) {
			t.Fatalf("minted id %q does not match the cross-SDK charset", id)
		}
		// UUIDv4 shape: 8-4-4-4-12 with version nibble 4.
		if len(id) != 36 || id[14] != '4' {
			t.Fatalf("minted id %q is not a v4 UUID", id)
		}
	}
	if MintAnonID() == MintAnonID() {
		t.Fatal("MintAnonID returned a duplicate")
	}
}

func TestReadOrMintAnonID(t *testing.T) {
	// Valid existing cookie is returned untouched.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: AnonIDCookie, Value: "abc-123_XYZ"})
	if id, minted := ReadOrMintAnonID(r); id != "abc-123_XYZ" || minted {
		t.Fatalf("valid cookie: got (%q, %v), want (abc-123_XYZ, false)", id, minted)
	}

	// Absent cookie mints.
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	if id, minted := ReadOrMintAnonID(r); !minted || !anonIDRx.MatchString(id) {
		t.Fatalf("absent cookie: got (%q, %v), want a freshly minted id", id, minted)
	}

	// Tampered (out-of-charset) cookie is treated as absent → mint.
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: AnonIDCookie, Value: "bad value!"})
	if id, minted := ReadOrMintAnonID(r); !minted || id == "bad value!" {
		t.Fatalf("tampered cookie: got (%q, %v), want a freshly minted id", id, minted)
	}
}

func TestMiddlewareMintsAndExposes(t *testing.T) {
	var seen string
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = AnonID(r)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))

	if seen == "" || !anonIDRx.MatchString(seen) {
		t.Fatalf("handler saw AnonID %q, want a minted id", seen)
	}
	cs := rec.Result().Cookies()
	if len(cs) != 1 || cs[0].Name != AnonIDCookie || cs[0].Value != seen {
		t.Fatalf("expected one %s cookie matching the resolved id, got %+v", AnonIDCookie, cs)
	}
	c := cs[0]
	if c.Path != "/" || c.MaxAge != anonIDMaxAge || c.SameSite != http.SameSiteLaxMode || c.HttpOnly {
		t.Fatalf("cookie attributes off contract: %+v", c)
	}
	if !c.Secure {
		t.Fatal("cookie should be Secure on an HTTPS request")
	}
}

func TestMiddlewareReusesExistingCookie(t *testing.T) {
	var seen string
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = AnonID(r)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.AddCookie(&http.Cookie{Name: AnonIDCookie, Value: "stable-id-1"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if seen != "stable-id-1" {
		t.Fatalf("handler saw %q, want the existing cookie value", seen)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("must not re-set the cookie when a valid one is already present")
	}
}

func TestRequestIsHTTPS(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	if requestIsHTTPS(r) {
		t.Fatal("plain http request reported HTTPS")
	}
	r.Header.Set("X-Forwarded-Proto", "https, http")
	if !requestIsHTTPS(r) {
		t.Fatal("X-Forwarded-Proto=https not honoured")
	}
}

func TestAnonIDWithoutMiddleware(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if AnonID(r) != "" {
		t.Fatal("AnonID should be empty when Middleware did not run")
	}
}
