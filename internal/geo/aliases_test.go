package geo

import (
	"strings"
	"testing"
)

func TestAliasResolveIndianITHubs(t *testing.T) {
	cases := map[string]string{
		"bangalore":  "Bengaluru, India",
		"Bangalore":  "Bengaluru, India",
		"gurgaon":    "Gurugram, India",
		"bombay":     "Mumbai, India",
		"calcutta":   "Kolkata, India",
		"madras":     "Chennai, India",
		"poona":      "Pune, India",
		"trivandrum": "Thiruvananthapuram, India",
		"kochi":      "Cochin, India",
		"mysore":     "Mysuru, India",
		"mangalore":  "Mangaluru, India",
		"hubli":      "Hubballi, India",
		"vizag":      "Visakhapatnam, India",
		"pondicherry": "Puducherry, India",
		"baroda":     "Vadodara, India",
	}
	for in, want := range cases {
		c, ok := Resolve(in)
		if !ok {
			t.Errorf("Resolve(%q) failed", in)
			continue
		}
		if c.Display() != want {
			t.Errorf("Resolve(%q) = %q, want %q", in, c.Display(), want)
		}
	}
}

func TestAliasSearchBangalore(t *testing.T) {
	hits := Search("bangalore", 8)
	if len(hits) == 0 {
		t.Fatal("Search(bangalore) returned no hits")
	}
	if hits[0].Display() != "Bengaluru, India" {
		t.Fatalf("first hit = %q, want Bengaluru, India", hits[0].Display())
	}
}

func TestAliasSearchGurgaon(t *testing.T) {
	hits := Search("gurgaon", 8)
	if len(hits) == 0 {
		t.Fatal("Search(gurgaon) returned no hits")
	}
	if hits[0].Display() != "Gurugram, India" {
		t.Fatalf("first hit = %q, want Gurugram, India", hits[0].Display())
	}
}

func TestExpandIncludesAliases(t *testing.T) {
	got := ExpandLocations([]string{"Bengaluru, India"})
	joined := strings.ToLower(strings.Join(got, "|"))
	for _, want := range []string{"bengaluru", "india", "bangalore"} {
		if !strings.Contains(joined, want) {
			t.Errorf("ExpandLocations missing %q in %v", want, got)
		}
	}
	got2 := ExpandLocations([]string{"bangalore"})
	joined2 := strings.ToLower(strings.Join(got2, "|"))
	for _, want := range []string{"bengaluru", "india", "bangalore"} {
		if !strings.Contains(joined2, want) {
			t.Errorf("ExpandLocations(bangalore) missing %q in %v", want, got2)
		}
	}
}
