package osint

import (
	"context"
	"errors"
	"net/textproto"
	"testing"
	"time"
)

func TestResolveMailHosts_MXPreferenceOrder(t *testing.T) {
	r := &fakeResolver{mx: map[string][]string{
		"acme.test": {"mx10.acme.test", "mx20.acme.test"},
	}}
	hosts, nullMX := resolveMailHosts(context.Background(), r, "acme.test", 25)
	if nullMX {
		t.Fatal("nullMX = true; want false")
	}
	if len(hosts) != 2 {
		t.Fatalf("len(hosts) = %d; want 2", len(hosts))
	}
	if hosts[0].host != "mx10.acme.test" || hosts[1].host != "mx20.acme.test" {
		t.Errorf("hosts = %+v; want mx10.acme.test then mx20.acme.test in preference order", hosts)
	}
	if hosts[0].port != 25 {
		t.Errorf("port = %d; want 25", hosts[0].port)
	}
}

func TestResolveMailHosts_NullMX(t *testing.T) {
	r := &fakeResolver{nullMX: map[string]bool{"nulldom.test": true}}
	hosts, nullMX := resolveMailHosts(context.Background(), r, "nulldom.test", 25)
	if !nullMX {
		t.Error("nullMX = false; want true (RFC 7505)")
	}
	if len(hosts) != 0 {
		t.Errorf("hosts = %+v; want none", hosts)
	}
}

func TestResolveMailHosts_AFallback(t *testing.T) {
	r := &fakeResolver{hosts: map[string][]string{"acme.test": {"192.0.2.1"}}}
	hosts, nullMX := resolveMailHosts(context.Background(), r, "acme.test", 25)
	if nullMX || len(hosts) != 1 || hosts[0].host != "acme.test" {
		t.Errorf("hosts = %+v nullMX=%v; want the domain itself from its A record", hosts, nullMX)
	}
}

func TestResolveMailHosts_SmtpSubdomainFallback(t *testing.T) {
	r := &fakeResolver{hosts: map[string][]string{"smtp.acme.test": {"192.0.2.2"}}}
	hosts, nullMX := resolveMailHosts(context.Background(), r, "acme.test", 25)
	if nullMX || len(hosts) != 1 || hosts[0].host != "smtp.acme.test" {
		t.Errorf("hosts = %+v nullMX=%v; want smtp.acme.test fallback", hosts, nullMX)
	}
}

func TestResolveMailHosts_None(t *testing.T) {
	r := &fakeResolver{}
	hosts, nullMX := resolveMailHosts(context.Background(), r, "acme.test", 25)
	if len(hosts) != 0 || nullMX {
		t.Errorf("hosts = %+v nullMX=%v; want no hosts and no null-MX", hosts, nullMX)
	}
}

func TestTextprotoCode(t *testing.T) {
	if got := textprotoCode(nil); got != 0 {
		t.Errorf("textprotoCode(nil) = %d; want 0", got)
	}
	if got := textprotoCode(&textproto.Error{Code: 550, Msg: "user unknown"}); got != 550 {
		t.Errorf("textprotoCode(550 error) = %d; want 550", got)
	}
	if got := textprotoCode(errors.New("read: connection reset")); got != 0 {
		t.Errorf("textprotoCode(network error) = %d; want 0", got)
	}
}

func TestClassifyProbe(t *testing.T) {
	cases := []struct {
		name       string
		code       int
		at         string
		err        error
		wantStatus VerificationStatus
		wantReason string
	}{
		{"accepted", 250, "rcpt", nil, StatusValid, reasonConfirmed},
		{"rejected rcpt", 550, "rcpt", errors.New("550 user unknown"), StatusInvalid, reasonRejected},
		{"rejected rcpt 553", 553, "rcpt", errors.New("553 no mailbox"), StatusInvalid, reasonRejected},
		{"sender rejected", 553, "mail", errors.New("553 sender refused"), StatusInconclusive, reasonSenderRejected},
		{"greylisted", 451, "rcpt", errTransient, StatusInconclusive, reasonGreylisted},
		{"cancelled", 0, "rcpt", context.Canceled, StatusInconclusive, reasonCancelled},
		{"deadline", 0, "rcpt", context.DeadlineExceeded, StatusInconclusive, reasonCancelled},
		{"unreachable", 0, "connect", errUnreachable, StatusInconclusive, reasonUnreachable},
		{"network loss", 0, "rcpt", errors.New("read: connection reset"), StatusInconclusive, reasonConnectFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, reason, detail := classifyProbe(c.code, c.at, c.err)
			if status != c.wantStatus {
				t.Errorf("status = %v; want %v", status, c.wantStatus)
			}
			if reason != c.wantReason {
				t.Errorf("reason = %q; want %q", reason, c.wantReason)
			}
			if detail == "" {
				t.Error("detail is empty; want an actionable explanation")
			}
		})
	}
}

func TestApplyVerification(t *testing.T) {
	cases := []struct {
		name     string
		contact  Contact
		v        Verification
		wantConf int
	}{
		{
			"pattern confirmed", Contact{Email: "a@b.com", Source: "pattern", Confidence: 25},
			Verification{Status: StatusValid, Confidence: confidenceConfirmed}, confidenceConfirmed,
		},
		{
			"hunter confirmed keeps higher", Contact{Email: "a@b.com", Source: "hunter", Confidence: 95},
			Verification{Status: StatusValid, Confidence: confidenceConfirmed}, 95,
		},
		{
			"pattern catch-all", Contact{Email: "a@b.com", Source: "pattern", Confidence: 25},
			Verification{Status: StatusCatchAll, Confidence: confidenceCatchAll}, confidenceCatchAll,
		},
		{
			"hunter catch-all keeps higher", Contact{Email: "a@b.com", Source: "hunter", Confidence: 90},
			Verification{Status: StatusCatchAll, Confidence: confidenceCatchAll}, 90,
		},
		{
			"rejected zeroes", Contact{Email: "a@b.com", Source: "github", Confidence: 60},
			Verification{Status: StatusInvalid, Confidence: 0}, 0,
		},
		{
			"malformed zeroes", Contact{Email: "bad", Source: "hunter", Confidence: 95},
			Verification{Status: StatusInvalid, Confidence: 0}, 0,
		},
		{
			"inconclusive keeps source", Contact{Email: "a@b.com", Source: "hunter", Confidence: 92},
			Verification{Status: StatusInconclusive, Confidence: 0}, 92,
		},
		{
			"inconclusive reachable bumps pattern", Contact{Email: "a@b.com", Source: "pattern", Confidence: 25},
			Verification{Status: StatusInconclusive, Confidence: confidenceDomainReachable}, confidenceDomainReachable,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := applyVerification(c.contact, c.v)
			if got.Confidence != c.wantConf {
				t.Errorf("confidence = %d; want %d", got.Confidence, c.wantConf)
			}
			if got.Email != c.contact.Email {
				t.Errorf("email changed: %q → %q", c.contact.Email, got.Email)
			}
			if c.v.Detail != "" && got.Notes != c.v.Detail {
				t.Errorf("notes = %q; want %q", got.Notes, c.v.Detail)
			}
		})
	}
}

func TestSleepCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleep(ctx, time.Hour) {
		t.Error("sleep returned true on a cancelled context; want false")
	}
	if !sleep(context.Background(), 0) {
		t.Error("sleep with zero delay returned false; want true")
	}
}
