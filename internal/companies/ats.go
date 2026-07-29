package companies

import (
	"net/url"
	"strings"
)

// ParseATSURL extracts ats vendor + board slug from a public careers/ATS URL.
func ParseATSURL(raw string) (ats, board string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	host := strings.ToLower(u.Host)
	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")

	switch {
	case strings.Contains(host, "greenhouse.io"):
		// boards.greenhouse.io/{board} or job-boards.greenhouse.io/{board}
		if len(parts) >= 1 && parts[0] != "" {
			return "greenhouse", parts[0]
		}
	case strings.Contains(host, "lever.co"):
		if len(parts) >= 1 && parts[0] != "" {
			return "lever", parts[0]
		}
	case strings.Contains(host, "ashbyhq.com"):
		if len(parts) >= 1 && parts[0] != "" {
			return "ashby", parts[0]
		}
	case strings.Contains(host, "smartrecruiters.com"):
		// jobs.smartrecruiters.com/{slug} or careers.smartrecruiters.com/{slug}
		if len(parts) >= 1 && parts[0] != "" {
			return "smartrecruiters", parts[0]
		}
	case strings.Contains(host, "workable.com"):
		if len(parts) >= 1 && parts[0] != "" {
			return "workable", parts[0]
		}
	case strings.Contains(host, "jobvite.com"):
		if len(parts) >= 1 && parts[0] != "" {
			return "jobvite", parts[0]
		}
	case strings.Contains(host, "bamboohr.com"):
		// {slug}.bamboohr.com
		sub := strings.TrimSuffix(host, ".bamboohr.com")
		if i := strings.Index(sub, "."); i >= 0 {
			sub = sub[:i]
		}
		if sub != "" && sub != host {
			return "bamboohr", sub
		}
	case strings.Contains(host, "recruitee.com"):
		sub := strings.TrimSuffix(host, ".recruitee.com")
		if sub != "" && sub != host {
			return "recruitee", sub
		}
	case strings.Contains(host, "breezy.hr"):
		sub := strings.TrimSuffix(host, ".breezy.hr")
		if sub != "" && sub != host {
			return "breezy", sub
		}
	case strings.Contains(host, "myworkdayjobs.com"):
		return "workday", raw
	case strings.Contains(host, "teamtailor.com"):
		sub := strings.TrimSuffix(host, ".teamtailor.com")
		if sub != "" && sub != host {
			return "teamtailor", sub
		}
	case strings.Contains(host, "personio"):
		return "personio", raw
	}
	return "", ""
}
