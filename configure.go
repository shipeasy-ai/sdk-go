package shipeasy

import (
	"context"
	"log"
	"sync"
)

// Global front door: Configure once at process start, then build a cheap
// user-bound Client per request with NewClient(user). This is the primary,
// idiomatic way to use the SDK; it mirrors the TS `configure()` + `new
// Client(user)` shape and the same two-part model across every Shipeasy SDK.
//
//	func main() {
//	    shipeasy.Configure(shipeasy.Options{
//	        APIKey: os.Getenv("SHIPEASY_SERVER_KEY"),
//	        // Optional: map YOUR user type to the Shipeasy attribute map.
//	        Attributes: func(u any) shipeasy.User {
//	            acct := u.(*Account)
//	            return shipeasy.User{"user_id": acct.ID, "plan": acct.Plan}
//	        },
//	    })
//	    // ... later, per request:
//	    on := shipeasy.NewClient(acct).GetFlag("new_checkout")
//	}
//
// When no Attributes transform is configured the identity transform is used:
// the value passed to NewClient is assumed to already BE a User (attribute map),
// so NewClient(shipeasy.User{"user_id": "u1", "plan": "pro"}) works as-is.

var (
	configureOnce   sync.Once
	globalEngine    *Engine
	globalAttrs     func(any) User
	globalEngineMu  sync.RWMutex
	identityAttrsFn = func(u any) User {
		switch v := u.(type) {
		case User:
			return v
		case map[string]any:
			return User(v)
		case nil:
			return User{}
		default:
			// Caller passed something that isn't already an attribute map and
			// didn't configure a transform. We can't bucket it, so degrade to an
			// empty map (evaluations behave as for an unidentified user) and warn.
			log.Printf("[shipeasy] NewClient called with a non-map user and no Attributes transform configured — pass a shipeasy.User or configure Options.Attributes")
			return User{}
		}
	}
)

// Configure builds the one process-wide Engine from opts and stores it (plus the
// optional Attributes transform) as the package global. It is first-config-wins
// (idempotent): the FIRST call builds and registers the engine and kicks off a
// fire-and-forget one-shot fetch (InitOnce-equivalent) so a later
// NewClient(user).GetFlag(...) resolves against real rules without an explicit
// init call; subsequent Configure calls are no-ops and return the already-built
// engine. Configure also registers the engine as the default backing
// package-level See() (NewEngine does the same).
//
// Long-running servers that want background polling (not just the one-shot
// fetch) can call Init on the returned engine instead, or use Engine() to fetch
// it later. Returns the global engine.
func Configure(opts Options) *Engine {
	configureOnce.Do(func() {
		attrs := opts.Attributes
		if attrs == nil {
			attrs = identityAttrsFn
		}
		eng := NewEngine(opts)

		globalEngineMu.Lock()
		globalEngine = eng
		globalAttrs = attrs
		globalEngineMu.Unlock()

		// One-shot fetch, fire-and-forget, so bound Clients resolve against real
		// rules without an explicit init. Long-running servers may call
		// eng.Init(ctx) themselves to also start the background poll.
		go func() {
			if err := eng.InitOnce(context.Background()); err != nil {
				log.Printf("[shipeasy] Configure initial fetch failed: %v", err)
			}
		}()
	})
	return resolveGlobalEngine()
}

// ConfiguredEngine returns the process-wide engine built by Configure, or nil if
// Configure has not been called. Useful when you need the engine's full surface
// (Init, Track, OnChange, overrides, see) directly.
func ConfiguredEngine() *Engine { return resolveGlobalEngine() }

func resolveGlobalEngine() *Engine {
	globalEngineMu.RLock()
	defer globalEngineMu.RUnlock()
	return globalEngine
}

func resolveGlobalAttrs() func(any) User {
	globalEngineMu.RLock()
	defer globalEngineMu.RUnlock()
	if globalAttrs == nil {
		return identityAttrsFn
	}
	return globalAttrs
}

// Client is the lightweight, user-bound handle. It carries NO api key, opens NO
// connection, and runs NO poll timer — it delegates every evaluation to the
// single Engine built by Configure, with the bound attribute map. Build one per
// request/user; it is cheap.
//
//	c := shipeasy.NewClient(acct)
//	if c.GetFlag("new_checkout") { ... }
type Client struct {
	// attributes is the resolved Shipeasy attribute map for the bound user,
	// computed once in NewClient via the configured Attributes transform.
	attributes User
	engine     *Engine
}

// NewClient binds a user to the configured Engine. The configured Attributes
// transform is applied to user ONCE here and the resulting attribute map is
// stored; every method then evaluates against it with no user argument.
//
// NewClient panics if Configure has not been called — that is a programmer error
// (the api key lives in the global config), and failing loudly surfaces it
// immediately rather than silently returning false for every flag. The panic
// message is: "shipeasy: NewClient(user) called before Configure({APIKey: ...})".
func NewClient(user any) *Client {
	eng := resolveGlobalEngine()
	if eng == nil {
		panic("shipeasy: NewClient(user) called before Configure({APIKey: ...})")
	}
	attrs := resolveGlobalAttrs()(user)
	if attrs == nil {
		attrs = User{}
	}
	return &Client{attributes: attrs, engine: eng}
}

// GetFlag evaluates a gate for the bound user.
func (c *Client) GetFlag(name string) bool {
	return c.engine.GetFlag(name, c.attributes)
}

// GetFlagOr evaluates a gate for the bound user, returning def only when the
// flag cannot be evaluated (engine not ready, or the gate is absent).
func (c *Client) GetFlagOr(name string, def bool) bool {
	return c.engine.GetFlagOr(name, c.attributes, def)
}

// GetFlagDetail evaluates a gate for the bound user and reports why.
func (c *Client) GetFlagDetail(name string) FlagDetail {
	return c.engine.GetFlagDetail(name, c.attributes)
}

// GetConfig returns a dynamic config value (configs are not user-scoped; this is
// exposed on Client for one-stop ergonomics and forwards to the engine).
func (c *Client) GetConfig(name string) (any, bool) {
	return c.engine.GetConfig(name)
}

// GetConfigOr returns a dynamic config value, or def when the key is absent.
func (c *Client) GetConfigOr(name string, def any) any {
	return c.engine.GetConfigOr(name, def)
}

// GetExperiment evaluates an experiment for the bound user.
func (c *Client) GetExperiment(name string, defaultParams any) ExperimentResult {
	return c.engine.GetExperiment(name, c.attributes, defaultParams)
}

// GetKillswitch reports whether a kill switch is engaged. Kill switches are not
// user-scoped; this forwards to the engine. An optional switchKey selects a
// named per-key override switch (the dashboard "switches" feature).
func (c *Client) GetKillswitch(name string, switchKey ...string) bool {
	key := ""
	if len(switchKey) > 0 {
		key = switchKey[0]
	}
	return c.engine.GetKillswitch(name, key)
}
