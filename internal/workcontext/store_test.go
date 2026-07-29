package workcontext

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpsertLoadDelete(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	p := Project{
		Name:    "Payments API",
		Repo:    "github.com/acme/payments",
		Period:  "2024 – Present",
		Role:    "Backend Engineer",
		Summary: "Built billing.\n- Cut latency 40%\n- Added GraphQL gateway",
		Source:  "claude",
	}
	if err := Upsert(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".nexus", "work_context.json")); err != nil {
		t.Fatal(err)
	}
	all, err := Load()
	if err != nil || len(all) != 1 {
		t.Fatalf("load: %v len=%d", err, len(all))
	}
	if all[0].Name != "Payments API" || len(all[0].Bullets) < 2 {
		t.Fatalf("got %+v", all[0])
	}
	id := all[0].ID
	p.ID = id
	p.Name = "Payments API v2"
	if err := Upsert(p); err != nil {
		t.Fatal(err)
	}
	all, _ = Load()
	if len(all) != 1 || all[0].Name != "Payments API v2" {
		t.Fatalf("upsert replace failed: %+v", all)
	}
	if err := Delete(id); err != nil {
		t.Fatal(err)
	}
	all, _ = Load()
	if len(all) != 0 {
		t.Fatalf("expected empty, got %d", len(all))
	}
}

func TestExtractBullets(t *testing.T) {
	got := ExtractBullets("Intro\n- One\n* Two\n• Three\nplain")
	if len(got) != 3 {
		t.Fatalf("%v", got)
	}
}
