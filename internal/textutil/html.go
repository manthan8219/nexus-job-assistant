package textutil

import (
	"html"
	"regexp"
	"strings"
)

var (
	reBR       = regexp.MustCompile(`(?i)<br\s*/?\s*>`)
	reBlockEnd = regexp.MustCompile(`(?i)</\s*(p|div|h[1-6]|li|ul|ol|tr|section|article|header|footer)\s*>`)
	reBlockBeg = regexp.MustCompile(`(?i)<\s*(p|div|h[1-6]|li|ul|ol|tr|section|article|header|footer)(\s[^>]*)?>`)
	reHR       = regexp.MustCompile(`(?i)<hr\s*/?\s*>`)
	reTag      = regexp.MustCompile(`(?s)<[^>]*>`)
	reSpaces   = regexp.MustCompile(`[\t\xA0 ]+`)
	reBlank    = regexp.MustCompile(`\n{3,}`)
)

// HTMLToPlain turns HTML (including double-escaped entities like &amp;lt;p&amp;gt;)
// into readable plain text for TUI display and LLM prompts.
func HTMLToPlain(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// Greenhouse sometimes stores escaped markup (&lt;p&gt;…); unescape until stable.
	for i := 0; i < 4; i++ {
		next := html.UnescapeString(s)
		if next == s {
			break
		}
		s = next
	}

	s = reBR.ReplaceAllString(s, "\n")
	s = reHR.ReplaceAllString(s, "\n")
	s = reBlockEnd.ReplaceAllString(s, "\n")
	s = reBlockBeg.ReplaceAllString(s, "\n")
	s = reTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)

	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\u00a0", " ")

	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(reSpaces.ReplaceAllString(line, " "))
		lines = append(lines, line)
	}
	s = strings.Join(lines, "\n")
	s = reBlank.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
