package osint

import (
	"strings"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  Alice@Acme.COM  ", "Alice@acme.com"},
		{"alice@acme.com", "alice@acme.com"},
		{"", ""},
		{"   ", ""},
		{"not-an-email", "not-an-email"},
		{"foo@", "foo@"},
	}
	for _, c := range cases {
		if got := normalizeEmail(c.in); got != c.want {
			t.Errorf("normalizeEmail(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestValidEmail(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"alice@acme.com", true},
		{"a.b+c-d@sub.domain.co.uk", true},
		{"first_last+tag@example.io", true},
		{"user@123.xyz", true},
		{"UPPER.Case@Acme.COM", true},
		{"", false},
		{"   ", false},
		{"not-an-email", false},
		{"foo@", false},
		{"@gmail.com", false},
		{"a@b@c.com", false},
		{"foo bar@acme.com", false},
		{"foo@acme..com", false},
		{"foo@-acme.com", false},
		{"foo@acme.com-", false},
		{".foo@acme.com", false},
		{"foo.@acme.com", false},
		{"foo..bar@acme.com", false},
		{"foo@localhost", false},                       // single label
		{"foo@acme", false},                            // single label
		{strings.Repeat("a", 65) + "@acme.com", false}, // local part > 64
		{"a@" + strings.Repeat("x", 254), false},       // domain too long
	}
	for _, c := range cases {
		if got := validEmail(c.in); got != c.want {
			t.Errorf("validEmail(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

func TestDomainOf(t *testing.T) {
	cases := []struct{ in, want string }{
		{"alice@Acme.COM", "acme.com"},
		{"alice@acme.com.", "acme.com"},
		{"alice@acme.com", "acme.com"},
		{"no-at-sign", ""},
		{"foo@", ""},
		{"@gmail.com", "gmail.com"},
		{"", ""},
	}
	for _, c := range cases {
		if got := domainOf(c.in); got != c.want {
			t.Errorf("domainOf(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestIsDisposableDomain(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"alice@mailinator.com", true},
		{"alice@guerrillamail.com", true},
		{"alice@10minutemail.com", true},
		{"ALICE@MAILINATOR.COM", true},
		{"alice@acme.com", false},
		{"alice@gmail.com", false},
		{"not-an-email", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isDisposableDomain(c.in); got != c.want {
			t.Errorf("isDisposableDomain(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

func TestIsRoleAddress(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"info@acme.com", true},
		{"sales@acme.com", true},
		{"postmaster@acme.com", true},
		{"careers@acme.com", true},
		{"INFO@acme.com", true},
		{"alice@acme.com", false},
		{"alice.smith@acme.com", false},
		{"", false},
		{"not-an-email", false},
	}
	for _, c := range cases {
		if got := isRoleAddress(c.in); got != c.want {
			t.Errorf("isRoleAddress(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

func TestRandomLocalPart(t *testing.T) {
	a, b := randomLocalPart(), randomLocalPart()
	if a == b {
		t.Errorf("randomLocalPart() returned the same value twice: %q", a)
	}
	if !strings.HasPrefix(a, "verif") || len(a) < 8 {
		t.Errorf("randomLocalPart() = %q; want a verif-prefixed random value", a)
	}
}
