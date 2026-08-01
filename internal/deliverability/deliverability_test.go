package deliverability

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeResolver serves canned TXT records per name; any name not in the map
// returns a DNS error, which the audit treats as "no record / unknown".
type fakeResolver struct {
	records map[string][]string
	errs    map[string]error
}

func (f *fakeResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.errs != nil {
		if e, ok := f.errs[name]; ok {
			return nil, e
		}
	}
	return f.records[name], nil
}

func TestValidDomain(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"example.com", true},
		{"mail.example.co.uk", true},
		{"sub.example.com", true},
		{"example.com.", true},
		{"EXAMPLE.com", true},
		{"", false},
		{"localhost", false},
		{"example", false},
		{"https://example.com", false},
		{"example.com/path", false},
		{"example.com:443", false},
		{"exa mple.com", false},
		{"-example.com", false},
		{"example-.com", false},
		{"sub.-bad.com", false},
		{"sub.bad-.com", false},
		{"exa..mple.com", false},
		{"a." + strings.Repeat("b", 63) + ".com", true},
		{"a." + strings.Repeat("b", 64) + ".com", false},
	}
	for _, tt := range tests {
		if got := ValidDomain(tt.in); got != tt.want {
			t.Errorf("ValidDomain(%q) = %v; want %v", tt.in, got, tt.want)
		}
	}
}

func TestAudit_InvalidDomain(t *testing.T) {
	for _, in := range []string{"", "https://example.com", "not a domain"} {
		if _, err := Audit(context.Background(), in, &fakeResolver{}); err == nil {
			t.Errorf("Audit(%q): want error, got nil", in)
		}
	}
}

func TestAudit_SPF(t *testing.T) {
	tests := []struct {
		name        string
		spf         []string
		wantPresent bool
		wantVerdict string
	}{
		{"missing", nil, false, "missing"},
		{"softfail", []string{"v=spf1 include:_spf.google.com ~all"}, true, "softfail"},
		{"hardfail", []string{"v=spf1 include:_spf.google.com -all"}, true, "pass (hard fail)"},
		{"no all qualifier", []string{"v=spf1 include:_spf.google.com"}, true, "pass (no enforcement)"},
		{"non-spf txt ignored", []string{"some-other-txt"}, false, "missing"},
		{"dns error", nil, false, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := &fakeResolver{records: map[string][]string{"example.com": tt.spf}}
			if tt.name == "dns error" {
				fr = &fakeResolver{errs: map[string]error{"example.com": errors.New("nxdomain")}}
			}
			rep, err := Audit(context.Background(), "example.com", fr)
			if err != nil {
				t.Fatalf("Audit: %v", err)
			}
			if rep.SPF.Present != tt.wantPresent || rep.SPF.Verdict != tt.wantVerdict {
				t.Errorf("SPF present=%v verdict=%q; want present=%v verdict=%q",
					rep.SPF.Present, rep.SPF.Verdict, tt.wantPresent, tt.wantVerdict)
			}
			if rep.SPF.Guidance == "" {
				t.Error("SPF guidance must never be empty")
			}
		})
	}
}

func TestAudit_DMARC(t *testing.T) {
	tests := []struct {
		name        string
		dmarc       []string
		wantPresent bool
		wantPolicy  string
	}{
		{"missing", nil, false, "missing"},
		{"reject", []string{"v=DMARC1; p=reject; rua=mailto:postmaster@example.com"}, true, "reject"},
		{"quarantine", []string{"v=DMARC1; p=quarantine"}, true, "quarantine"},
		{"none", []string{"v=DMARC1; p=none"}, true, "none"},
		{"case-insensitive policy", []string{"v=DMARC1; P=REJECT"}, true, "reject"},
		{"non-dmarc ignored", []string{"spf-only"}, false, "missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := &fakeResolver{records: map[string][]string{"_dmarc.example.com": tt.dmarc}}
			rep, err := Audit(context.Background(), "example.com", fr)
			if err != nil {
				t.Fatalf("Audit: %v", err)
			}
			if rep.DMARC.Present != tt.wantPresent || rep.DMARC.Verdict != tt.wantPolicy {
				t.Errorf("DMARC present=%v verdict=%q; want present=%v verdict=%q",
					rep.DMARC.Present, rep.DMARC.Verdict, tt.wantPresent, tt.wantPolicy)
			}
			if rep.DMARC.Guidance == "" {
				t.Error("DMARC guidance must never be empty")
			}
		})
	}
}

func TestAudit_DKIM(t *testing.T) {
	records := map[string][]string{
		"google._domainkey.example.com": {"v=DKIM1; k=rsa; p=MIGfMA0GCSqGSIb3"},
	}
	fr := &fakeResolver{records: records}
	rep, err := Audit(context.Background(), "example.com", fr)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if !rep.DKIM.Found || len(rep.DKIM.Selectors) != 1 || rep.DKIM.Selectors[0] != "google" {
		t.Errorf("DKIM = %+v; want found with [google]", rep.DKIM)
	}

	none := &fakeResolver{}
	rep2, err := Audit(context.Background(), "nodkim.com", none)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if rep2.DKIM.Found {
		t.Error("DKIM must not be found when no selectors resolve")
	}
}

func TestAudit_Summary(t *testing.T) {
	// Everything in place.
	fr := &fakeResolver{records: map[string][]string{
		"example.com":                   {"v=spf1 include:_spf.google.com ~all"},
		"_dmarc.example.com":            {"v=DMARC1; p=quarantine"},
		"google._domainkey.example.com": {"v=DKIM1; p=MIGfMA"},
	}}
	rep, err := Audit(context.Background(), "example.com", fr)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if !strings.Contains(rep.Summary, "in place") {
		t.Errorf("Summary = %q; want success message", rep.Summary)
	}

	// All missing.
	empty := &fakeResolver{}
	rep2, err := Audit(context.Background(), "bare.com", empty)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	for _, want := range []string{"SPF missing", "DMARC missing", "DKIM not found"} {
		if !strings.Contains(rep2.Summary, want) {
			t.Errorf("Summary %q should mention %q", rep2.Summary, want)
		}
	}
}

func TestAudit_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fr := &fakeResolver{records: map[string][]string{
		"example.com":                   {"v=spf1 -all"},
		"_dmarc.example.com":            {"v=DMARC1; p=reject"},
		"google._domainkey.example.com": {"v=DKIM1; p=MIGfMA"},
	}}
	rep, err := Audit(ctx, "example.com", fr)
	if err != nil {
		t.Fatalf("Audit with cancelled ctx: %v", err)
	}
	// Cancellation must not panic and must surface as unknown/missing.
	if rep.SPF.Verdict == "" || rep.DMARC.Verdict == "" {
		t.Errorf("verdicts must be populated after cancellation; got %+v", rep)
	}
}
