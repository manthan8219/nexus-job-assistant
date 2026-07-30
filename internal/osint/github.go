package osint

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// githubSearch looks up public members of a GitHub organisation.
// It tries a few org-name guesses derived from the company name and domain.
func (f *Finder) githubSearch(ctx context.Context, company, domain string) ([]Contact, error) {
	// Build candidate org slugs
	slugs := githubSlugs(company, domain)

	for _, slug := range slugs {
		contacts, err := fetchGitHubOrgMembers(ctx, f.http, slug, company, domain)
		if err == nil && len(contacts) > 0 {
			return contacts, nil
		}
	}
	return nil, nil
}

// githubSlugs generates likely GitHub org names from company + domain.
func githubSlugs(company, domain string) []string {
	seen := map[string]bool{}
	var slugs []string
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.ReplaceAll(s, " ", "-")
		if s != "" && !seen[s] {
			seen[s] = true
			slugs = append(slugs, s)
		}
	}

	// From domain: "linear.app" → "linear", "deshaw.com" → "deshaw"
	if domain != "" {
		parts := strings.Split(domain, ".")
		if len(parts) >= 2 {
			add(parts[0])
		}
	}

	// From company name: "D.E. Shaw" → "de-shaw", "Linear" → "linear"
	clean := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' {
			return r
		}
		return ' '
	}, company)
	add(clean)

	// Words joined with dash: "De Shaw Group" → "de-shaw-group"
	words := strings.Fields(clean)
	if len(words) > 1 {
		add(strings.Join(words, "-"))
		add(words[0]) // first word only
	}

	return slugs
}

type ghMember struct {
	Login string `json:"login"`
}

type ghUser struct {
	Login   string `json:"login"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Company string `json:"company"`
	Bio     string `json:"bio"`
	HTMLURL string `json:"html_url"`
}

func fetchGitHubOrgMembers(ctx context.Context, client *http.Client, org, company, domain string) ([]Contact, error) {
	// Verify the org exists first
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.github.com/orgs/%s", org), nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("org not found: %s (status %d)", org, resp.StatusCode)
	}

	// Fetch members (up to 100)
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.github.com/orgs/%s/members?per_page=100&filter=all", org), nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err = client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("members fetch failed: %d", resp.StatusCode)
	}

	var members []ghMember
	if err := json.NewDecoder(resp.Body).Decode(&members); err != nil {
		return nil, err
	}

	// Fetch each member's profile — cap at 30 to avoid rate limits
	if len(members) > 30 {
		members = members[:30]
	}

	now := time.Now()
	var contacts []Contact
	for _, m := range members {
		select {
		case <-ctx.Done():
			return contacts, nil
		default:
		}

		user, err := fetchGitHubUser(ctx, client, m.Login)
		if err != nil {
			continue
		}

		if user.Name == "" && user.Email == "" {
			continue
		}

		ghNote := "github.com/" + user.Login
		title := cleanGitHubCompany(user.Company)

		personalEmail := strings.ToLower(strings.TrimSpace(user.Email))

		// Generate work email from name + domain
		var workEmail string
		if domain != "" && user.Name != "" {
			parts := strings.Fields(strings.ToLower(user.Name))
			if len(parts) >= 2 {
				workEmail = fmt.Sprintf("%s.%s@%s", parts[0], parts[len(parts)-1], domain)
			}
		}

		// One contact per person. Prefer personal email as primary (user published it);
		// put work email in Notes so it's visible but not a separate row.
		var primaryEmail, emailType string
		var conf int
		var noteExtra string

		if personalEmail != "" && isPersonalEmailDomain(personalEmail) {
			primaryEmail = personalEmail
			emailType = "personal"
			conf = 85
			if workEmail != "" {
				noteExtra = " | work: " + workEmail
			}
		} else if personalEmail != "" {
			// GitHub email is already a work/custom domain email
			primaryEmail = personalEmail
			emailType = "work"
			conf = 80
		} else if workEmail != "" {
			primaryEmail = workEmail
			emailType = "work"
			conf = 55
		} else {
			continue // no email at all — skip
		}

		contacts = append(contacts, Contact{
			Company:    company,
			Domain:     domain,
			Name:       user.Name,
			Title:      title,
			Email:      primaryEmail,
			EmailType:  emailType,
			Source:     "github",
			Confidence: conf,
			FoundAt:    now,
			Notes:      ghNote + noteExtra,
		})
	}

	return contacts, nil
}

func fetchGitHubUser(ctx context.Context, client *http.Client, login string) (ghUser, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/users/"+login, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return ghUser{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ghUser{}, fmt.Errorf("user %s: %d", login, resp.StatusCode)
	}
	var u ghUser
	return u, json.NewDecoder(resp.Body).Decode(&u)
}

func cleanGitHubCompany(s string) string {
	s = strings.TrimPrefix(s, "@")
	return strings.TrimSpace(s)
}

var personalDomains = map[string]bool{
	"gmail.com": true, "googlemail.com": true,
	"yahoo.com": true, "yahoo.in": true, "yahoo.co.uk": true,
	"hotmail.com": true, "hotmail.in": true,
	"outlook.com": true, "outlook.in": true,
	"live.com": true, "live.in": true,
	"icloud.com": true, "me.com": true, "mac.com": true,
	"protonmail.com": true, "proton.me": true,
	"tutanota.com": true, "fastmail.com": true,
	"zoho.com": true, "rediffmail.com": true,
	"aol.com": true, "inbox.com": true, "mail.com": true,
	"yandex.com": true, "yandex.ru": true,
}

// isPersonalEmailDomain returns true for well-known free/personal email providers.
func isPersonalEmailDomain(email string) bool {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return false
	}
	return personalDomains[strings.ToLower(parts[1])]
}
