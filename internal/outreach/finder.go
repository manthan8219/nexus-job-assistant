package outreach

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/config"
)

var nonDomain = regexp.MustCompile(`(?i)\b(inc|llc|ltd|corp|corporation|company|co|the)\b`)
var spaceRe = regexp.MustCompile(`\s+`)

// Contact is a resolved recruiter/HM email.
type Contact struct {
	Name  string
	Email string
	Title string
}

// ResolveContact tries Hunter (and later Apollo) using a guessed domain from the job/company.
func ResolveContact(cfg *config.Config, company, jobURL string) (Contact, error) {
	domain := GuessDomain(company, jobURL)
	if domain == "" {
		return Contact{}, fmt.Errorf("could not guess domain for %q", company)
	}
	if cfg != nil && strings.TrimSpace(cfg.HunterKey) != "" {
		c, err := hunterDomainSearch(cfg.HunterKey, domain)
		if err == nil && c.Email != "" {
			return c, nil
		}
		if err != nil {
			return Contact{}, err
		}
	}
	return Contact{}, fmt.Errorf("no contact found for %s", domain)
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

type hunterResp struct {
	Data struct {
		Emails []struct {
			Value      string `json:"value"`
			FirstName  string `json:"first_name"`
			LastName   string `json:"last_name"`
			Position   string `json:"position"`
			Confidence int    `json:"confidence"`
			Type       string `json:"type"`
		} `json:"emails"`
	} `json:"data"`
}

func hunterDomainSearch(apiKey, domain string) (Contact, error) {
	u := fmt.Sprintf(
		"https://api.hunter.io/v2/domain-search?domain=%s&department=hr,recruiting&limit=10&api_key=%s",
		url.QueryEscape(domain),
		url.QueryEscape(apiKey),
	)
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return Contact{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return Contact{}, fmt.Errorf("hunter HTTP %d", resp.StatusCode)
	}
	var doc hunterResp
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return Contact{}, err
	}
	best := Contact{}
	bestScore := -1
	for _, e := range doc.Data.Emails {
		if strings.TrimSpace(e.Value) == "" {
			continue
		}
		score := e.Confidence
		pos := strings.ToLower(e.Position)
		if strings.Contains(pos, "recruit") || strings.Contains(pos, "talent") || strings.Contains(pos, "people") || strings.Contains(pos, "hr") {
			score += 20
		}
		if score > bestScore {
			bestScore = score
			name := strings.TrimSpace(e.FirstName + " " + e.LastName)
			best = Contact{Name: name, Email: e.Value, Title: e.Position}
		}
	}
	if best.Email == "" {
		return Contact{}, fmt.Errorf("hunter: no emails for %s", domain)
	}
	return best, nil
}
