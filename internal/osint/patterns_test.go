package osint

import (
	"strings"
	"testing"
)

func TestGeneratePatterns(t *testing.T) {
	contacts := generatePatterns("Acme", "acme.com")
	if len(contacts) != 7 {
		t.Fatalf("expected 7 pattern contacts, got %d", len(contacts))
	}
	for _, c := range contacts {
		if c.Company != "Acme" {
			t.Errorf("company = %q; want \"Acme\"", c.Company)
		}
		if c.Domain != "acme.com" {
			t.Errorf("domain = %q; want \"acme.com\"", c.Domain)
		}
		if c.EmailType != "pattern" {
			t.Errorf("emailType = %q; want \"pattern\"", c.EmailType)
		}
		if c.Source != "pattern" {
			t.Errorf("source = %q; want \"pattern\"", c.Source)
		}
		if c.Confidence != 25 {
			t.Errorf("confidence = %d; want 25", c.Confidence)
		}
		if c.Title != "Generic Inbox" {
			t.Errorf("title = %q; want \"Generic Inbox\"", c.Title)
		}
		if !strings.HasSuffix(c.Email, "@acme.com") {
			t.Errorf("email = %q; want it to end with \"@acme.com\"", c.Email)
		}
	}
}

func TestGeneratePatterns_EmptyDomain(t *testing.T) {
	contacts := generatePatterns("Acme", "")
	if len(contacts) != 7 {
		t.Fatalf("expected 7 pattern contacts even with empty domain, got %d", len(contacts))
	}
	for _, c := range contacts {
		if !strings.HasSuffix(c.Email, "@") {
			t.Errorf("email = %q; want it to end with \"@\" for empty domain", c.Email)
		}
	}
}

func TestNewFinder(t *testing.T) {
	f := NewFinder("hunter-key", "apollo-key")
	if f.hunterKey != "hunter-key" {
		t.Errorf("hunterKey = %q; want \"hunter-key\"", f.hunterKey)
	}
	if f.apolloKey != "apollo-key" {
		t.Errorf("apolloKey = %q; want \"apollo-key\"", f.apolloKey)
	}
	if f.http == nil {
		t.Error("expected non-nil http client")
	}
	if f.Verify {
		t.Error("expected Verify=false by default")
	}
}

func TestGithubSlugs(t *testing.T) {
	if slugs := githubSlugs("Linear", "linear.app"); !has(slugs, "linear") {
		t.Errorf("githubSlugs(Linear) = %v; want it to contain \"linear\"", slugs)
	}
	slugs := githubSlugs("D.E. Shaw", "deshaw.com")
	if !has(slugs, "deshaw") {
		t.Errorf("githubSlugs(D.E. Shaw) = %v; want it to contain \"deshaw\"", slugs)
	}
	if !has(slugs, "d-e-shaw") {
		t.Errorf("githubSlugs(D.E. Shaw) = %v; want it to contain \"d-e-shaw\"", slugs)
	}
}

func TestIsPersonalEmailDomain(t *testing.T) {
	cases := []struct {
		email string
		want  bool
	}{
		{"x@gmail.com", true},
		{"x@acme.com", false},
		{"bad", false},
		{"X@GMAIL.COM", true},
		{"x@proton.me", true},
	}
	for _, c := range cases {
		if got := isPersonalEmailDomain(c.email); got != c.want {
			t.Errorf("isPersonalEmailDomain(%q) = %v; want %v", c.email, got, c.want)
		}
	}
}

func TestCleanGitHubCompany(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"@Acme", "Acme"},
		{"  Acme  ", "Acme"},
		{"Acme", "Acme"},
		{"", ""},
	}
	for _, c := range cases {
		if got := cleanGitHubCompany(c.in); got != c.want {
			t.Errorf("cleanGitHubCompany(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

// has reports whether ss contains s.
func has(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
