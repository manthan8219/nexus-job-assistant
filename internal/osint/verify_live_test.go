package osint

import (
	"context"
	"os"
	"testing"
	"time"
)

// liveEnabled skips a test unless NEXUS_E2E_NET=1 is set. These tests hit
// real DNS and outbound SMTP port 25, which CI environments (and many home
// ISPs) block. Run them on a network that allows port 25 with:
//
//	$env:NEXUS_E2E_NET=1; go test ./internal/osint -run TestLive -v
func liveEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("NEXUS_E2E_NET") == "" {
		t.Skip("// reason: live SMTP verification needs outbound port 25 + real DNS; set NEXUS_E2E_NET=1 to run")
	}
}

// TestLiveVerifier_UserGmail is the ground-truth check: the user's own Gmail
// account must never come back invalid. A definitive rejection here would be
// a verifier bug; inconclusive (blocked port 25) is a network limitation.
func TestLiveVerifier_UserGmail(t *testing.T) {
	liveEnabled(t)
	v := NewVerifier()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	got := v.Verify(ctx, "manthanbhatia367@gmail.com")
	t.Logf("manthanbhatia367@gmail.com → %v (%d%%) reason=%s detail=%q (%s)",
		got.Status, got.Confidence, got.Reason, got.Detail, got.Duration)
	if got.Status == StatusInvalid {
		t.Errorf("user's real Gmail classified %v (%s); a real account must never be invalid", got.Status, got.Reason)
	}
}

// TestLiveVerifier_RandomGmailIsNotValid probes a nonexistent local part on
// Gmail: Gmail rejects unknown users with 550, so this must never come back
// valid or catch-all.
func TestLiveVerifier_RandomGmailIsNotValid(t *testing.T) {
	liveEnabled(t)
	v := NewVerifier()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	email := "zxqwerty-nonexistent-" + randomLocalPart() + "@gmail.com"
	got := v.Verify(ctx, email)
	t.Logf("%s → %v reason=%s detail=%q", email, got.Status, got.Reason, got.Detail)
	if got.Status == StatusValid || got.Status == StatusCatchAll {
		t.Errorf("random address on gmail.com must never verify as %v", got.Status)
	}
}

// TestLiveVerifier_NonexistentDomain uses the reserved .invalid TLD, which
// never resolves — it needs only DNS, so it runs even without port 25 access.
func TestLiveVerifier_NonexistentDomain(t *testing.T) {
	v := NewVerifier()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	got := v.Verify(ctx, "nobody@definitely-not-a-real-domain-xyz123.invalid")
	t.Logf("nonexistent domain → %v reason=%s", got.Status, got.Reason)
	if got.Status != StatusInvalid {
		t.Errorf("reserved .invalid domain: status = %v; want invalid", got.Status)
	}
}

// TestLiveVerifier_SampleRealAddresses reports verification results for real
// internet addresses so the confidence model can be eyeballed against ground
// truth. postmaster@ must exist per RFC 5321 on the big providers.
func TestLiveVerifier_SampleRealAddresses(t *testing.T) {
	liveEnabled(t)
	cases := []struct{ email, label string }{
		{"postmaster@google.com", "Google postmaster (must exist)"},
		{"postmaster@microsoft.com", "Microsoft postmaster"},
		{"info@openai.com", "OpenAI info inbox"},
		{"support@github.com", "GitHub support inbox"},
		{"careers@linear.app", "Linear careers inbox"},
	}
	v := NewVerifier()
	for _, c := range cases {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		got := v.Verify(ctx, c.email)
		cancel()
		t.Logf("%-28s [%s] → %v (%d%%) reason=%s detail=%q",
			c.email, c.label, got.Status, got.Confidence, got.Reason, got.Detail)
		if got.Status == StatusInvalid && c.label == "Google postmaster (must exist)" {
			t.Errorf("postmaster@google.com classified %v (%s)", got.Status, got.Reason)
		}
	}
}
