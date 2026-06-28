package shipeasy

import "testing"

// resetGlobalForTest clears the package-global engine + the configureOnce gate so
// each test starts clean (the ConfigureFor* siblings replace, but Configure is
// first-wins via the Once).
func resetGlobalForTest() {
	globalEngineMu.Lock()
	if globalEngine != nil {
		globalEngine.Destroy()
	}
	globalEngine = nil
	globalAttrs = nil
	globalEngineMu.Unlock()
}

func TestConfigureForTestingSeedsAndReplaces(t *testing.T) {
	defer resetGlobalForTest()

	ConfigureForTesting(TestOptions{
		Flags:       map[string]bool{"new_checkout": true},
		Configs:     map[string]any{"theme": "blue"},
		Experiments: map[string]ExperimentOverride{"price_test": {Group: "treatment", Params: map[string]any{"price": 9}}},
	})
	c := NewClient(User{"user_id": "u_1"})
	if !c.GetFlag("new_checkout") {
		t.Fatal("seeded flag should be true")
	}
	if v, _ := c.GetConfig("theme"); v != "blue" {
		t.Fatalf("seeded config = %v, want blue", v)
	}
	exp := c.GetExperiment("price_test", nil)
	if !exp.InExperiment || exp.Group != "treatment" {
		t.Fatalf("seeded experiment = %+v", exp)
	}

	// REPLACE (not first-wins): a second ConfigureForTesting wins.
	ConfigureForTesting(TestOptions{Flags: map[string]bool{"new_checkout": false}})
	if NewClient(User{}).GetFlag("new_checkout") {
		t.Fatal("reconfigured flag should be false")
	}
}

func TestPackageOverridesAndClear(t *testing.T) {
	defer resetGlobalForTest()
	ConfigureForTesting(TestOptions{Flags: map[string]bool{"f": true}})
	OverrideFlag("f", false)
	OverrideConfig("c", 123)
	OverrideExperiment("e", "B", map[string]any{"v": 2})
	c := NewClient(User{"user_id": "u"})
	if c.GetFlag("f") {
		t.Fatal("override should win")
	}
	if v, _ := c.GetConfig("c"); v != 123 {
		t.Fatalf("config override = %v", v)
	}
	if c.GetExperiment("e", nil).Group != "B" {
		t.Fatal("experiment override group")
	}
	// Test mode has no blob beneath, so ClearOverrides drops the seed too.
	ClearOverrides()
	if _, ok := NewClient(User{}).GetConfig("c"); ok {
		t.Fatal("config override should be cleared")
	}
}

func TestPackageOverrideBeforeConfigurePanics(t *testing.T) {
	defer resetGlobalForTest()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic before Configure")
		}
	}()
	OverrideFlag("f", true)
}

func TestConfigureForOfflineLayersOverrides(t *testing.T) {
	defer resetGlobalForTest()
	flags := map[string]any{
		"version": "snap", "plan": "free",
		"gates": map[string]any{
			"on_for_all": map[string]any{"rules": []any{}, "rolloutPct": 10000, "salt": "s", "enabled": 1},
		},
		"configs":      map[string]any{"color": map[string]any{"value": "green"}},
		"killswitches": map[string]any{},
	}
	exps := map[string]any{"version": "snap", "experiments": map[string]any{}, "universes": map[string]any{}}

	if _, err := ConfigureForOffline(OfflineOptions{Snapshot: &Snapshot{Flags: flags, Experiments: exps}}); err != nil {
		t.Fatal(err)
	}
	if !NewClient(User{"user_id": "u_1"}).GetFlag("on_for_all") {
		t.Fatal("snapshot gate should evaluate true")
	}
	if v, _ := NewClient(User{}).GetConfig("color"); v != "green" {
		t.Fatalf("snapshot config = %v", v)
	}

	// Layer an override, then clear it back to the snapshot.
	OverrideFlag("on_for_all", false)
	if NewClient(User{}).GetFlag("on_for_all") {
		t.Fatal("override should win over snapshot")
	}
	ClearOverrides()
	if !NewClient(User{}).GetFlag("on_for_all") {
		t.Fatal("clear should revert to snapshot (true)")
	}
}
