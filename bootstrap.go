package shipeasy

import (
	"encoding/json"
	"html"
	"strings"
)

// defaultCDNBase is the CDN origin that serves the static loader scripts
// (/sdk/bootstrap.js, /sdk/i18n/loader.js). Distinct from defaultBaseURL, which
// is the edge API the client fetches flag/experiment blobs from.
const defaultCDNBase = "https://cdn.shipeasy.ai"

// Bootstrap is the SSR-evaluated payload for one request: every loaded gate,
// config and experiment evaluated for the user, ready to ride the
// se-bootstrap.js <script> tag's data-* attributes. Killswitches are folded
// into per-gate evaluation (a killed gate reads false in Flags), so the
// standalone Killswitches map is empty for this SDK.
type Bootstrap struct {
	Flags        map[string]bool         `json:"flags"`
	Configs      map[string]any          `json:"configs"`
	Experiments  map[string]BootstrapExp `json:"experiments"`
	Killswitches map[string]any          `json:"killswitches"`
}

// BootstrapExp is one experiment's assignment, keyed to match the browser SDK's
// window.__SE_BOOTSTRAP shape.
type BootstrapExp struct {
	InExperiment bool   `json:"inExperiment"`
	Group        string `json:"group"`
	Params       any    `json:"params"`
}

// BootstrapTagOptions tunes the emitted <script> tags.
type BootstrapTagOptions struct {
	// AnonID is the stable anonymous bucketing id the server evaluated against.
	// Emitted as data-anon-id; se-bootstrap.js writes it to the __se_anon_id
	// cookie and window.__SE_BOOTSTRAP so the browser buckets identically to SSR.
	AnonID string
	// I18nProfile recorded on the tag (defaults to "en:prod").
	I18nProfile string
	// BaseURL overrides the CDN base for the tag src + data-api-url
	// (defaults to https://cdn.shipeasy.ai).
	BaseURL string
}

// Evaluate builds the bootstrap payload for a user by evaluating every loaded
// gate, config and experiment. Local overrides (OverrideFlag/Config/Experiment)
// win, matching the per-key getters. No telemetry is emitted (a batch evaluate
// is not a per-flag exposure).
func (c *Engine) Evaluate(user User) Bootstrap {
	c.mu.RLock()
	flags := c.flags
	exps := c.exps
	sticky := c.stickyStore
	flagOv := make(map[string]bool, len(c.flagOverrides))
	for k, v := range c.flagOverrides {
		flagOv[k] = v
	}
	configOv := make(map[string]any, len(c.configOverrides))
	for k, v := range c.configOverrides {
		configOv[k] = v
	}
	expOv := make(map[string]ExperimentResult, len(c.expOverrides))
	for k, v := range c.expOverrides {
		expOv[k] = v
	}
	c.mu.RUnlock()

	b := Bootstrap{
		Flags:        map[string]bool{},
		Configs:      map[string]any{},
		Experiments:  map[string]BootstrapExp{},
		Killswitches: map[string]any{},
	}
	if flags != nil {
		for name, g := range flags.Gates {
			if v, ok := flagOv[name]; ok {
				b.Flags[name] = v
				continue
			}
			b.Flags[name] = evalGate(g, user)
		}
		for name, cfg := range flags.Configs {
			if v, ok := configOv[name]; ok {
				b.Configs[name] = v
				continue
			}
			b.Configs[name] = cfg.Value
		}
	}
	if exps != nil {
		for name := range exps.Experiments {
			if ov, ok := expOv[name]; ok {
				b.Experiments[name] = BootstrapExp{ov.InExperiment, ov.Group, ov.Params}
				continue
			}
			e := exps.Experiments[name]
			r := evalExperiment(name, &e, flags, exps, user, sticky)
			b.Experiments[name] = BootstrapExp{r.InExperiment, r.Group, r.Params}
		}
	}
	return b
}

// BootstrapScriptTag returns the cross-platform SSR bootstrap <script> tag for a
// request: se-bootstrap.js reads its data-* attributes and hydrates
// window.__SE_BOOTSTRAP (and writes the anon cookie). No SDK key is embedded —
// the server key must never reach the browser.
func (c *Engine) BootstrapScriptTag(user User, opts BootstrapTagOptions) string {
	b := c.Evaluate(user)
	base := cdnBase(opts.BaseURL)
	profile := opts.I18nProfile
	if profile == "" {
		profile = "en:prod"
	}
	attrs := []string{
		"data-se-bootstrap",
		attr("data-flags", jsonStr(b.Flags)),
		attr("data-configs", jsonStr(b.Configs)),
		attr("data-experiments", jsonStr(b.Experiments)),
		attr("data-killswitches", jsonStr(b.Killswitches)),
		attr("data-i18n-profile", profile),
		attr("data-api-url", base),
	}
	if opts.AnonID != "" {
		attrs = append(attrs, attr("data-anon-id", opts.AnonID))
	}
	return `<script src="` + html.EscapeString(base+"/sdk/bootstrap.js") + `" ` +
		strings.Join(attrs, " ") + `></script>`
}

// I18nScriptTag returns the i18n loader <script> tag. The loader fetches and
// installs translations for the profile using the PUBLIC client key (safe to
// embed in HTML). Pair it with BootstrapScriptTag in your document head.
func (c *Engine) I18nScriptTag(clientKey, profile string, opts BootstrapTagOptions) string {
	base := cdnBase(opts.BaseURL)
	if profile == "" {
		profile = "en:prod"
	}
	return `<script src="` + html.EscapeString(base+"/sdk/i18n/loader.js") + `" ` +
		attr("data-key", clientKey) + ` ` + attr("data-profile", profile) + `></script>`
}

func cdnBase(override string) string {
	base := override
	if base == "" {
		base = defaultCDNBase
	}
	return strings.TrimRight(base, "/")
}

func attr(name, val string) string {
	return name + `="` + html.EscapeString(val) + `"`
}

func jsonStr(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
