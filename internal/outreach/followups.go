package outreach

import (
	"fmt"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// followUpDeltas[i] is the wait after the previous send before follow-up i+1.
// Touches land at +3, +7 and +14 days after the initial email — the cadence
// that roughly triples reply rates vs. single-shot outreach.
var followUpDeltas = []time.Duration{
	3 * 24 * time.Hour, // FU1: +3d
	4 * 24 * time.Hour, // FU2: +7d total
	7 * 24 * time.Hour, // FU3: +14d total
}

// MaxFollowUps is how many follow-up messages one sequence sends.
const MaxFollowUps = 3

// FollowUpsEnabled reports whether automatic follow-up sequences are active.
// They require outreach consent (they are outreach emails) and can be turned
// off in config; daily caps still apply to every follow-up send.
func FollowUpsEnabled(cfg *config.Config) bool {
	return cfg != nil && cfg.OutreachConsent && !cfg.OutreachFollowUpsOff
}

// IsFollowUpDue reports whether a followup_due item can be sent at t.
func IsFollowUpDue(it Item, t time.Time) bool {
	return it.Status == StatusFollowUpDue &&
		!it.NextSendAt.IsZero() && !t.Before(it.NextSendAt)
}

// ScheduleAfterSend transitions an item that was just sent. Email items with
// follow-ups enabled advance to the next step: status followup_due, a due
// date, and the next follow-up's rendered subject/body (so the queue stays
// reviewable before each send). Everything else closes as usual.
func ScheduleAfterSend(cfg *config.Config, it *Item, now time.Time) {
	it.NextSendAt = time.Time{}
	if it.Channel != ChannelEmail || !FollowUpsEnabled(cfg) {
		it.Status = StatusSent
		return
	}
	if it.FollowUpStep >= MaxFollowUps {
		it.Status = StatusSequenceDone
		return
	}
	it.FollowUpStep++
	it.Status = StatusFollowUpDue
	it.NextSendAt = now.Add(followUpDeltas[it.FollowUpStep-1])
	it.Subject, it.Body = FollowUpDraft(cfg, *it)
}

// MarkReplied stops the sequence for an item — no more follow-ups go out
// once a human has answered.
func MarkReplied(it *Item) {
	it.Status = StatusReplied
	it.NextSendAt = time.Time{}
}

// FollowUpDueIn describes how long until the next follow-up fires
// ("" when the item is not waiting).
func FollowUpDueIn(it Item, now time.Time) string {
	if it.Status != StatusFollowUpDue || it.NextSendAt.IsZero() {
		return ""
	}
	d := it.NextSendAt.Sub(now)
	if d <= 0 {
		return "due now"
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("due in %dh", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	return fmt.Sprintf("due in %dd", days)
}

// reSubject prefixes a subject for threading without doubling "Re:".
func reSubject(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "Re: following up"
	}
	if strings.HasPrefix(strings.ToLower(s), "re:") {
		return s
	}
	return "Re: " + s
}

// FollowUpDraft renders the subject + body for the item's current
// FollowUpStep (1..MaxFollowUps). Each step is a new short value-add
// message — never "just bumping this".
func FollowUpDraft(cfg *config.Config, it Item) (string, string) {
	step := it.FollowUpStep
	if step < 1 {
		step = 1
	}
	if step > MaxFollowUps {
		step = MaxFollowUps
	}
	job := JobRef{URL: it.JobURL, Company: it.Company, Role: it.Role, Provider: it.Provider}
	vars := Vars(cfg, job, it.ContactName, it.ContactEmail)

	subject := reSubject(it.Subject)
	var body string
	switch step {
	case 1:
		body = RenderTemplate(`Hi {{contact_name}},

Quick follow-up on my note about {{role}} at {{company}} — in case it got buried.

One thing I didn't mention: {{headline}}, and I'd genuinely love to help with the problems your team is working on.

Happy to share more, or a quick 15 minutes whenever suits you.

{{full_name}}`, vars)
	case 2:
		body = RenderTemplate(`Hi {{contact_name}},

Still interested in {{role}} at {{company}} — my background lines up well with what the role asks for, and I can ramp fast.

If you're not the right person for this, a pointer to whoever owns the role would be hugely appreciated.

Thanks,
{{full_name}}`, vars)
	default:
		body = RenderTemplate(`Hi {{contact_name}},

Closing the loop on {{role}} at {{company}} — I know inboxes get busy and timing isn't always right.

If the role is still open I'd welcome a conversation; if not, happy to stay in touch for the next one.

All the best,
{{full_name}}
{{linkedin}}`, vars)
	}
	return subject, body
}
