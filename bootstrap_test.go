package shipeasy

import (
	"encoding/json"
	"strings"
	"testing"
)

func seedClient() *Engine {
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

func TestBootstrapScriptTagCarriesIdentifiedUser(t *testing.T) {
	c := seedClient()
	user := User{"user_id": "u1", "email": "u1@example.com", "anonymous_id": "anon-1"}
	tag := c.BootstrapScriptTag(user, BootstrapTagOptions{AnonID: "anon-1"})

	// anonymous_id still rides data-anon-id.
	if !strings.Contains(tag, `data-anon-id="anon-1"`) {
		t.Fatalf("expected data-anon-id on the tag: %s", tag)
	}

	// data-user is the HTML-escaped JSON of the traits minus anonymous_id.
	// Go's json.Marshal sorts map keys, so the object is deterministic.
	wantJSON := `{"email":"u1@example.com","user_id":"u1"}`
	escaped := strings.NewReplacer(`"`, "&#34;", "&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(wantJSON)
	if !strings.Contains(tag, `data-user="`+escaped+`"`) {
		t.Fatalf("tag missing data-user=%q (escaped %q)\n%s", wantJSON, escaped, tag)
	}
	// anonymous_id must not leak into data-user.
	raw := tag[strings.Index(tag, `data-user="`)+len(`data-user="`):]
	raw = raw[:strings.Index(raw, `"`)]
	if strings.Contains(raw, "anonymous_id") {
		t.Fatalf("data-user must not carry anonymous_id: %s", raw)
	}
}

func TestBootstrapScriptTagNoUserWhenAnonymous(t *testing.T) {
	c := seedClient()
	// Only an anonymous_id => no identity, no data-user.
	tag := c.BootstrapScriptTag(User{"anonymous_id": "anon-1"}, BootstrapTagOptions{AnonID: "anon-1"})
	if strings.Contains(tag, "data-user") {
		t.Fatalf("anonymous user must not emit data-user: %s", tag)
	}
	// Empty user => no data-user either.
	tag = c.BootstrapScriptTag(User{}, BootstrapTagOptions{})
	if strings.Contains(tag, "data-user") {
		t.Fatalf("empty user must not emit data-user: %s", tag)
	}
}

func TestI18nScriptTag(t *testing.T) {
	c := seedClient()
	tag := c.I18nScriptTag(TagOptions{ClientKey: "client_pub", I18nProfile: "fr:prod"})
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

// --- every argument is optional: the tags read what Configure was given ------

// configuredClient is a local engine carrying the SSR tag defaults Configure
// would have set, so the helpers can be called with no options at all.
func configuredClient() *Engine {
	c := seedClient()
	c.clientKey = "sdk_client_cfg"
	c.projectID = "proj_cfg"
	c.profile = "fr:prod"
	c.cdnBaseURL = "https://cdn.example.test"
	return c
}

func TestTagsDefaultFromConfigure(t *testing.T) {
	c := configuredClient()
	for _, tc := range []struct {
		name, tag string
		want      []string
	}{
		{"i18n", c.I18nScriptTag(), []string{
			`src="https://cdn.example.test/sdk/i18n/loader.js"`,
			`data-key="sdk_client_cfg"`,
			`data-profile="fr:prod"`,
		}},
		{"bootstrap", c.BootstrapScriptTag(nil), []string{
			`src="https://cdn.example.test/sdk/bootstrap.js"`,
			`data-i18n-profile="fr:prod"`,
		}},
		{"devtools", c.DevtoolsScriptTag(), []string{
			`src="https://cdn.example.test/se-devtools.js"`,
			`data-project-id="proj_cfg"`,
			`data-client-api-key="sdk_client_cfg"`,
			`defer`,
		}},
	} {
		for _, want := range tc.want {
			if !strings.Contains(tc.tag, want) {
				t.Fatalf("%s tag missing %q\n%s", tc.name, want, tc.tag)
			}
		}
	}
	// A nil user is an anonymous request — no identity on the tag.
	if strings.Contains(c.BootstrapScriptTag(nil), "data-user") {
		t.Fatalf("nil user must not emit data-user")
	}
}

func TestExplicitTagOptionsWin(t *testing.T) {
	c := configuredClient()
	i18n := c.I18nScriptTag(TagOptions{ClientKey: "other_key", I18nProfile: "de:prod"})
	if !strings.Contains(i18n, `data-key="other_key"`) || !strings.Contains(i18n, `data-profile="de:prod"`) {
		t.Fatalf("explicit i18n options ignored: %s", i18n)
	}
	boot := c.BootstrapScriptTag(User{"user_id": "u1"}, TagOptions{I18nProfile: "de:prod"})
	if !strings.Contains(boot, `data-i18n-profile="de:prod"`) {
		t.Fatalf("explicit bootstrap profile ignored: %s", boot)
	}
	dev := c.DevtoolsScriptTag(TagOptions{ProjectID: "proj_other", ClientKey: "other_key", NoDefer: true})
	if !strings.Contains(dev, `data-project-id="proj_other"`) || !strings.Contains(dev, `data-client-api-key="other_key"`) {
		t.Fatalf("explicit devtools options ignored: %s", dev)
	}
	if strings.Contains(dev, "defer") {
		t.Fatalf("NoDefer must drop the defer attribute: %s", dev)
	}
}

func TestDevtoolsTagRendersWhenUnconfigured(t *testing.T) {
	// A missing project id / client key renders anyway (the browser bundle
	// reports what it needs) — a tag helper must never break a template.
	tag := seedClient().DevtoolsScriptTag()
	if !strings.Contains(tag, `src="https://cdn.shipeasy.ai/se-devtools.js"`) {
		t.Fatalf("devtools tag did not render: %s", tag)
	}
	if !strings.Contains(tag, `data-project-id=""`) {
		t.Fatalf("devtools tag should carry an empty project id: %s", tag)
	}
}
