package osint

import (
	"context"
	"net"
	"testing"
	"time"
)

// TestVerifyContacts_ConfidenceMerge drives contacts from every source type
// through one batch and checks the confidence model end to end.
func TestVerifyContacts_ConfidenceMerge(t *testing.T) {
	srv := newFakeSMTP(t)
	srv.recipients["alice@acme.test"] = 250
	srv.recipients["carl@acme.test"] = 250
	// bob@acme.test stays at the default 550 → rejected.
	v := testVerifier(t, srv, "acme.test", []string{"mx1.acme.test"})

	contacts := []Contact{
		{Email: "alice@acme.test", Source: "pattern", Confidence: 25},    // → 85
		{Email: "alice@acme.test", Source: "hunter", Confidence: 95},     // dedupe, keeps 95
		{Email: "bob@acme.test", Source: "github", Confidence: 60},       // rejected → 0
		{Email: "carl@acme.test", Source: "scraper", Confidence: 60},     // → 85
		{Email: "", Source: "github", Confidence: 50},                    // untouched → 50
		{Email: "not-an-email", Source: "pattern", Confidence: 25},       // malformed → 0
		{Email: "junk@mailinator.com", Source: "hunter", Confidence: 90}, // disposable → 0
	}
	got := v.VerifyContacts(context.Background(), contacts)

	if len(got) != len(contacts) {
		t.Fatalf("VerifyContacts dropped contacts: %d → %d", len(contacts), len(got))
	}
	for i := range contacts {
		if got[i].Email != contacts[i].Email {
			t.Errorf("contact %d reordered: %q → %q", i, contacts[i].Email, got[i].Email)
		}
	}
	assertConf := func(i, want int, what string) {
		t.Helper()
		if got[i].Confidence != want {
			t.Errorf("%s: confidence = %d; want %d", what, got[i].Confidence, want)
		}
	}
	assertConf(0, confidenceConfirmed, "pattern verified")
	assertConf(1, 95, "hunter verified keeps higher source confidence")
	assertConf(2, 0, "github rejected")
	assertConf(3, confidenceConfirmed, "scraper verified")
	assertConf(4, 50, "empty email untouched")
	assertConf(5, 0, "malformed")
	assertConf(6, 0, "disposable")
	if got[2].Notes == "" {
		t.Error("rejected contact has no explanatory note")
	}
}

// TestVerifyContacts_SessionReuse ensures one SMTP connection serves an
// entire domain (politeness + performance), not one connection per address.
func TestVerifyContacts_SessionReuse(t *testing.T) {
	srv := newFakeSMTP(t)
	for _, a := range []string{"a@acme.test", "b@acme.test", "c@acme.test", "d@acme.test"} {
		srv.recipients[a] = 250
	}
	v := testVerifier(t, srv, "acme.test", []string{"mx1.acme.test"})

	contacts := make([]Contact, 4)
	locals := []string{"a", "b", "c", "d"}
	for i := range contacts {
		contacts[i] = Contact{Email: locals[i] + "@acme.test", Source: "pattern", Confidence: 25}
	}
	got := v.VerifyContacts(context.Background(), contacts)
	for i := range got {
		if got[i].Confidence != confidenceConfirmed {
			t.Errorf("contact %d confidence = %d; want %d", i, got[i].Confidence, confidenceConfirmed)
		}
	}
	if n := srv.connCount(); n != 1 {
		t.Errorf("expected exactly 1 SMTP connection for the whole domain; got %d", n)
	}
}

// TestVerifyContacts_UnreachableKeepsSourceConfidence verifies the "never
// lose data, never invent confidence" contract on blocked networks.
func TestVerifyContacts_UnreachableKeepsSourceConfidence(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	v := NewVerifier()
	v.Resolver = &fakeResolver{mx: map[string][]string{"acme.test": {"mx1.acme.test"}}}
	v.Connector = &fakeConnector{addr: addr}
	v.ConnectTimeout = 300 * time.Millisecond

	contacts := []Contact{
		{Email: "alice@acme.test", Source: "hunter", Confidence: 92},
		{Email: "bob@acme.test", Source: "pattern", Confidence: 25},
	}
	got := v.VerifyContacts(context.Background(), contacts)
	if got[0].Confidence != 92 {
		t.Errorf("hunter confidence = %d; want 92 preserved on unreachable network", got[0].Confidence)
	}
	if got[1].Confidence != 25 {
		t.Errorf("pattern confidence = %d; want 25 preserved on unreachable network", got[1].Confidence)
	}
	if got[0].Notes == "" || got[1].Notes == "" {
		t.Error("unreachable contacts must carry an explanatory note")
	}
}

// TestVerifyContacts_CancelledContext reports every remaining address as
// cancelled rather than blocking or panicking.
func TestVerifyContacts_CancelledContext(t *testing.T) {
	v := NewVerifier()
	v.Resolver = &fakeResolver{mx: map[string][]string{"acme.test": {"mx1.acme.test"}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	contacts := []Contact{
		{Email: "a@acme.test", Source: "pattern", Confidence: 25},
		{Email: "b@acme.test", Source: "pattern", Confidence: 25},
	}
	got := v.VerifyContacts(ctx, contacts)
	if len(got) != 2 {
		t.Fatalf("len = %d; want 2 (never drop)", len(got))
	}
	for i, c := range got {
		if c.Notes == "" {
			t.Errorf("contact %d has no note; want cancellation reason", i)
		}
	}
}

// TestVerifyContacts_EmptyInputs is a safe no-op.
func TestVerifyContacts_EmptyInputs(t *testing.T) {
	v := NewVerifier()
	if got := v.VerifyContacts(context.Background(), nil); got != nil {
		t.Errorf("nil input returned %v; want nil", got)
	}
	if got := v.VerifyContacts(context.Background(), []Contact{}); len(got) != 0 {
		t.Errorf("empty input returned %d contacts; want 0", len(got))
	}
}

// TestVerifyContacts_MultipleDomains checks that a batch spanning several
// domains resolves each one independently and keeps everything.
func TestVerifyContacts_MultipleDomains(t *testing.T) {
	srv := newFakeSMTP(t)
	srv.recipients["a@one.test"] = 250
	srv.recipients["x@two.test"] = 250
	v := NewVerifier()
	v.Resolver = &fakeResolver{mx: map[string][]string{
		"one.test": {"mx1.one.test"},
		"two.test": {"mx1.two.test"},
	}}
	v.Connector = &fakeConnector{addr: srv.addr()}
	v.RetryBaseDelay = time.Millisecond
	v.InterProbeDelay = 0

	contacts := []Contact{
		{Email: "a@one.test", Confidence: 25},
		{Email: "b@one.test", Confidence: 25}, // rejected at one.test
		{Email: "x@two.test", Confidence: 25},
	}
	got := v.VerifyContacts(context.Background(), contacts)
	if len(got) != 3 {
		t.Fatalf("len = %d; want 3", len(got))
	}
	if got[0].Confidence != confidenceConfirmed {
		t.Errorf("a@one.test confidence = %d; want %d", got[0].Confidence, confidenceConfirmed)
	}
	if got[1].Confidence != 0 {
		t.Errorf("b@one.test confidence = %d; want 0", got[1].Confidence)
	}
	if got[2].Confidence != confidenceConfirmed {
		t.Errorf("x@two.test confidence = %d; want %d", got[2].Confidence, confidenceConfirmed)
	}
}
