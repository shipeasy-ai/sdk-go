package shipeasy

import "sync"

// Doc-23 configure() family + package-level helpers. The documented surface is
// exactly Configure (+ these test/offline siblings) and the bound Client(user);
// the heavyweight Engine stays public but undocumented. These helpers let users
// avoid naming the Engine in tests, overrides, change listeners, and SSR tags.

// ExperimentOverride is the seed shape for a forced experiment enrolment:
// GetExperiment(name) reports {InExperiment: true, Group, Params}.
type ExperimentOverride struct {
	Group  string
	Params any
}

// TestOptions seeds ConfigureForTesting. All fields are optional.
type TestOptions struct {
	// Attributes is the same transform as Configure (default identity).
	Attributes func(any) User
	// Flags are forced GetFlag results: name -> bool.
	Flags map[string]bool
	// Configs are forced GetConfig results: name -> value.
	Configs map[string]any
	// Experiments are forced enrolments: name -> {Group, Params}.
	Experiments map[string]ExperimentOverride
}

// Snapshot is the in-memory source for ConfigureForOffline: the parsed bodies of
// GET /sdk/flags and GET /sdk/experiments. Either may be nil.
type Snapshot struct {
	Flags       any
	Experiments any
}

// OfflineOptions seeds ConfigureForOffline. Provide exactly one source: Snapshot
// (in-memory) or Path (a JSON file {"flags":..., "experiments":...}). The
// Flags/Configs/Experiments overrides are layered on top of the real rules.
type OfflineOptions struct {
	Snapshot    *Snapshot
	Path        string
	Attributes  func(any) User
	Flags       map[string]bool
	Configs     map[string]any
	Experiments map[string]ExperimentOverride
}

// installGlobalEngine REPLACES the package-global engine + attribute transform
// (unlike Configure's first-config-wins, the ConfigureFor* siblings replace so a
// test suite can reconfigure between cases). The previous engine's poll timer is
// stopped, and the configureOnce gate is reset so a later Configure can run.
func installGlobalEngine(eng *Engine, attrs func(any) User) *Engine {
	if attrs == nil {
		attrs = identityAttrsFn
	}
	globalEngineMu.Lock()
	if globalEngine != nil {
		globalEngine.Destroy()
	}
	globalEngine = eng
	globalAttrs = attrs
	configureOnce = sync.Once{}
	globalEngineMu.Unlock()
	return eng
}

func applyOverrides(eng *Engine, flags map[string]bool, configs map[string]any, experiments map[string]ExperimentOverride) {
	for name, v := range flags {
		eng.OverrideFlag(name, v)
	}
	for name, v := range configs {
		eng.OverrideConfig(name, v)
	}
	for name, ov := range experiments {
		eng.OverrideExperiment(name, ov.Group, ov.Params)
	}
}

// ConfigureForTesting configures Shipeasy in test mode — a drop-in sibling of
// Configure with no network, ever (no api key needed). Seed the values your code
// under test should see, then read them through the ordinary NewClient(user):
//
//	shipeasy.ConfigureForTesting(shipeasy.TestOptions{Flags: map[string]bool{"new_checkout": true}})
//	c := shipeasy.NewClient(shipeasy.User{"user_id": "u_1"})
//	c.GetFlag("new_checkout") // true
//
// Replaces any previously-configured engine, so tests can reconfigure freely.
func ConfigureForTesting(opts TestOptions) *Engine {
	eng := NewTestClient()
	applyOverrides(eng, opts.Flags, opts.Configs, opts.Experiments)
	return installGlobalEngine(eng, opts.Attributes)
}

// ConfigureForOffline configures Shipeasy offline — evaluate the REAL rules from
// an in-memory snapshot or a JSON file, with no network. A drop-in sibling of
// Configure (no api key needed). The Flags/Configs/Experiments overrides are
// layered on top. Replaces any previously-configured engine. Returns an error
// only when reading/parsing a Path snapshot fails.
func ConfigureForOffline(opts OfflineOptions) (*Engine, error) {
	var eng *Engine
	switch {
	case opts.Path != "":
		e, err := NewOfflineClient(opts.Path)
		if err != nil {
			return nil, err
		}
		eng = e
	case opts.Snapshot != nil:
		eng = NewOfflineClientFromSnapshot(opts.Snapshot.Flags, opts.Snapshot.Experiments)
	default:
		eng = NewOfflineClientFromSnapshot(nil, nil)
	}
	applyOverrides(eng, opts.Flags, opts.Configs, opts.Experiments)
	return installGlobalEngine(eng, opts.Attributes), nil
}

func requireGlobal(fn string) *Engine {
	eng := resolveGlobalEngine()
	if eng == nil {
		panic("shipeasy: " + fn + " called before Configure({APIKey: ...}) (or a ConfigureFor* sibling)")
	}
	return eng
}

// OverrideFlag forces GetFlag(name) -> value on the spot, for the current
// configuration — a quick in-test override layered on top of whatever
// ConfigureForTesting / ConfigureForOffline (or Configure) set up. Wins over the
// blob until ClearOverrides.
func OverrideFlag(name string, value bool) { requireGlobal("OverrideFlag").OverrideFlag(name, value) }

// OverrideConfig forces GetConfig(name) -> value on the spot (see OverrideFlag).
func OverrideConfig(name string, value any) {
	requireGlobal("OverrideConfig").OverrideConfig(name, value)
}

// OverrideExperiment forces GetExperiment(name) to report enrolment in group
// with params on the spot (see OverrideFlag).
func OverrideExperiment(name, group string, params any) {
	requireGlobal("OverrideExperiment").OverrideExperiment(name, group, params)
}

// ClearOverrides drops every on-the-spot flag/config/experiment override —
// INCLUDING the seed from ConfigureForTesting (test mode has no blob beneath, so
// everything reverts to empty-blob defaults). Under ConfigureForOffline the
// snapshot remains and evaluations revert to it.
func ClearOverrides() { requireGlobal("ClearOverrides").ClearOverrides() }

// OnChange registers a listener fired after a background poll fetches NEW data (a
// 200, not a 304). Returns a cancel func. Requires Configure(Options{Poll: true})
// (no poll runs otherwise). Configuration owns the engine; you never touch it.
func OnChange(fn func()) (cancel func()) { return requireGlobal("OnChange").OnChange(fn) }

// BootstrapScriptTag returns the SSR bootstrap <script> tag for a request (no key
// embedded), delegating to the configured global engine — call Configure first.
func BootstrapScriptTag(user User, opts BootstrapTagOptions) string {
	return requireGlobal("BootstrapScriptTag").BootstrapScriptTag(user, opts)
}

// I18nScriptTag returns the i18n loader <script> tag (public client key) for SSR,
// delegating to the configured global engine — call Configure first.
func I18nScriptTag(clientKey, profile string, opts BootstrapTagOptions) string {
	return requireGlobal("I18nScriptTag").I18nScriptTag(clientKey, profile, opts)
}
