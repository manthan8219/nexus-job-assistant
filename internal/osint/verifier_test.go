package osint

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestVerifier_ValidAddress(t *testing.T) {
	srv := newFakeSMTP(t)
	srv.recipients["alice@acme.test"] = 250
	v := testVerifier(t, srv, "acme.test", []string{"mx1.acme.test"})

	got := v.Verify(context.Background(), "alice@acme.test")
	if got.Status != StatusValid {
		t.Fatalf("status = %v (%s); want %v", got.Status, got.Reason, StatusValid)
	}
	if got.Confidence != confidenceConfirmed {
		t.Errorf("confidence = %d; want %d", got.Confidence, confidenceConfirmed)
	}
	if got.Reason != reasonConfirmed {
		t.Errorf("reason = %q; want %q", got.Reason, reasonConfirmed)
	}
	if got.Duration < 0 {
		t.Errorf("duration = %v; want non-negative", got.Duration)
	}
}

func TestVerifier_CatchAll(t *testing.T) {
	srv := newFakeSMTP(t)
	srv.catchAll = true
	v := testVerifier(t, srv, "acme.test", []string{"mx1.acme.test"})

	got := v.Verify(context.Background(), "alice@acme.test")
	if got.Status != StatusCatchAll {
		t.Fatalf("status = %v (%s); want %v", got.Status, got.Reason, StatusCatchAll)
	}
	if got.Confidence != confidenceCatchAll {
		t.Errorf("confidence = %d; want %d", got.Confidence, confidenceCatchAll)
	}
	if got.Reason != reasonCatchAll {
		t.Errorf("reason = %q; want %q", got.Reason, reasonCatchAll)
	}
}

func TestVerifier_CatchAllUnknown(t *testing.T) {
	srv := newFakeSMTP(t)
	srv.recipients["alice@acme.test"] = 250
	srv.transientPrefix["verif"] = 100 // the random catch-all probe always 451s
	v := testVerifier(t, srv, "acme.test", []string{"mx1.acme.test"})

	got := v.Verify(context.Background(), "alice@acme.test")
	if got.Status != StatusValid {
		t.Fatalf("status = %v (%s); want %v", got.Status, got.Reason, StatusValid)
	}
	if got.Confidence != confidenceConfirmedCatchAllUnknown {
		t.Errorf("confidence = %d; want %d", got.Confidence, confidenceConfirmedCatchAllUnknown)
	}
	if got.Reason != reasonConfirmedCatchAllUkn {
		t.Errorf("reason = %q; want %q", got.Reason, reasonConfirmedCatchAllUkn)
	}
}

func TestVerifier_Rejected(t *testing.T) {
	srv := newFakeSMTP(t) // default 550 for unknown recipients
	v := testVerifier(t, srv, "acme.test", []string{"mx1.acme.test"})

	got := v.Verify(context.Background(), "nobody@acme.test")
	if got.Status != StatusInvalid {
		t.Fatalf("status = %v (%s); want %v", got.Status, got.Reason, StatusInvalid)
	}
	if got.Confidence != 0 {
		t.Errorf("confidence = %d; want 0", got.Confidence)
	}
	if got.Reason != reasonRejected {
		t.Errorf("reason = %q; want %q", got.Reason, reasonRejected)
	}
}

func TestVerifier_GreylistingRecovers(t *testing.T) {
	srv := newFakeSMTP(t)
	srv.recipients["alice@acme.test"] = 250
	srv.transient["alice@acme.test"] = 2 // two 451s, then 250
	v := testVerifier(t, srv, "acme.test", []string{"mx1.acme.test"})

	got := v.Verify(context.Background(), "alice@acme.test")
	if got.Status != StatusValid {
		t.Fatalf("status = %v (%s); want valid after greylisting retries", got.Status, got.Reason)
	}
}

func TestVerifier_GreylistingPersistent(t *testing.T) {
	srv := newFakeSMTP(t)
	srv.recipients["alice@acme.test"] = 250
	srv.transient["alice@acme.test"] = 100 // always 451
	v := testVerifier(t, srv, "acme.test", []string{"mx1.acme.test"})
	v.MaxAttempts = 2

	got := v.Verify(context.Background(), "alice@acme.test")
	if got.Status != StatusInconclusive {
		t.Fatalf("status = %v (%s); want %v", got.Status, got.Reason, StatusInconclusive)
	}
	if got.Reason != reasonGreylisted {
		t.Errorf("reason = %q; want %q", got.Reason, reasonGreylisted)
	}
	if got.Confidence != confidenceDomainReachable {
		t.Errorf("confidence = %d; want %d (domain reachable)", got.Confidence, confidenceDomainReachable)
	}
}

func TestVerifier_Unreachable(t *testing.T) {
	// Reserve a port and close it so dialing is refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	v := NewVerifier()
	v.Resolver = &fakeResolver{mx: map[string][]string{"acme.test": {"mx1.acme.test"}}}
	v.Connector = &fakeConnector{addr: addr}
	v.ConnectTimeout = 500 * time.Millisecond

	got := v.Verify(context.Background(), "alice@acme.test")
	if got.Status != StatusInconclusive {
		t.Fatalf("status = %v (%s); want %v", got.Status, got.Reason, StatusInconclusive)
	}
	if got.Reason != reasonUnreachable {
		t.Errorf("reason = %q; want %q", got.Reason, reasonUnreachable)
	}
	if got.Confidence != 0 {
		t.Errorf("confidence = %d; want 0 (unreachable)", got.Confidence)
	}
}

func TestVerifier_GreetingTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				time.Sleep(5 * time.Second) // never send the 220 greeting
				c.Close()
			}(conn)
		}
	}()

	v := NewVerifier()
	v.Resolver = &fakeResolver{mx: map[string][]string{"acme.test": {"mx1.acme.test"}}}
	v.Connector = &fakeConnector{addr: ln.Addr().String()}
	v.ReadTimeout = 150 * time.Millisecond

	got := v.Verify(context.Background(), "alice@acme.test")
	if got.Status != StatusInconclusive {
		t.Fatalf("status = %v (%s); want %v for a server that never greets", got.Status, got.Reason, StatusInconclusive)
	}
	if got.Confidence != 0 {
		t.Errorf("confidence = %d; want 0", got.Confidence)
	}
}

func TestVerifier_ServerWithoutRset(t *testing.T) {
	srv := newFakeSMTP(t)
	srv.rsetCode = 502 // minimal servers reply 502 to RSET
	srv.recipients["alice@acme.test"] = 250
	v := testVerifier(t, srv, "acme.test", []string{"mx1.acme.test"})

	got := v.Verify(context.Background(), "alice@acme.test")
	if got.Status != StatusValid {
		t.Fatalf("status = %v (%s); want valid even without RSET support", got.Status, got.Reason)
	}
}

func TestVerifier_StartTLS(t *testing.T) {
	srv := newFakeSMTP(t)
	srv.startTLS = true
	srv.recipients["alice@acme.test"] = 250
	v := testVerifier(t, srv, "acme.test", []string{"mx1.acme.test"})

	got := v.Verify(context.Background(), "alice@acme.test")
	if got.Status != StatusValid {
		t.Fatalf("status = %v (%s); want valid over STARTTLS", got.Status, got.Reason)
	}
}

func TestVerifier_MalformedAndDisposable(t *testing.T) {
	v := NewVerifier() // no DNS or SMTP involved
	cases := []struct {
		email      string
		wantReason string
	}{
		{"", reasonMalformed},
		{"   ", reasonMalformed},
		{"not-an-email", reasonMalformed},
		{"foo@", reasonMalformed},
		{"@gmail.com", reasonMalformed},
		{"a@b@c.com", reasonMalformed},
		{"alice@mailinator.com", reasonDisposable},
	}
	for _, c := range cases {
		got := v.Verify(context.Background(), c.email)
		if got.Status != StatusInvalid {
			t.Errorf("Verify(%q) status = %v; want %v", c.email, got.Status, StatusInvalid)
		}
		if got.Reason != c.wantReason {
			t.Errorf("Verify(%q) reason = %q; want %q", c.email, got.Reason, c.wantReason)
		}
		if got.Confidence != 0 {
			t.Errorf("Verify(%q) confidence = %d; want 0", c.email, got.Confidence)
		}
	}
}

func TestVerifier_NoMailHost(t *testing.T) {
	v := NewVerifier()
	v.Resolver = &fakeResolver{}
	got := v.Verify(context.Background(), "alice@acme.test")
	if got.Status != StatusInvalid || got.Reason != reasonNoMailHost {
		t.Errorf("status = %v reason = %q; want invalid + %q", got.Status, got.Reason, reasonNoMailHost)
	}
}

func TestVerifier_NullMX(t *testing.T) {
	v := NewVerifier()
	v.Resolver = &fakeResolver{nullMX: map[string]bool{"acme.test": true}}
	got := v.Verify(context.Background(), "alice@acme.test")
	if got.Status != StatusInvalid || got.Reason != reasonNullMX {
		t.Errorf("status = %v reason = %q; want invalid + %q", got.Status, got.Reason, reasonNullMX)
	}
}

func TestVerifier_CancelledContext(t *testing.T) {
	v := NewVerifier()
	v.Resolver = &fakeResolver{mx: map[string][]string{"acme.test": {"mx1.acme.test"}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got := v.Verify(ctx, "alice@acme.test")
	if got.Status != StatusInconclusive {
		t.Fatalf("status = %v; want %v for a cancelled context", got.Status, StatusInconclusive)
	}
	if got.Reason != reasonCancelled {
		t.Errorf("reason = %q; want %q", got.Reason, reasonCancelled)
	}
}

func TestVerifier_ZeroValueGetsDefaults(t *testing.T) {
	var v Verifier // not NewVerifier — must still behave
	got := v.Verify(context.Background(), "alice@mailinator.com")
	if got.Status != StatusInvalid || got.Reason != reasonDisposable {
		t.Errorf("zero-value Verifier: status = %v reason = %q; want invalid + disposable (no panic)", got.Status, got.Reason)
	}
}
