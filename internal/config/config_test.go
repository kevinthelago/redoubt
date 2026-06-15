package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kevinthelago/redoubt/internal/config"
)

func TestLoadFrom_MissingFile_ReturnsDefault(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	def := config.DefaultConfig()
	if cfg.Schedule.Cron != def.Schedule.Cron {
		t.Errorf("schedule.cron = %q, want %q", cfg.Schedule.Cron, def.Schedule.Cron)
	}
	if cfg.Retention.Daily != def.Retention.Daily {
		t.Errorf("retention.daily = %d, want %d", cfg.Retention.Daily, def.Retention.Daily)
	}
}

func TestLoadFrom_CorruptFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("not { valid toml [[["), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := config.LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for corrupt TOML, got nil")
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	original := config.DefaultConfig()
	original.Vault.URL = "rest:https://vault.lan:8000"
	original.Vault.CACert = "/etc/redoubt/ca.pem"
	original.Schedule.Cron = "30 2 * * *"
	original.Retention.Daily = 14

	ts := config.TrackedSet{
		Name:     "dev",
		Paths:    []string{"/home/user/code", "/home/user/notes"},
		Tags:     []string{"dev"},
		Excludes: []string{"*.tmp"},
	}
	if err := original.AddTrackedSet(ts); err != nil {
		t.Fatal(err)
	}

	if err := config.SaveTo(original, path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if loaded.Vault.URL != original.Vault.URL {
		t.Errorf("vault.url = %q, want %q", loaded.Vault.URL, original.Vault.URL)
	}
	if loaded.Vault.CACert != original.Vault.CACert {
		t.Errorf("vault.ca_cert = %q, want %q", loaded.Vault.CACert, original.Vault.CACert)
	}
	if loaded.Schedule.Cron != original.Schedule.Cron {
		t.Errorf("schedule.cron = %q, want %q", loaded.Schedule.Cron, original.Schedule.Cron)
	}
	if loaded.Retention.Daily != original.Retention.Daily {
		t.Errorf("retention.daily = %d, want %d", loaded.Retention.Daily, original.Retention.Daily)
	}
	if len(loaded.TrackedSets) != 1 {
		t.Fatalf("len(TrackedSets) = %d, want 1", len(loaded.TrackedSets))
	}
	if loaded.TrackedSets[0].Name != "dev" {
		t.Errorf("tracked_set[0].name = %q, want %q", loaded.TrackedSets[0].Name, "dev")
	}
	if len(loaded.TrackedSets[0].Paths) != 2 {
		t.Errorf("tracked_set[0].paths len = %d, want 2", len(loaded.TrackedSets[0].Paths))
	}
}

func TestSave_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "config.toml")
	if err := config.SaveTo(config.DefaultConfig(), path); err != nil {
		t.Fatalf("SaveTo nested path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestTrackedSet_AddDuplicate(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := config.TrackedSet{Name: "docs", Paths: []string{"/docs"}}
	if err := cfg.AddTrackedSet(ts); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := cfg.AddTrackedSet(ts); err == nil {
		t.Error("expected error on duplicate add, got nil")
	}
}

func TestTrackedSet_RemoveAndGet(t *testing.T) {
	cfg := config.DefaultConfig()
	_ = cfg.AddTrackedSet(config.TrackedSet{Name: "a", Paths: []string{"/a"}})
	_ = cfg.AddTrackedSet(config.TrackedSet{Name: "b", Paths: []string{"/b"}})

	if _, ok := cfg.GetTrackedSet("a"); !ok {
		t.Error("GetTrackedSet('a') not found")
	}
	if removed := cfg.RemoveTrackedSet("a"); !removed {
		t.Error("RemoveTrackedSet('a') = false, want true")
	}
	if _, ok := cfg.GetTrackedSet("a"); ok {
		t.Error("GetTrackedSet('a') found after remove")
	}
	if removed := cfg.RemoveTrackedSet("nonexistent"); removed {
		t.Error("RemoveTrackedSet('nonexistent') = true, want false")
	}
	if len(cfg.TrackedSets) != 1 {
		t.Errorf("len(TrackedSets) = %d, want 1", len(cfg.TrackedSets))
	}
}

func TestConfigPath_ReturnsNonEmpty(t *testing.T) {
	p := config.ConfigPath()
	if p == "" {
		t.Error("ConfigPath() returned empty string")
	}
}

func TestDataDir_ReturnsNonEmpty(t *testing.T) {
	d := config.DataDir()
	if d == "" {
		t.Error("DataDir() returned empty string")
	}
}
