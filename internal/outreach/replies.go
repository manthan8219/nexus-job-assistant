package outreach

import (
	"context"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// Reply is one inbox message that may answer an outreach email or an
// application. From is the bare sender address, lowercased.
type Reply struct {
	From     string
	FromName string
	Subject  string
	Date     time.Time
}

// MessageFetcher abstracts the inbox source (Gmail IMAP in production,
// fakes in tests) so reply-check logic stays hermetic.
type MessageFetcher interface {
	FetchMessages(ctx context.Context, since time.Time) ([]Reply, error)
}

// MatchKind distinguishes a human reply from an automated ATS rejection.
type MatchKind int

const (
	// MatchHumanReply is a person answering our outreach/application.
	MatchHumanReply MatchKind = iota
	// MatchATSRejection is an automated "unfortunately…" notice from a job board.
	MatchATSRejection
)

// ReplyMatch ties one inbox message to the outreach item and/or the
// application it belongs to.
type ReplyMatch struct {
	Reply   Reply
	Kind    MatchKind
	ItemID  string // outreach item matched ("" when none)
	Company string
	AppID   int64 // application matched (0 when none)
}

// genericDomains must never be used for domain-based matching — half the
// internet shares them.
var genericDomains = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "yahoo.com": true,
	"outlook.com": true, "hotmail.com": true, "live.com": true,
	"icloud.com": true, "me.com": true, "aol.com": true,
	"proton.me": true, "protonmail.com": true, "pm.me": true,
}

// atsSenderDomains are job-board mail domains whose messages are automated,
// not human replies.
var atsSenderDomains = []string{
	"greenhouse.io", "lever.co", "ashbyhq.com", "workable.com",
	"smartrecruiters.com", "myworkday.com", "myworkdayjobs.com",
	"bamboohr.com", "jobvite.com", "recruitee.com", "teamtailor.com",
	"pinpointhq.com", "personio.de", "personio.com",
}

// rejectionPhrases mark an automated notice as a rejection.
var rejectionPhrases = []string{
	"unfortunately", "regret to inform", "not moving forward",
	"will not be moving forward", "decided not to proceed",
	"other candidates", "not been selected", "not selected",
	"unable to move forward", "position has been filled",
	"no longer be considered", "not be proceeding",
}

// domainOf extracts the lowercased domain of an email address.
func domainOf(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 && i+1 < len(addr) {
		return strings.ToLower(strings.TrimSpace(addr[i+1:]))
	}
	return ""
}

func isATSSender(domain string) bool {
	for _, d := range atsSenderDomains {
		if domain == d || strings.HasSuffix(domain, "."+d) {
			return true
		}
	}
	return false
}

func looksLikeRejection(subject string) bool {
	s := strings.ToLower(subject)
	for _, p := range rejectionPhrases {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// companyInText reports whether the company name appears in free text.
// Matches the full name or the first word of a multi-word name
// ("Acme Corp" → "acme"). Single-word names under 3 chars never match —
// too noisy ("Go" inside "go fishing").
func companyInText(company, text string) bool {
	name := strings.ToLower(strings.TrimSpace(company))
	if name == "" || (!strings.Contains(name, " ") && len(name) < 3) {
		return false
	}
	text = strings.ToLower(text)
	if strings.Contains(text, name) {
		return true
	}
	first := strings.Fields(name)
	return len(first) > 1 && len(first[0]) >= 4 && strings.Contains(text, first[0])
}

// MatchReplies pairs inbox messages with outreach items and applications.
//
// Human replies match by exact contact email first, then by the contact's
// company domain (never a generic provider domain), then by a company's
// known domain from the contacts DB. ATS auto-mail is classified separately:
// rejection subjects are matched to an application by company name so the
// pipeline records the outcome. companyDomains maps CompanyKey → domain.
func MatchReplies(msgs []Reply, items []Item, apps []store.Application, companyDomains map[string]string) []ReplyMatch {
	var out []ReplyMatch
	for _, msg := range msgs {
		from := strings.ToLower(strings.TrimSpace(msg.From))
		if from == "" {
			continue
		}
		domain := domainOf(from)

		if isATSSender(domain) {
			if m, ok := matchATSRejection(msg, apps); ok {
				out = append(out, m)
			}
			continue
		}

		// Human reply: exact contact address wins, then contact's domain.
		m := matchContactReply(msg, items, from, domain)
		if m != nil {
			out = append(out, *m)
			continue
		}
		// A recruiter writing from an address we never emailed: fall back to
		// the company's known domain against applied applications.
		if genericDomains[domain] || domain == "" {
			continue
		}
		for _, a := range apps {
			if a.Status != store.StatusApplied || a.Outcome != store.OutcomeNone {
				continue
			}
			if companyDomains[store.CompanyKey(a.Company)] == domain {
				out = append(out, ReplyMatch{
					Reply: msg, Kind: MatchHumanReply,
					Company: a.Company, AppID: a.ID,
				})
				break
			}
		}
	}
	return out
}

// matchATSRejection classifies automated ATS mail; only rejection notices
// that name a company we applied to produce a match.
func matchATSRejection(msg Reply, apps []store.Application) (ReplyMatch, bool) {
	if !looksLikeRejection(msg.Subject) {
		return ReplyMatch{}, false // confirmations, newsletters etc.
	}
	for _, a := range apps {
		if a.Status == store.StatusApplied && a.Outcome == store.OutcomeNone && companyInText(a.Company, msg.Subject) {
			return ReplyMatch{
				Reply: msg, Kind: MatchATSRejection,
				Company: a.Company, AppID: a.ID,
			}, true
		}
	}
	return ReplyMatch{}, false
}

// matchContactReply finds the outreach item this message answers.
// Items that already ended (replied) are skipped — no double-processing.
func matchContactReply(msg Reply, items []Item, from, domain string) *ReplyMatch {
	for i := range items {
		it := &items[i]
		if it.Channel != ChannelEmail || !countedAsSent(it.Status) || it.Status == StatusReplied {
			continue
		}
		contact := strings.ToLower(strings.TrimSpace(it.ContactEmail))
		if contact == "" {
			continue
		}
		contactDomain := domainOf(contact)
		if from == contact || (domain != "" && !genericDomains[domain] && domain == contactDomain) {
			return &ReplyMatch{
				Reply: msg, Kind: MatchHumanReply,
				ItemID: it.ID, Company: it.Company,
			}
		}
	}
	return nil
}
