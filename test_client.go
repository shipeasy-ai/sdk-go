package shipeasy

import "time"

// Local-override test utility (Statsig-style). Lets tests construct a client
// that does ZERO network and seed every entity by hand, so unit tests never
// depend on a live edge or a real API key.
//
//	c := shipeasy.NewTestClient()
//	c.OverrideFlag("new_checkout", true)
//	c.OverrideConfig("billing_copy", map[string]any{"cta": "Buy now"})
//	c.OverrideExperiment("checkout_button", "treatment", map[string]any{"color": "green"})
//
// The override setters also work on a normal client built with NewClient — an
// override always wins over fetched data in GetFlag/GetConfig/GetExperiment.

// NewTestClient returns a no-network, immediately-usable client: telemetry is
// disabled, Init/InitOnce are no-ops (they never fetch), Track is a no-op, and
// no API key is required. Seed behavior with the Override* setters.
func NewTestClient() *Client {
	return &Client{
		baseURL:      defaultBaseURL,
		pollInterval: 30 * time.Second,
		stop:         make(chan struct{}),
		telemetry:    newTelemetry("", "", "server", "prod", true, nil),
		localMode:    true,
		initialized:  true,
	}
}

// OverrideFlag forces GetFlag(name, _) to return value regardless of the
// fetched gate definition or the user passed in.
func (c *Client) OverrideFlag(name string, value bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.flagOverrides == nil {
		c.flagOverrides = map[string]bool{}
	}
	c.flagOverrides[name] = value
}

// OverrideConfig forces GetConfig(name) to return (value, true) regardless of
// the fetched config.
func (c *Client) OverrideConfig(name string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.configOverrides == nil {
		c.configOverrides = map[string]any{}
	}
	c.configOverrides[name] = value
}

// OverrideExperiment forces GetExperiment(name, _, _) to return
// ExperimentResult{InExperiment: true, Group: group, Params: params}. If params
// is nil the call's defaultParams still applies, matching normal behavior.
func (c *Client) OverrideExperiment(name string, group string, params any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.expOverrides == nil {
		c.expOverrides = map[string]ExperimentResult{}
	}
	c.expOverrides[name] = ExperimentResult{InExperiment: true, Group: group, Params: params}
}

// ClearOverrides removes every flag, config, and experiment override.
func (c *Client) ClearOverrides() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flagOverrides = nil
	c.configOverrides = nil
	c.expOverrides = nil
}
