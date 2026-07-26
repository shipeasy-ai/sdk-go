package shipeasy

import (
	"encoding/json"
	"html"
	"strings"
)

// defaultCDNBase is the CDN origin that serves the static loader scripts
// (/sdk/runtime.js, /sdk/i18n/loader.js). Distinct from defaultBaseURL, which
// is the edge API the client fetches flag/experiment blobs from.
const defaultCDNBase = "https://cdn.shipeasy.ai"

// Bootstrap is the SSR-evaluated payload for one request: every loaded gate,
// config and experiment evaluated for the user, ready to ride the
// SSR bootstrap <script> tag's data-* attributes. Killswitches are folded
// into per-gate evaluation (a killed gate reads false in Flags), so the
// standalone Killswitches map is empty for this SDK.
type Bootstrap struct {
	Flags        map[string]bool         `json:"flags"`
	Configs      map[string]any          `json:"configs"`
	Experiments  map[string]BootstrapExp `json:"experiments"`
	Killswitches map[string]any          `json:"killswitches"`
	// Universes carries per-universe param defaults so the browser can resolve
	// universe(name).get(field) to a default even when the unit is not enrolled
	// anywhere in the universe. Only universes referenced by a loaded experiment
	// appear.
	Universes map[string]BootstrapUniverse `json:"universes"`
}

// BootstrapExp is one experiment's assignment, keyed to match the browser SDK's
// window.__SE_BOOTSTRAP shape. Universe is the experiment's universe name so the
// client can resolve universe(name).assign() by finding the enrolled experiment.
// Params (when enrolled) is ALREADY merged (universeDefaults ⊕ variant).
type BootstrapExp struct {
	InExperiment bool   `json:"inExperiment"`
	Group        string `json:"group"`
	Params       any    `json:"params"`
	Universe     string `json:"universe"`
}

// BootstrapUniverse is one universe's SSR handoff: the flattened param defaults.
type BootstrapUniverse struct {
	Defaults map[string]any `json:"defaults"`
}

// TagOptions tunes the emitted <script> tags. EVERY field is optional: an unset
// field falls back to the matching Options value passed to Configure, so a
// template can call the tag helpers with no options at all.
type TagOptions struct {
	// AnonID is the stable anonymous bucketing id the server evaluated against.
	// Emitted as data-anon-id; the runtime writes it to the __se_anon_id
	// cookie and window.__SE_BOOTSTRAP so the browser buckets identically to SSR.
	// Bootstrap tag only.
	AnonID string
	// I18nProfile recorded on the tag (defaults to Options.Profile, else
	// "en:prod"). Read by the i18n and bootstrap tags.
	I18nProfile string
	// ClientKey is the PUBLIC client key put on the i18n / devtools tags
	// (defaults to Options.ClientKey). NEVER the server key.
	ClientKey string
	// ProjectID for the devtools tag (defaults to Options.ProjectID).
	ProjectID string
	// BaseURL overrides the CDN base for the tag src + data-api-url
	// (defaults to Options.CDNBaseURL, else https://cdn.shipeasy.ai).
	BaseURL string
	// NoDefer drops the `defer` attribute from the devtools tag. The overlay is
	// deferred by default — a developer tool is never needed for first paint.
	NoDefer bool
}

// BootstrapTagOptions is the former name of TagOptions, kept as an alias so
// existing call sites keep compiling.
type BootstrapTagOptions = TagOptions

// firstTagOptions collapses the variadic options every tag helper takes: callers
// pass zero options (everything from Configure) or exactly one.
func firstTagOptions(opts []TagOptions) TagOptions {
	if len(opts) == 0 {
		return TagOptions{}
	}
	return opts[0]
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
		Universes:    map[string]BootstrapUniverse{},
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
			e := exps.Experiments[name]
			uniName := e.Universe
			// Per-universe param defaults so the client can resolve
			// universe(name).get() even when the unit is not enrolled anywhere.
			if _, seen := b.Universes[uniName]; !seen {
				var defaults map[string]any
				if uni, ok := exps.Universes[uniName]; ok {
					defaults = paramDefaultsFromSchema(uni.ParamSchema)
				}
				if defaults == nil {
					defaults = map[string]any{}
				}
				b.Universes[uniName] = BootstrapUniverse{Defaults: defaults}
			}
			var ov *ExperimentResult
			if r, ok := expOv[name]; ok {
				ov = &r
			}
			st := classifyOne(name, &e, flags, exps, user, sticky, ov)
			if st.State == stateGroup {
				params := st.Params
				if params == nil {
					params = map[string]any{}
				}
				b.Experiments[name] = BootstrapExp{InExperiment: true, Group: st.Group, Params: params, Universe: uniName}
			} else {
				b.Experiments[name] = BootstrapExp{InExperiment: false, Group: "control", Params: map[string]any{}, Universe: uniName}
			}
		}
	}
	return b
}

// BootstrapScriptTag returns the cross-platform SSR bootstrap <script> tag for a
// request: /sdk/runtime.js reads its data-* attributes, installs
// window.shipeasy, republishes window.__SE_BOOTSTRAP for the npm client SDK and
// writes the anon cookie. No SDK key is embedded — the server key must never
// reach the browser.
//
// This used to point at /sdk/bootstrap.js, which did nothing the runtime does
// not; both marker attributes are emitted so the npm client SDK still finds the
// tag by data-se-bootstrap while the runtime finds itself by data-se-boot.
// Every argument is OPTIONAL: a nil user renders an anonymous request, and each
// unset TagOptions field falls back to what Configure was given.
func (c *Engine) BootstrapScriptTag(user User, opts ...TagOptions) string {
	o := firstTagOptions(opts)
	b := c.Evaluate(user)
	base := c.cdnBaseFor(o.BaseURL)
	profile := c.profileFor(o.I18nProfile)
	attrs := []string{
		"data-se-bootstrap",
		"data-se-boot",
		attr("data-flags", jsonStr(b.Flags)),
		attr("data-configs", jsonStr(b.Configs)),
		attr("data-experiments", jsonStr(b.Experiments)),
		attr("data-killswitches", jsonStr(b.Killswitches)),
		attr("data-i18n-profile", profile),
		attr("data-api-url", base),
	}
	if o.AnonID != "" {
		attrs = append(attrs, attr("data-anon-id", o.AnonID))
	}
	// Carry the server-identified user so the browser SDK adopts the same
	// identity on first paint (no anon→identified flip). anonymous_id is
	// dropped — it already rides data-anon-id.
	if du, ok := identityAttrs(user); ok {
		attrs = append(attrs, attr("data-user", du))
	}
	return `<script src="` + html.EscapeString(base+"/sdk/runtime.js") + `" ` +
		strings.Join(attrs, " ") + `></script>`
}

// I18nScriptTag returns the i18n loader <script> tag. The loader fetches and
// installs translations for the profile using the PUBLIC client key (safe to
// embed in HTML). Pair it with BootstrapScriptTag in your document head.
//
// Every argument is OPTIONAL: with no options the tag carries the ClientKey,
// Profile and CDNBaseURL passed to Configure.
func (c *Engine) I18nScriptTag(opts ...TagOptions) string {
	o := firstTagOptions(opts)
	base := c.cdnBaseFor(o.BaseURL)
	key := c.clientKeyFor(o.ClientKey)
	c.warnMissingTagSetting("I18nScriptTag", "ClientKey", key)
	return `<script src="` + html.EscapeString(base+"/sdk/i18n/loader.js") + `" ` +
		attr("data-key", key) + ` ` + attr("data-profile", c.profileFor(o.I18nProfile)) + `></script>`
}

// DevtoolsScriptTag returns the devtools overlay <script> tag. se-devtools.js is
// a hosted, self-executing bundle — nothing to install — that reads the project
// and the PUBLIC client key off the tag. The overlay opens with Shift+Alt+S or
// on any page loaded with ?se=1.
//
// Every argument is OPTIONAL: with no options the tag carries the ProjectID,
// ClientKey and CDNBaseURL passed to Configure. The tag is deferred unless
// TagOptions.NoDefer is set — a developer tool never belongs on the critical
// rendering path.
func (c *Engine) DevtoolsScriptTag(opts ...TagOptions) string {
	o := firstTagOptions(opts)
	base := c.cdnBaseFor(o.BaseURL)
	pid := o.ProjectID
	if pid == "" {
		pid = c.projectID
	}
	key := c.clientKeyFor(o.ClientKey)
	c.warnMissingTagSetting("DevtoolsScriptTag", "ProjectID", pid)
	c.warnMissingTagSetting("DevtoolsScriptTag", "ClientKey", key)
	attrs := attr("data-project-id", pid) + ` ` + attr("data-client-api-key", key)
	if !o.NoDefer {
		attrs += ` defer`
	}
	return `<script src="` + html.EscapeString(base+"/se-devtools.js") + `" ` +
		attrs + `></script>`
}

// cdnBaseFor resolves the CDN origin for a tag: the per-call override, else the
// configured CDNBaseURL, else the platform default.
func (c *Engine) cdnBaseFor(override string) string {
	if override == "" {
		override = c.cdnBaseURL
	}
	return cdnBase(override)
}

// profileFor resolves the i18n profile a tag carries: the per-call override,
// else the configured Profile, else the platform default.
func (c *Engine) profileFor(override string) string {
	if override != "" {
		return override
	}
	if c.profile != "" {
		return c.profile
	}
	return "en:prod"
}

// clientKeyFor resolves the PUBLIC client key a tag carries.
func (c *Engine) clientKeyFor(override string) string {
	if override != "" {
		return override
	}
	return c.clientKey
}

// warnMissingTagSetting logs once per (helper, setting) when a tag is built
// without a key/id. It is not an error — the tag renders, and the browser bundle
// reports what it needs — but it is never what the caller wanted, and a helper
// that runs on every render must not log a line per request.
func (c *Engine) warnMissingTagSetting(fnName, setting, value string) {
	if value != "" {
		return
	}
	seen := fnName + "." + setting
	c.exposureMu.Lock()
	if c.warnedTagSettings == nil {
		c.warnedTagSettings = map[string]struct{}{}
	}
	if _, done := c.warnedTagSettings[seen]; done {
		c.exposureMu.Unlock()
		return
	}
	c.warnedTagSettings[seen] = struct{}{}
	c.exposureMu.Unlock()
	c.logf(LogLevelWarn, "%s: no %s — pass it in TagOptions, or set Options.%s in Configure; the tag will render without it", fnName, setting, setting)
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

// identityAttrs returns the JSON object of the user's identity traits for the
// data-user attribute, dropping anonymous_id (it rides data-anon-id) and any
// nil values. The bool is false when nothing is left after filtering (an
// anonymous or empty user) so no PII rides the tag. Go's json.Marshal sorts
// map keys, so the output is deterministic.
func identityAttrs(user User) (string, bool) {
	if len(user) == 0 {
		return "", false
	}
	traits := make(map[string]any, len(user))
	for k, v := range user {
		if k == "anonymous_id" || v == nil {
			continue
		}
		traits[k] = v
	}
	if len(traits) == 0 {
		return "", false
	}
	b, err := json.Marshal(traits)
	if err != nil {
		return "", false
	}
	return string(b), true
}

func jsonStr(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
