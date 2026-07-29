package outreach

import "testing"

func TestNormalizeMode(t *testing.T) {
	cases := map[string]string{
		"": "confirm", "assisted": "confirm", "confirm": "confirm",
		"queue": "queue", "autosend": "auto", "auto": "auto",
	}
	for in, want := range cases {
		if got := NormalizeMode(in); got != want {
			t.Fatalf("NormalizeMode(%q)=%q want %q", in, got, want)
		}
	}
}

func TestGuessDomain(t *testing.T) {
	if d := GuessDomain("Acme Inc", ""); d != "acme.com" {
		t.Fatalf("company guess: %q", d)
	}
	if d := GuessDomain("Ignored", "https://jobs.lever.co/stripe/abc"); d != "stripe.com" {
		t.Fatalf("lever guess: %q", d)
	}
}
