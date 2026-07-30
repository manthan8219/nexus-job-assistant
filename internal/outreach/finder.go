package outreach

import (
	"net/url"
	"regexp"
	"strings"
)

var nonDomain = regexp.MustCompile(`(?i)\b(inc|llc|ltd|corp|corporation|company|co|the)\b`)
var spaceRe = regexp.MustCompile(`\s+`)

// Contact is a resolved recruiter/HM email.
type Contact struct {
	Name  string
	Email string
	Title string
}

// GuessDomain derives a likely company email domain.
func GuessDomain(company, jobURL string) string {
	if u, err := url.Parse(jobURL); err == nil {
		host := strings.ToLower(u.Hostname())
		// lever: company.lever.co / jobs.lever.co/company
		if strings.HasSuffix(host, ".lever.co") {
			sub := strings.TrimSuffix(host, ".lever.co")
			if sub != "" && sub != "jobs" && !strings.Contains(sub, ".") {
				return sub + ".com"
			}
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) > 0 && parts[0] != "" && parts[0] != "jobs" {
				return sanitizeDomainToken(parts[0]) + ".com"
			}
		}
		if strings.Contains(host, "greenhouse.io") {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			for _, p := range parts {
				if p == "" || p == "embed" || p == "jobs" {
					continue
				}
				return sanitizeDomainToken(p) + ".com"
			}
		}
		// ashby: jobs.ashbyhq.com/company
		if strings.Contains(host, "ashbyhq.com") {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) > 0 && parts[0] != "" {
				return sanitizeDomainToken(parts[0]) + ".com"
			}
		}
	}
	name := strings.ToLower(strings.TrimSpace(company))
	name = nonDomain.ReplaceAllString(name, " ")
	name = spaceRe.ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	tok := sanitizeDomainToken(strings.Fields(name)[0])
	if tok == "" {
		return ""
	}
	return tok + ".com"
}

func sanitizeDomainToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
