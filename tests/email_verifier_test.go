// Package tests — see notifier_fanout_test.go for the package doc.
package tests

import (
	"context"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/osint"
)

// TestEmailVerifier_BlackBoxContract exercises the public osint.Verifier API
// from a consumer's perspective (AGENTS.md §13) using only deterministic,
// no-network paths: malformed addresses and disposable domains are rejected
// before any DNS or SMTP activity, and batch verification never drops or
// reorders contacts.
func TestEmailVerifier_BlackBoxContract(t *testing.T) {
	v := osint.NewVerifier()
	ctx := context.Background()

	cases := []struct {
		email string
		want  osint.VerificationStatus
	}{
		{"", osint.StatusInvalid},
		{"not-an-email", osint.StatusInvalid},
		{"foo@", osint.StatusInvalid},
		{"@gmail.com", osint.StatusInvalid},
		{"alice@mailinator.com", osint.StatusInvalid}, // disposable — no network
	}
	for _, c := range cases {
		got := v.Verify(ctx, c.email)
		if got.Status != c.want {
			t.Errorf("Verify(%q) status = %v; want %v (reason %s)", c.email, got.Status, c.want, got.Reason)
		}
		if got.Reason == "" {
			t.Errorf("Verify(%q) has no stable reason", c.email)
		}
	}

	in := []osint.Contact{
		{Email: "hiring@mailinator.com", Source: "scraper", Confidence: 60},
		{Email: "", Source: "github", Confidence: 50},
		{Email: "bad", Source: "pattern", Confidence: 25},
	}
	got := v.VerifyContacts(ctx, in)
	if len(got) != len(in) {
		t.Fatalf("VerifyContacts %d → %d contacts; must never drop", len(in), len(got))
	}
	for i := range in {
		if got[i].Email != in[i].Email {
			t.Errorf("index %d email changed: %q → %q", i, in[i].Email, got[i].Email)
		}
	}
	if got[0].Confidence != 0 || got[0].Notes == "" {
		t.Errorf("disposable contact: confidence=%d notes=%q; want 0 + a reason", got[0].Confidence, got[0].Notes)
	}
	if got[1].Confidence != 50 {
		t.Errorf("empty email must stay untouched; confidence = %d; want 50", got[1].Confidence)
	}
	if got[2].Confidence != 0 {
		t.Errorf("malformed contact must be zeroed; confidence = %d; want 0", got[2].Confidence)
	}
}
