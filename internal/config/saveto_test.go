package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSaveTo verifies the per-island config writer: it creates parent
// directories, writes 0600 JSON, and round-trips through LoadFrom.
func TestSaveTo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "one", "config.json")
	cfg := &Config{FirstName: "Ada", Email: "ada@example.com", DailyRunAt: "09:00"}

	if err := SaveTo(cfg, path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat saved config: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v; want 0600", info.Mode().Perm())
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if loaded.FirstName != "Ada" || loaded.Email != "ada@example.com" || loaded.DailyRunAt != "09:00" {
		t.Errorf("roundtrip = %+v; want Ada/ada@example.com/09:00", loaded)
	}
}
