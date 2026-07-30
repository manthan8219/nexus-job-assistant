package outreach

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// ReplyReport summarizes one reply-check pass.
type ReplyReport struct {
	Scanned       int          // inbox messages scanned
	HumanReplies  []ReplyMatch // sequences stopped + outcome set
	Rejections    []ReplyMatch // ATS rejections recorded
	FollowUpsSent int          // due follow-ups fired after the check
	Errors        []string     // non-fatal problems (per-item)
}

// lookback returns how far back the inbox scan covers.
func lookback(cfg *config.Config) time.Duration {
	days := 45
	if cfg != nil && cfg.ReplyLookbackDays > 0 {
		days = cfg.ReplyLookbackDays
	}
	return time.Duration(days) * 24 * time.Hour
}

// CompanyDomains maps CompanyKey → email domain using saved contacts,
// so replies from addresses we never emailed can still be attributed
// to a company we applied to.
func CompanyDomains(st *store.Store) map[string]string {
	out := map[string]string{}
	if st == nil {
		return out
	}
	contacts, err := st.ListContacts()
	if err != nil {
		return out
	}
	for _, c := range contacts {
		key := store.CompanyKey(c.Company)
		d := strings.ToLower(strings.TrimSpace(c.Domain))
		if key == "" || d == "" || genericDomains[d] {
			continue
		}
		if _, have := out[key]; !have {
			out[key] = d
		}
	}
	return out
}

// RunReplyCheck fetches recent inbox messages, stops follow-up sequences
// that got a human reply, records application outcomes (replied/rejected),
// and fires a notification for every human reply. The fetcher is injected
// so tests stay hermetic; production wires NewGmailIMAPFetcher.
//
// logf may be nil; it receives one line per applied change (never content).
func RunReplyCheck(
	ctx context.Context,
	cfg *config.Config,
	st *store.Store,
	mn notifier.MultiNotifier,
	fetcher MessageFetcher,
	logf func(string),
) (*ReplyReport, error) {
	if fetcher == nil {
		return nil, fmt.Errorf("reply check: no inbox fetcher configured")
	}
	if st == nil {
		return nil, fmt.Errorf("reply check: store is nil")
	}
	log := func(format string, args ...any) {
		if logf != nil {
			logf(fmt.Sprintf(format, args...))
		}
	}

	since := time.Now().Add(-lookback(cfg))
	msgs, err := fetcher.FetchMessages(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("reply check: fetch inbox: %w", err)
	}
	items, err := Load()
	if err != nil {
		return nil, fmt.Errorf("reply check: load outreach items: %w", err)
	}
	apps, err := st.List()
	if err != nil {
		return nil, fmt.Errorf("reply check: list applications: %w", err)
	}

	rep := &ReplyReport{Scanned: len(msgs)}
	matches := MatchReplies(msgs, items, apps, CompanyDomains(st))
	for _, m := range matches {
		switch m.Kind {
		case MatchHumanReply:
			rep.HumanReplies = append(rep.HumanReplies, m)
			applyHumanReply(st, mn, m, items, log)
		case MatchATSRejection:
			rep.Rejections = append(rep.Rejections, m)
			if m.AppID != 0 {
				if err := st.SetOutcome(m.AppID, store.OutcomeRejected); err != nil {
					rep.Errors = append(rep.Errors, fmt.Sprintf("set rejected for %s: %v", m.Company, err))
					continue
				}
				log("rejection recorded: %s", m.Company)
			}
		}
	}
	return rep, nil
}

// applyHumanReply stops the outreach sequence (when there is one), sets the
// application outcome, and notifies. Errors are logged, not returned — one
// bad item must not lose the other replies.
func applyHumanReply(st *store.Store, mn notifier.MultiNotifier, m ReplyMatch, items []Item, log func(string, ...any)) {
	jobURL := ""
	role := ""
	if m.ItemID != "" {
		for i := range items {
			if items[i].ID != m.ItemID {
				continue
			}
			it := items[i]
			jobURL, role = it.JobURL, it.Role
			if it.Status == StatusReplied {
				return // already processed — don't re-notify
			}
			MarkReplied(&it)
			if err := Upsert(it); err != nil {
				log("stop sequence for %s: %v", m.Company, err)
			}
			break
		}
	}

	outcomeSet := false
	if m.AppID != 0 {
		outcomeSet = st.SetOutcome(m.AppID, store.OutcomeReplied) == nil
	} else if jobURL != "" {
		ok, err := st.SetOutcomeByURL(jobURL, store.OutcomeReplied)
		outcomeSet = err == nil && ok
	}
	if outcomeSet {
		log("reply → outcome=replied: %s (%s)", m.Company, m.Reply.From)
	}

	mn.Send(context.Background(), notifier.Event{
		Kind:         notifier.EventReplyReceived,
		ReplyFrom:    m.Reply.From,
		ReplySubject: m.Reply.Subject,
		Company:      m.Company,
		JobTitle:     role,
	})
}

// SendDueFollowUps fires every email follow-up whose scheduled time has
// arrived. SendEmail enforces the daily cap; hitting it stops the pass.
// Sequences for threads that already replied were closed by RunReplyCheck,
// so this never mails someone who just answered.
func SendDueFollowUps(cfg *config.Config, logf func(string)) (sent int, errs []string) {
	if !FollowUpsEnabled(cfg) {
		return 0, nil
	}
	items, err := Load()
	if err != nil {
		return 0, []string{fmt.Sprintf("load outreach items: %v", err)}
	}
	log := func(format string, args ...any) {
		if logf != nil {
			logf(fmt.Sprintf(format, args...))
		}
	}
	now := time.Now()
	for _, it := range items {
		if it.Channel != ChannelEmail || !IsFollowUpDue(it, now) {
			continue
		}
		if err := SendEmail(cfg, it); err != nil {
			errs = append(errs, fmt.Sprintf("follow-up #%d to %s: %v", it.FollowUpStep, it.Company, err))
			if strings.Contains(err.Error(), "daily email cap") {
				break // cap reached — remaining follow-ups fire on the next pass
			}
			continue
		}
		sent++
		log("follow-up #%d sent → %s", it.FollowUpStep, it.Company)
	}
	return sent, errs
}
