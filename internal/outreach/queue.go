package outreach

import (
	"fmt"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/config"
	"github.com/manthanmanthan/nexus/internal/store"
)

// Queueable statuses that still need action.
func IsPending(it Item) bool {
	switch it.Status {
	case StatusDraft, StatusReady, StatusOpened, StatusFailed:
		return true
	default:
		return false
	}
}

func FilterChannel(items []Item, ch Channel) []Item {
	var out []Item
	for _, it := range items {
		if it.Channel == ch {
			out = append(out, it)
		}
	}
	return out
}

func Pending(items []Item, ch Channel) []Item {
	var out []Item
	for _, it := range FilterChannel(items, ch) {
		if IsPending(it) {
			out = append(out, it)
		}
	}
	return out
}

// NextPending returns the next item to act on (oldest first among pending).
// For email, prefers items that already have a ContactEmail.
func NextPending(items []Item, ch Channel) (Item, bool) {
	pend := Pending(items, ch)
	if len(pend) == 0 {
		return Item{}, false
	}
	pick := func(list []Item) Item {
		best := list[0]
		for _, it := range list[1:] {
			if it.CreatedAt.Before(best.CreatedAt) {
				best = it
			}
		}
		return best
	}
	if ch == ChannelEmail {
		var withMail []Item
		for _, it := range pend {
			if strings.TrimSpace(it.ContactEmail) != "" {
				withMail = append(withMail, it)
			}
		}
		if len(withMail) > 0 {
			return pick(withMail), true
		}
	}
	return pick(pend), true
}

// BuildEmailQueue creates email drafts for Jobs tab jobs (skips URLs already queued/sent).
func BuildEmailQueue(cfg *config.Config, jobs []store.Application, existing []Item, resolve bool) (created []Item, skipped int, errs []string) {
	have := map[string]bool{}
	for _, it := range existing {
		if it.Channel == ChannelEmail && it.JobURL != "" {
			have[it.JobURL] = true
		}
	}
	for _, app := range jobs {
		if app.URL == "" || have[app.URL] {
			skipped++
			continue
		}
		// Prefer applied; still allow others so dry-runs can outreach.
		job := JobRef{URL: app.URL, Company: app.Company, Role: app.Role, Provider: app.Provider}
		name, email := "", ""
		if resolve {
			if c, err := ResolveContact(cfg, app.Company, app.URL); err == nil {
				name, email = c.Name, c.Email
			} else if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", app.Company, err))
			}
		}
		item := NewEmailDraft(cfg, job, name, email)
		if err := Upsert(item); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		created = append(created, item)
		have[app.URL] = true
	}
	return created, skipped, errs
}

// BuildLinkedInQueue creates LinkedIn outreach items for Jobs tab jobs.
func BuildLinkedInQueue(cfg *config.Config, jobs []store.Application, existing []Item) (created []Item, skipped int, errs []string) {
	have := map[string]bool{}
	for _, it := range existing {
		if it.Channel == ChannelLinkedIn && it.JobURL != "" {
			have[it.JobURL] = true
		}
	}
	for _, app := range jobs {
		if app.URL == "" || have[app.URL] {
			skipped++
			continue
		}
		job := JobRef{URL: app.URL, Company: app.Company, Role: app.Role, Provider: app.Provider}
		item := NewLinkedInDraft(cfg, job, "", "")
		item.Status = StatusReady
		item.LinkedInURL = LinkedInPeopleSearchURL(app.Company)
		if err := Upsert(item); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		created = append(created, item)
		have[app.URL] = true
	}
	return created, skipped, errs
}

// ExecuteEmail sends one email item (SMTP).
func ExecuteEmail(cfg *config.Config, it Item) error {
	if strings.TrimSpace(it.ContactEmail) == "" {
		return fmt.Errorf("no contact email — add Hunter key or set To: manually")
	}
	return SendEmail(cfg, it)
}

// ExecuteLinkedIn opens the browser for LinkedIn outreach and records status.
func ExecuteLinkedIn(cfg *config.Config, it Item, markSent bool) error {
	if cfg != nil && !cfg.OutreachConsent {
		return fmt.Errorf("outreach consent required")
	}
	max := 10
	if cfg != nil && cfg.MaxLinkedInPerDay > 0 {
		max = cfg.MaxLinkedInPerDay
	}
	if n, err := CountSentToday(ChannelLinkedIn); err == nil && n >= max {
		return fmt.Errorf("daily LinkedIn cap reached (%d)", max)
	}
	company := it.Company
	u := it.LinkedInURL
	if u == "" {
		u = LinkedInPeopleSearchURL(company)
	}
	if err := OpenBrowser(u); err != nil {
		return err
	}
	it.LinkedInURL = u
	it.UpdatedAt = time.Now()
	// Count toward daily cap once the browser action has run.
	it.Status = StatusSent
	it.SentAt = time.Now()
	if !markSent {
		// Still "sent" for queue progress; Opened kept for history display if needed later.
		_ = StatusOpened
	}
	return Upsert(it)
}
