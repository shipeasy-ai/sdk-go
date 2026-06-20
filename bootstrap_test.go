package shipeasy

import (
	"encoding/json"
	"strings"
	"testing"
)

func seedClient() *Client {
	c := NewTestClient()
	c.flags = &flagsBlob{
		Gates: map[string]gate{
			"new_ui":   {Enabled: true, Salt: "s", RolloutPct: 10000},
			"off_gate": {Enabled: false, Salt: "s", RolloutPct: 10000},
		},
		Configs: map[string]struct {
			Value any `json:"value"`
		}{
			"theme": {Value: map[string]any{"color": "blue"}},
		},
	}
	c.exps = &expsBlob{Experiments: map[string]experiment{}}
	return c
}

func TestEvaluateBuildsPayload(t *testing.T) {
	c := seedClient()
	b := c.Evaluate(User{"user_id": "u1"})
	if !b.Flags["new_ui"] {
		t.Fatal("100% gate should evaluate true")
	}
	if b.Flags["off_gate"] {
		t.Fatal("disabled gate should evaluate false")
	}
	if _, ok := b.Configs["theme"]; !ok {
		t.Fatal("config should be present")
	}
	if b.Killswitches == nil {
		t.Fatal("killswitches must be a (possibly empty) map, not nil")
	}
}

func TestBootstrapScriptTagAttrs(t *testing.T) {
	c := seedClient()
	tag := c.BootstrapScriptTag(User{"user_id": "u1"}, BootstrapTagOptions{AnonID: "anon-1"})

	for _, want := range []string{
		`src="https://cdn.shipeasy.ai/sdk/bootstrap.js"`,
		"data-se-bootstrap",
		"data-flags=",
		"data-configs=",
		"data-experiments=",
		"data-killswitches=",
		`data-anon-id="anon-1"`,
		`data-i18n-profile="en:prod"`,
	} {
		if !strings.Contains(tag, want) {
			t.Fatalf("tag missing %q\n%s", want, tag)
		}
	}
	// No key of any kind is embedded.
	if strings.Contains(tag, "data-key") {
		t.Fatalf("bootstrap tag must not carry a key: %s", tag)
	}

	// data-flags holds valid JSON once HTML entities are decoded.
	raw := tag[strings.Index(tag, `data-flags="`)+len(`data-flags="`):]
	raw = raw[:strings.Index(raw, `"`)]
	decoded := strings.NewReplacer("&#34;", `"`, "&amp;", "&", "&lt;", "<", "&gt;", ">").Replace(raw)
	var flags map[string]bool
	if err := json.Unmarshal([]byte(decoded), &flags); err != nil {
		t.Fatalf("data-flags is not valid JSON: %v (%s)", err, decoded)
	}
	if !flags["new_ui"] {
		t.Fatal("decoded data-flags should carry new_ui=true")
	}
}

func TestBootstrapScriptTagNoAnonWhenEmpty(t *testing.T) {
	c := seedClient()
	tag := c.BootstrapScriptTag(User{"user_id": "u1"}, BootstrapTagOptions{})
	if strings.Contains(tag, "data-anon-id") {
		t.Fatalf("no anon id should be emitted when unset: %s", tag)
	}
}

func TestI18nScriptTag(t *testing.T) {
	c := seedClient()
	tag := c.I18nScriptTag("client_pub", "fr:prod", BootstrapTagOptions{})
	for _, want := range []string{
		`src="https://cdn.shipeasy.ai/sdk/i18n/loader.js"`,
		`data-key="client_pub"`,
		`data-profile="fr:prod"`,
	} {
		if !strings.Contains(tag, want) {
			t.Fatalf("i18n tag missing %q\n%s", want, tag)
		}
	}
}
