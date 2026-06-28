// Command guide renders a single-page "entity guide" for the Shipeasy Go SDK.
//
// It is a Gin web app whose one page reads like a guide document: one card per
// Shipeasy entity (feature flag, dynamic config, A/B experiment, kill switch,
// event/metric, i18n label, error reporting). Run it with `go run .` and open
// http://localhost:8080.
//
// The guide now wires the installed Shipeasy Go SDK at startup so the flag,
// config, experiment, and kill-switch cards reflect the live SDK calls from the
// version pinned in this module.
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	shipeasy "github.com/shipeasy-ai/sdk-go"
)

// Entity is one card in the guide.
type Entity struct {
	Label  string // UPPERCASE type label, e.g. "FEATURE FLAG"
	Accent string // hex accent colour for this entity
	Key    string // the entity key, shown as the card title (mono)
	Value  string // the (placeholder) resolved value, shown in the right-hand pill
	Desc   string // one-line plain-English description
	Call   string // the real SDK call, rendered as a code block
	Meta   string // faint meta line under the code block
}

func entities(c *shipeasy.Client) []Entity {
	featureOn := c.GetFlag("new_checkout")
	cfg, cfgOK := c.GetConfig("billing_copy")
	exp := c.GetExperiment("checkout_button", map[string]any{"color": "#888", "label": "Buy"})
	paused := c.GetKillswitch("payments_paused")

	return []Entity{
		// 1. FEATURE FLAG
		{
			Label:  "FEATURE FLAG",
			Accent: "#34d399",
			Key:    "new_checkout",
			Value:  fmt.Sprintf("%t", featureOn),
			Desc:   "A boolean on/off switch with targeting rules + percentage rollout.",
			Call:   `on := c.GetFlag("new_checkout")`,
			Meta:   "evaluated once at request time for the bound user",
		},

		// 2. DYNAMIC CONFIG
		{
			Label:  "DYNAMIC CONFIG",
			Accent: "#60a5fa",
			Key:    "billing_copy",
			Value:  formatValue(cfg, cfgOK),
			Desc:   "A typed JSON blob you change without deploying.",
			Call:   `cfg, _ := c.GetConfig("billing_copy")`,
			Meta:   "returns the raw config payload and a found bit",
		},

		// 3. A/B EXPERIMENT
		{
			Label:  "A/B EXPERIMENT",
			Accent: "#c084fc",
			Key:    "checkout_button",
			Value:  fmt.Sprintf("%s · %s", exp.Group, formatAny(exp.Params)),
			Desc:   "Splits users into variants and measures a metric.",
			Call:   `r := c.GetExperiment("checkout_button", map[string]any{"color":"#888","label":"Buy"})`,
			Meta:   "r.InExperiment == true · r.Group == \"treatment\" · read params from r.Params",
		},

		// 4. KILL SWITCH
		{
			Label:  "KILL SWITCH",
			Accent: "#f87171",
			Key:    "payments_paused",
			Value:  formatKillSwitch(paused),
			Desc:   "An operational off-switch shipped alongside flags — flip it to disable a subsystem during an incident.",
			Call:   `paused := c.GetKillswitch("payments_paused")`,
			Meta:   "operational toggles are read from the same configured engine",
		},

		// 5. EVENT / METRIC
		{
			Label:  "EVENT / METRIC",
			Accent: "#22d3ee",
			Key:    "checkout_completed",
			Value:  `last event queued · {"revenue":49.99,"plan":"pro"}`,
			Desc:   "Fire-and-forget events that power experiment metrics + dashboards.",
			Call:   `shipeasy.ConfiguredEngine().Track("u_123", "checkout_completed", map[string]any{"revenue":49.99,"plan":"pro"})`,
			Meta:   "non-blocking · batched by the shared engine",
		},

		// 6. I18N LABEL
		{
			Label:  "I18N LABEL",
			Accent: "#fbbf24",
			Key:    "hero.title",
			Value:  "Ship features, not stress",
			Desc:   "Server-managed copy you translate + publish from the dashboard — no redeploy. (i18n for the Go SDK ships as a follow-up; shown here for completeness.)",
			Call:   `t("hero.title", map[string]any{"name":"Sam"})`,
			Meta:   "illustrative · the Go i18n helper is a follow-up to the flags/experiments SDK",
		},

		// 7. ERROR REPORTING — see()
		{
			Label:  "ERROR REPORTING",
			Accent: "#f87171",
			Key:    "see()",
			Value:  "0 issues reported this session",
			Desc:   "Structured error reports that document the product consequence, not just a stack trace.",
			Call:   `if err := submitOrder(o); err != nil { shipeasy.See(err).CausesThe("checkout").To("use cached prices").Extras(map[string]any{"order_id": o.ID}) }`,
			Meta:   "the consequence (\"causes checkout to use cached prices\") is the report's payload",
		},
	}
}

func formatValue(v any, ok bool) string {
	if !ok {
		return "missing"
	}
	return formatAny(v)
}

func formatKillSwitch(paused bool) string {
	if paused {
		return "true · payments paused"
	}
	return "false · payments live"
}

func formatAny(v any) string {
	if v == nil {
		return "null"
	}
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	}

	if b, err := json.Marshal(v); err == nil {
		return string(b)
	}
	return fmt.Sprint(v)
}

// pageData is the template payload.
type pageData struct {
	Title    string
	Subtitle string
	Banner   string
	Entities []Entity
}

// newRouter builds the Gin engine for the guide, bound to the given Shipeasy
// client. It is split out of main() so tests can construct the router
// in-process (after shipeasy.ConfigureForTesting) and exercise it with
// net/http/httptest.
func newRouter(client *shipeasy.Client) *gin.Engine {
	r := gin.Default()

	tmpl := template.Must(template.ParseFiles("templates/guide.html"))
	r.SetHTMLTemplate(tmpl)

	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "guide.html", pageData{
			Title:    "Shipeasy · Go Entity Guide",
			Subtitle: "One card per Shipeasy entity — feature flags, configs, experiments, kill switches, events, i18n, and error reporting — with the Go SDK configured once at startup.",
			Banner:   "SDK wired at startup with SHIPEASY_SERVER_KEY; the live cards are evaluated through the installed github.com/shipeasy-ai/sdk-go version.",
			Entities: entities(client),
		})
	})

	return r
}

func main() {
	gin.SetMode(gin.ReleaseMode)

	_ = godotenv.Load()
	shipeasy.Configure(shipeasy.Options{APIKey: os.Getenv("SHIPEASY_SERVER_KEY")})
	client := shipeasy.NewClient(shipeasy.User{"user_id": "u_123", "plan": "pro"})

	r := newRouter(client)

	log.Println("Shipeasy Go Entity Guide → http://localhost:8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
