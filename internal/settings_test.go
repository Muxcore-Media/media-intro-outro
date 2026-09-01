package internal

import (
	"context"
	"testing"
)

func TestSettingsUpdateAndPersist(t *testing.T) {
	dir := t.TempDir()
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	m.store = NewStore(dir)
	if err := m.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := m.UpdateSetting("heuristic_enabled", "false"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("min_confidence", "0.8"); err != nil {
		t.Fatal(err)
	}

	defs := m.Settings()
	keys := map[string]string{}
	for _, d := range defs {
		keys[d.Key] = d.Value
	}
	if keys["heuristic_enabled"] != "false" {
		t.Fatalf("heuristic_enabled=%q", keys["heuristic_enabled"])
	}
	if keys["min_confidence"] != "0.8" {
		t.Fatalf("min_confidence=%q", keys["min_confidence"])
	}

	reloaded := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	reloaded.store = NewStore(dir)
	if err := reloaded.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	reloaded.cfgMu.RLock()
	heuristic := reloaded.heuristicEnabled
	minConf := reloaded.minConfidence
	reloaded.cfgMu.RUnlock()
	if heuristic {
		t.Fatal("expected heuristic_enabled false after reload")
	}
	if minConf != 0.8 {
		t.Fatalf("min_confidence after reload: %g", minConf)
	}
}

func TestUpdateSettingValidation(t *testing.T) {
	m := NewModule(Config{})
	if err := m.UpdateSetting("intro_max_seconds", "-1"); err == nil {
		t.Fatal("expected error for negative intro_max_seconds")
	}
	if err := m.UpdateSetting("min_confidence", "2"); err == nil {
		t.Fatal("expected error for min_confidence > 1")
	}
	if err := m.UpdateSetting("unknown_key", "1"); err == nil {
		t.Fatal("expected error for unknown key")
	}
}
