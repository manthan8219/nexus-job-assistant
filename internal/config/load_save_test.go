package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFromMissingPathReturnsEmpty(t *testing.T) {
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadFrom must return a non-nil config for a missing file")
	}
}

func TestLoadFromParsesJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"email":"ada@example.com","work_type":"Remote"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Email != "ada@example.com" || cfg.WorkType != "Remote" {
		t.Errorf("cfg = %+v, want parsed email/work_type", cfg)
	}
}

func TestLoadFromInvalidJSONFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{invalid`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFrom(path); err == nil {
		t.Fatal("LoadFrom must fail on invalid JSON")
	}
}

// Save writes into $HOME/.nexus/config.json — pointing HOME at a temp dir
// keeps the test hermetic.
func TestSaveWritesConfigToHomeDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &Config{Email: "ada@example.com", ApplyConsent: true}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(data), "ada@example.com") {
		t.Errorf("saved config missing email: %s", data)
	}
}
