package osint

import "fmt"

// generatePatterns creates likely email patterns for a domain as a fallback.
func generatePatterns(company, domain string) []Contact {
	addresses := []struct {
		local string
		title string
	}{
		{"careers", "Careers Inbox"},
		{"recruiting", "Recruiting Team"},
		{"talent", "Talent Acquisition"},
		{"jobs", "Jobs Inbox"},
		{"hr", "HR Team"},
		{"people", "People Team"},
		{"hiring", "Hiring Team"},
	}

	var contacts []Contact
	for _, a := range addresses {
		contacts = append(contacts, Contact{
			Company:    company,
			Domain:     domain,
			Name:       a.title,
			Title:      "Generic Inbox",
			Email:      fmt.Sprintf("%s@%s", a.local, domain),
			EmailType:  "pattern",
			Source:     "pattern",
			Confidence: 25,
		})
	}
	return contacts
}
