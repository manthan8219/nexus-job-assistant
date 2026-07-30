package outreach

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// Queueable statuses that still need action.
// StatusFollowUpDue counts as pending work, but IsActionable gates it on the
// scheduled send time so future follow-ups don't fire early.
func IsPending(it Item) bool {
	switch it.Status {
	case StatusDraft, StatusReady, StatusOpened, StatusFailed, StatusFollowUpDue:
		return true
	default:
		return false
	}
}

// IsActionable reports whether the item can be fired at t.
// Follow-ups only become actionable once their scheduled time arrives.
func IsActionable(it Item, t time.Time) bool {
	if it.Status == StatusFollowUpDue {
		return IsFollowUpDue(it, t)
	}
	return IsPending(it)
}

// IsInProgress reports the pipeline is actively working on this item.
func IsInProgress(it Item) bool {
	return it.Status == StatusFinding || it.Status == StatusDrafting
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
	return NextPendingAt(items, ch, time.Now())
}

// NextPendingAt is NextPending evaluated against an explicit clock
// (follow-up due times are compared to t) — split out for tests.
func NextPendingAt(items []Item, ch Channel, t time.Time) (Item, bool) {
	var pend []Item
	for _, it := range FilterChannel(items, ch) {
		if IsActionable(it, t) {
			pend = append(pend, it)
		}
	}
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
		// Ready items (contact + reviewed draft) always go first.
		var ready []Item
		for _, it := range pend {
			if it.Status == StatusReady && strings.TrimSpace(it.ContactEmail) != "" {
				ready = append(ready, it)
			}
		}
		if len(ready) > 0 {
			return pick(ready), true
		}
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

// ExistsForJob reports whether an item for this job URL already exists on the channel.
func ExistsForJob(items []Item, ch Channel, jobURL string) bool {
	for _, it := range items {
		if it.Channel == ch && it.JobURL != "" && it.JobURL == jobURL {
			return true
		}
	}
	return false
}

// NewEmailStub creates a pipeline placeholder item (status finding) for a job.
// The background Worker picks it up: find contact → AI draft → review → ready.
func NewEmailStub(job JobRef, auto bool) Item {
	now := time.Now()
	return Item{
		ID:        uuid.NewString(),
		Channel:   ChannelEmail,
		JobURL:    job.URL,
		Company:   job.Company,
		Role:      job.Role,
		Provider:  job.Provider,
		Status:    StatusFinding,
		Auto:      auto,
		CreatedAt: now,
		UpdatedAt: now,
	}
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

// BuildEmailQueue creates email pipeline stubs (status finding) for Jobs tab
// jobs that don't have an email item yet. The background Worker walks each
// stub through contact-finding → AI drafting → review → ready.
func BuildEmailQueue(cfg *config.Config, jobs []store.Application, existing []Item, auto bool) (created []Item, skipped int, errs []string) {
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
		job := JobRef{URL: app.URL, Company: app.Company, Role: app.Role, Provider: app.Provider}
		item := NewEmailStub(job, auto)
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
	if err := Upsert(it); err != nil {
		return err
	}
	logAttempt(it, StatusOpened, nil)
	return nil
}
