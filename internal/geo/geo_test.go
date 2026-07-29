package geo

import (
	"strings"
	"testing"
)

func TestLen(t *testing.T) {
	n := Len()
	if n < 1000 {
		t.Fatalf("Len() = %d, want thousands of cities", n)
	}
}

func TestSearchBengaluru(t *testing.T) {
	hits := Search("bengaluru", 8)
	if len(hits) == 0 {
		t.Fatal("expected Bengaluru hits")
	}
	found := false
	for _, c := range hits {
		if strings.EqualFold(c.Name, "Bengaluru") && strings.EqualFold(c.Country, "India") {
			found = true
			if c.Display() != "Bengaluru, India" {
				t.Errorf("Display = %q", c.Display())
			}
		}
	}
	if !found {
		t.Fatalf("Bengaluru, India not in %v", hits)
	}
}

func TestResolveDisplay(t *testing.T) {
	c, ok := Resolve("Bengaluru, India")
	if !ok {
		t.Fatal("Resolve failed")
	}
	if c.Name != "Bengaluru" || c.Country != "India" || c.ISO2 != "IN" {
		t.Fatalf("got %+v", c)
	}
}

func TestExpandLocations(t *testing.T) {
	got := ExpandLocations([]string{"Bengaluru, India", "Berlin, Germany"})
	want := map[string]bool{"Bengaluru": true, "India": true, "Berlin": true, "Germany": true}
	for _, need := range []string{"Bengaluru", "India", "Berlin", "Germany"} {
		found := false
		for _, term := range got {
			if term == need {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing %q in %v", need, got)
		}
	}
	_ = want
}

func TestExpandLocationsLegacyPassthrough(t *testing.T) {
	got := ExpandLocations([]string{"Remote-ish custom"})
	if len(got) != 1 || got[0] != "Remote-ish custom" {
		t.Fatalf("got %v", got)
	}
}

func TestParseJoinLocationTags(t *testing.T) {
	tags := []string{"Bengaluru, India", "Berlin, Germany"}
	joined := JoinLocationTags(tags)
	if joined != "Bengaluru, India; Berlin, Germany" {
		t.Fatalf("join = %q", joined)
	}
	got := ParseLocationTags(joined)
	if len(got) != 2 || got[0] != "Bengaluru, India" || got[1] != "Berlin, Germany" {
		t.Fatalf("parse = %#v", got)
	}
	legacy := ParseLocationTags("San Francisco, New York")
	if len(legacy) != 2 || legacy[0] != "San Francisco" || legacy[1] != "New York" {
		t.Fatalf("legacy = %#v", legacy)
	}
}

func TestCountriesFromTags(t *testing.T) {
	got := CountriesFromTags([]string{"Bengaluru, India", "Berlin, Germany", "IN"})
	want := map[string]bool{"IN": true, "DE": true}
	for _, g := range got {
		if !want[g] {
			t.Fatalf("unexpected %q in %v", g, got)
		}
		delete(want, g)
	}
	if len(want) != 0 {
		t.Fatalf("missing %v from %v", want, got)
	}
	if CountriesFromTags([]string{"India"})[0] != "IN" {
		t.Fatalf("India alone → IN, got %v", CountriesFromTags([]string{"India"}))
	}
}
