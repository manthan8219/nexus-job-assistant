package engine

import (
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

func TestCountriesFromConfig(t *testing.T) {
	cfg := &config.Config{TargetLocations: "Bengaluru, India; Berlin, Germany"}
	got := countriesFromConfig(cfg)
	want := map[string]bool{"IN": true, "DE": true}
	for _, g := range got {
		if !want[g] {
			t.Fatalf("unexpected %q in %v", g, got)
		}
		delete(want, g)
	}
	if len(want) != 0 {
		t.Fatalf("missing %v; got %v", want, got)
	}

	cfg2 := &config.Config{TargetLocations: "India"}
	if g := countriesFromConfig(cfg2); len(g) != 1 || g[0] != "IN" {
		t.Fatalf("India alone → [IN], got %v", g)
	}
}
