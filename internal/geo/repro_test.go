package geo

import (
	"strings"
	"testing"
)

func displays(cs []City) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Display()
	}
	return out
}

func TestSearchPartialAliasBangl(t *testing.T) {
	hits := Search("Bangl", 8)
	if len(hits) == 0 {
		t.Fatal("no hits for Bangl")
	}
	if hits[0].Display() != "Bengaluru, India" {
		t.Fatalf("first hit = %q, want Bengaluru, India; all=%v", hits[0].Display(), displays(hits))
	}
}

func TestSearchPartialAliasGurga(t *testing.T) {
	hits := Search("gurga", 8)
	if len(hits) == 0 {
		t.Fatal("no hits for gurga")
	}
	if hits[0].Display() != "Gurugram, India" {
		t.Fatalf("first hit = %q, want Gurugram, India; all=%v", hits[0].Display(), displays(hits))
	}
}

func TestSearchBangaloreFull(t *testing.T) {
	hits := Search("bangalore", 8)
	if len(hits) == 0 || hits[0].Display() != "Bengaluru, India" {
		t.Fatalf("got %v", displays(hits))
	}
}

func TestSearchDoesNotFloodBangladeshOnBangl(t *testing.T) {
	hits := Search("bangl", 8)
	if hits[0].Display() != "Bengaluru, India" {
		t.Fatalf("want Bengaluru first, got %v", displays(hits))
	}
	for _, c := range hits {
		if strings.EqualFold(c.Country, "Bangladesh") {
			t.Fatalf("unexpected Bangladesh city in %v", displays(hits))
		}
	}
}
