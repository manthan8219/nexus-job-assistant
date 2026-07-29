package hackernews

import (
	"regexp"
	"strings"
)

// Parsed comment fields after HTML-stripping a HN job comment.
type parsedComment struct {
	Title    string
	URL      string
	Company  string
	Location string
}

var (
	anchorRe   = regexp.MustCompile(`(?i)<a\s[^>]*href="([^"]+)"[^>]*>.*?</a>`)
	blockTagRe = regexp.MustCompile(`(?i)</?(?:p|br|div|li|h[1-6])\b[^>]*>`)
	tagRe      = regexp.MustCompile(`<[^>]+>`)
	urlRe      = regexp.MustCompile(`https?://[^\s<>"')]+`)
	trailingRe = regexp.MustCompile(`[.,;!?)]+$`)
)

var entityReplacer = strings.NewReplacer(
	"&amp;", "&",
	"&lt;", "<",
	"&gt;", ">",
	"&quot;", "\"",
	"&#x27;", "'",
	"&#39;", "'",
	"&nbsp;", " ",
)

// parseComment strips HTML from a HN comment and extracts job fields.
// The canonical format is "Company | Role | Location | URL" on the first
// line, but posts are free-form — we guarantee title (first line) and url
// (first URL found anywhere). company and location extracted from pipe-delimited
// header when present; left empty otherwise.
func parseComment(text, threadURL string) *parsedComment {
	if text == "" {
		return nil
	}

	// Strip HTML: keep anchor hrefs inline, block tags as newlines.
	plain := anchorRe.ReplaceAllString(text, "$1")
	plain = blockTagRe.ReplaceAllString(plain, "\n")
	plain = tagRe.ReplaceAllString(plain, " ")
	plain = entityReplacer.Replace(plain)
	plain = strings.ReplaceAll(plain, "\r\n", "\n")
	plain = strings.ReplaceAll(plain, "\r", "\n")

	// Split into lines; first non-blank is the header.
	lines := strings.Split(plain, "\n")
	var firstLine string
	for _, l := range lines {
		if trimmed := strings.TrimSpace(l); trimmed != "" {
			firstLine = trimmed
			break
		}
	}
	if firstLine == "" {
		return nil
	}

	// Try pipe-delimited header: Company | Role | [Location | URL]
	parts := strings.Split(firstLine, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	var company, location string
	if len(parts) >= 3 {
		company = parts[0]
		// parts[2] = location — strip embedded URLs.
		location = urlRe.ReplaceAllString(parts[2], "")
		location = strings.TrimSpace(location)
	} else if len(parts) == 2 {
		company = parts[0]
	}

	// Title = first line with URLs stripped.
	title := urlRe.ReplaceAllString(firstLine, "")
	title = strings.Join(strings.Fields(title), " ") // collapse whitespace
	if title == "" {
		return nil
	}

	// First URL anywhere in the plain text.
	url := extractURL(plain)
	if url == "" {
		url = threadURL
	}
	if url == "" {
		return nil
	}

	return &parsedComment{
		Title:    title,
		URL:      url,
		Company:  company,
		Location: location,
	}
}

// extractURL finds the first absolute http/https URL and cleans trailing punctuation.
func extractURL(text string) string {
	m := urlRe.FindString(text)
	if m == "" {
		return ""
	}
	return trailingRe.ReplaceAllString(m, "")
}
