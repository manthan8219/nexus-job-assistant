package outreach

import (
	"context"
	"fmt"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/osint"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// hrTitleWords mark a contact as recruiting-related (ranked higher).
var hrTitleWords = []string{
	"recruit", "talent", "people", "hr", "human resources",
	"hiring", "sourc", "staffing", "people ops",
}

// FindBestContact searches every available source for an HR / careers contact
// at the company, persists everything found to the contacts store, and returns
// the single best address to email.
//
// Sources (via internal/osint): Hunter.io, Apollo.io, GitHub org members, the
// local OSINT scraper service, and careers@/hr@/jobs@ pattern fallbacks.
// The best contact is chosen by RankContacts (HR-title relevance + confidence).
func FindBestContact(ctx context.Context, cfg *config.Config, st *store.Store, company, jobURL string) (Contact, []osint.Contact, error) {
	company = strings.TrimSpace(company)
	if company == "" {
		return Contact{}, nil, fmt.Errorf("no company name")
	}

	// Domain: reuse one we discovered earlier, else guess from the job URL /
	// company name.
	domain := ""
	if st != nil {
		if d, err := st.DomainForCompany(company); err == nil {
			domain = d
		}
	}
	if domain == "" {
		domain = GuessDomain(company, jobURL)
	}
	if domain == "" {
		return Contact{}, nil, fmt.Errorf("could not determine email domain for %q", company)
	}

	hunterKey, apolloKey := "", ""
	smtpVerify := false
	if cfg != nil {
		hunterKey = strings.TrimSpace(cfg.HunterKey)
		apolloKey = strings.TrimSpace(cfg.ApolloKey)
		smtpVerify = cfg.OutreachSMTPVerify
	}
	finder := osint.NewFinder(hunterKey, apolloKey)
	finder.Verify = smtpVerify

	result := finder.Search(ctx, company, domain)

	// Persist everything we found so the Contacts tab fills up too (best effort).
	if st != nil {
		for _, c := range result.Contacts {
			_ = st.SaveContact(c)
		}
	}

	ranked := RankContacts(result.Contacts)
	if len(ranked) == 0 {
		err := fmt.Errorf("no contact found for %s (%s)", company, domain)
		if len(result.Errors) > 0 {
			err = fmt.Errorf("%w — sources: %s", err, strings.Join(result.Errors, "; "))
		}
		return Contact{}, nil, err
	}
	best := ranked[0]
	return Contact{Name: best.Name, Email: best.Email, Title: best.Title}, ranked, nil
}

// RankContacts sorts contacts best-first for cold outreach: recruiting-related
// titled people win, then high-confidence addresses, then named people, then
// generic pattern inboxes (careers@ first — most likely monitored).
func RankContacts(contacts []osint.Contact) []osint.Contact {
	out := make([]osint.Contact, len(contacts))
	copy(out, contacts)
	// stable insertion sort on descending score — lists are small (<40)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && contactScore(out[j]) > contactScore(out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func contactScore(c osint.Contact) int {
	score := c.Confidence
	title := strings.ToLower(c.Title + " " + c.Name)
	for _, w := range hrTitleWords {
		if strings.Contains(title, w) {
			score += 30
			break
		}
	}
	// A real named person beats a generic inbox at similar confidence.
	if c.EmailType != "pattern" && strings.TrimSpace(c.Name) != "" {
		score += 10
	}
	// Pattern inboxes: keep the generatePatterns preference order
	// (careers@ > recruiting@ > talent@ > jobs@ > hr@ > …).
	if c.EmailType == "pattern" {
		score += patternInboxBonus(c.Email)
	}
	return score
}

// patternInboxBonus orders generic inboxes by likelihood a human monitors them.
func patternInboxBonus(email string) int {
	local := strings.ToLower(strings.SplitN(email, "@", 2)[0])
	switch local {
	case "careers":
		return 7
	case "recruiting":
		return 6
	case "talent":
		return 5
	case "jobs":
		return 4
	case "hr":
		return 3
	case "people":
		return 2
	case "hiring":
		return 1
	default:
		return 0
	}
}
