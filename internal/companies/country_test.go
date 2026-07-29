package companies

import "testing"

func TestNormalizeCountry(t *testing.T) {
	cases := []struct {
		in, name, iso string
	}{
		{"IN", "India", "IN"},
		{"india", "India", "IN"},
		{"India", "India", "IN"},
		{"US", "United States", "US"},
		{"usa", "United States", "US"},
		{"United Kingdom", "United Kingdom", "GB"},
		{"UK", "United Kingdom", "GB"},
	}
	for _, tc := range cases {
		name, iso, ok := NormalizeCountry(tc.in)
		if !ok || name != tc.name || iso != tc.iso {
			t.Fatalf("%q → (%q,%q,%v) want (%q,%q,true)", tc.in, name, iso, ok, tc.name, tc.iso)
		}
	}
}

func TestCountryKey(t *testing.T) {
	if CountryKey("India") != "in" || CountryKey("IN") != "in" {
		t.Fatalf("CountryKey India/IN mismatch")
	}
}
