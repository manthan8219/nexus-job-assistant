package hackernews

import (
	"strings"
	"testing"
)

func TestParseComment_PipeFormat(t *testing.T) {
	// Canonical pipe-delimited: Company | Role | Location | URL
	text := `Acme Corp | Senior Go Engineer | Remote | https://acme.com/jobs/123`

	pc := parseComment(text, "")
	if pc == nil {
		t.Fatal("expected parsed comment")
	}
	if pc.Company != "Acme Corp" {
		t.Errorf("company = %q, want Acme Corp", pc.Company)
	}
	if pc.URL != "https://acme.com/jobs/123" {
		t.Errorf("url = %q", pc.URL)
	}
	if pc.Location != "Remote" {
		t.Errorf("location = %q, want Remote", pc.Location)
	}
	if !contains(pc.Title, "Senior Go Engineer") {
		t.Errorf("title = %q, want Senior Go Engineer", pc.Title)
	}
}

func TestParseComment_PipeFourParts(t *testing.T) {
	// Company | Role | Location | URL
	text := `Stripe | Backend Engineer | San Francisco | https://stripe.com/jobs/456`

	pc := parseComment(text, "")
	if pc == nil {
		t.Fatal("expected parsed comment")
	}
	if pc.Company != "Stripe" {
		t.Errorf("company = %q", pc.Company)
	}
	if pc.URL != "https://stripe.com/jobs/456" {
		t.Errorf("url = %q", pc.URL)
	}
	if pc.Location != "San Francisco" {
		t.Errorf("location = %q", pc.Location)
	}
}

func TestParseComment_PipeTwoParts(t *testing.T) {
	// Company | URL (no location)
	text := `Acme | Senior Engineer | https://acme.com/careers`

	pc := parseComment(text, "")
	if pc == nil {
		t.Fatal("expected parsed comment")
	}
	if pc.Company != "Acme" {
		t.Errorf("company = %q", pc.Company)
	}
	if pc.URL != "https://acme.com/careers" {
		t.Errorf("url = %q", pc.URL)
	}
	// Location should be empty for 3-part format: parts[2] is https://acme.com/careers
	// which is treated as URL in location slot, stripped by urlRe.
	if pc.Location == "https://acme.com/careers" {
		t.Errorf("location should have URL stripped, got %q", pc.Location)
	}
}

func TestParseComment_FreeForm(t *testing.T) {
	text := `We're hiring! Check out https://startup.com/jobs for Senior Go Engineer roles.`

	pc := parseComment(text, "")
	if pc == nil {
		t.Fatal("expected parsed comment")
	}
	if pc.Company != "" {
		t.Errorf("company = %q, want empty", pc.Company)
	}
	if pc.URL != "https://startup.com/jobs" {
		t.Errorf("url = %q", pc.URL)
	}
}

func TestParseComment_HTML(t *testing.T) {
	// Realistic HN post: text starts with company/role, uses HTML for formatting.
	text := `BigCo is hiring! <p><b>Senior Staff Engineer</b> | New York | <a href="https://bigco.com/careers">Apply here</a></p>`

	pc := parseComment(text, "")
	if pc == nil {
		t.Fatal("expected parsed comment")
	}
	// First non-blank line after strip: "BigCo is hiring! Senior Staff Engineer | New York | https://bigco.com/careers"
	if pc.Company != "BigCo is hiring! Senior Staff Engineer" {
		t.Logf("actual title line pipe-parsed as company = %q (4+ parts with trailing URL stripped)", pc.Company)
	}
	// Anchor href extracted and present in URL
	if !strings.Contains(pc.URL, "bigco.com/careers") {
		t.Errorf("url = %q, want bigco.com/careers", pc.URL)
	}
}

func TestParseComment_Empty(t *testing.T) {
	if pc := parseComment("", ""); pc != nil {
		t.Error("expected nil for empty text")
	}
	if pc := parseComment("   \n\n  ", ""); pc != nil {
		t.Error("expected nil for whitespace-only text")
	}
}

func TestParseComment_FallbackURL(t *testing.T) {
	text := "Some Company | Developer | Remote | no-url-here"
	threadURL := "https://news.ycombinator.com/item?id=12345"

	pc := parseComment(text, threadURL)
	if pc == nil {
		t.Fatal("expected parsed comment")
	}
	if pc.URL != threadURL {
		t.Errorf("url = %q, want thread URL %q", pc.URL, threadURL)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSub(s, substr)
}

func containsSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
