package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	shipeasy "github.com/shipeasy-ai/sdk-go"
)

// TestGuidePageRendersMockedValues seeds every Shipeasy value the guide reads
// via the SDK's TESTING setup (ConfigureForTesting — zero network), builds the
// gin router in-process, fetches GET "/", and asserts the rendered HTML
// contains each mocked value.
//
// It runs entirely in-process (httptest, no real server, no network) so the
// ConfigureForTesting mock applies in the same process that renders the page.
func TestGuidePageRendersMockedValues(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Mock every value shipeasy returns. Keys match main.go's entities():
	//   flag        new_checkout
	//   config      billing_copy
	//   experiment  checkout_button
	shipeasy.ConfigureForTesting(shipeasy.TestOptions{
		Flags: map[string]bool{
			"new_checkout": true,
		},
		Configs: map[string]any{
			// A plain string so it renders verbatim (formatAny returns it as-is)
			// and survives html/template escaping without entity-encoding.
			// Distinctive sentinel — NOT a placeholder from main.go — so a green
			// assertion honestly proves the SDK→page path rendered the mock.
			"billing_copy": "Start free trial",
		},
		Experiments: map[string]shipeasy.ExperimentOverride{
			"checkout_button": {
				Group:  "treatment",
				Params: map[string]any{"color": "#0ea5e9", "label": "Checkout now"},
			},
		},
	})

	// Bind the client AFTER ConfigureForTesting so reads hit the seeded values.
	client := shipeasy.NewClient(shipeasy.User{"user_id": "u_123", "plan": "pro"})

	router := newRouter(client)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d; body:\n%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	body := rec.Body.String()

	// Each mocked value's rendered string must appear in the HTML.
	//
	// Flag: main.go renders fmt.Sprintf("%t", featureOn) → "true".
	// Config: formatValue("Start free trial", true) → the string verbatim.
	// Experiment: fmt.Sprintf("%s · %s", exp.Group, formatAny(exp.Params))
	//   → "treatment · {...json...}". We assert on stable substrings of that
	//   value: the group, and the param tokens. (The JSON's quotes are
	//   HTML-escaped by html/template to &#34;, so we match unquoted tokens.)
	//
	// These are DISTINCTIVE sentinel values (not main.go's placeholders), so a
	// green run proves the wired SDK→page path actually rendered the mocks.
	wantContains := []string{
		"true",             // new_checkout flag value
		"Start free trial", // billing_copy config value
		"treatment",        // checkout_button experiment group
		"#0ea5e9",          // experiment param value (color)
		"Checkout now",     // experiment param value (label)
	}

	for _, want := range wantContains {
		if !strings.Contains(body, want) {
			t.Errorf("rendered HTML does not contain mocked value %q", want)
		}
	}

	if t.Failed() {
		t.Logf("rendered body:\n%s", body)
	}
}
