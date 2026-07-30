package outreach

import (
	"strings"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

var testNow = time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)

func consentCfg() *config.Config {
	return &config.Config{
		FirstName:       "Ada",
		LastName:        "Lovelace",
		OutreachConsent: true,
	}
}

func sentItem(step int) Item {
	return Item{
		ID:           "x",
		Channel:      ChannelEmail,
		Company:      "Acme",
		Role:         "Backend Engineer",
		ContactName:  "Grace",
		ContactEmail: "grace@acme.com",
		Subject:      "Quick note — Backend Engineer at Acme",
		Body:         "initial body",
		Status:       StatusSent,
		SentAt:       testNow,
		FollowUpStep: step,
	}
}

func TestFollowUpsEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"nil config", nil, false},
		{"no consent", &config.Config{}, false},
		{"consent on", consentCfg(), true},
		{"consent but opted out", &config.Config{OutreachConsent: true, OutreachFollowUpsOff: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FollowUpsEnabled(tt.cfg); got != tt.want {
				t.Errorf("FollowUpsEnabled() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestScheduleAfterSend(t *testing.T) {
	tests := []struct {
		name        string
		item        Item
		cfg         *config.Config
		wantStatus  Status
		wantStep    int
		wantDueDays int // days after now; 0 = expect no due date
	}{
		{"initial send schedules FU1 at +3d", sentItem(0), consentCfg(), StatusFollowUpDue, 1, 3},
		{"FU1 sent schedules FU2 at +4d", sentItem(1), consentCfg(), StatusFollowUpDue, 2, 4},
		{"FU2 sent schedules FU3 at +7d", sentItem(2), consentCfg(), StatusFollowUpDue, 3, 7},
		{"FU3 sent closes sequence", sentItem(3), consentCfg(), StatusSequenceDone, 3, 0},
		{"follow-ups off → plain sent", sentItem(0), &config.Config{OutreachConsent: true, OutreachFollowUpsOff: true}, StatusSent, 0, 0},
		{"no consent → plain sent", sentItem(0), &config.Config{}, StatusSent, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := tt.item
			ScheduleAfterSend(tt.cfg, &it, testNow)
			if it.Status != tt.wantStatus {
				t.Errorf("status = %q; want %q", it.Status, tt.wantStatus)
			}
			if it.FollowUpStep != tt.wantStep {
				t.Errorf("step = %d; want %d", it.FollowUpStep, tt.wantStep)
			}
			if tt.wantDueDays == 0 {
				if !it.NextSendAt.IsZero() {
					t.Errorf("NextSendAt = %v; want zero", it.NextSendAt)
				}
				return
			}
			wantDue := testNow.Add(time.Duration(tt.wantDueDays) * 24 * time.Hour)
			if !it.NextSendAt.Equal(wantDue) {
				t.Errorf("NextSendAt = %v; want %v", it.NextSendAt, wantDue)
			}
			// Next message is pre-rendered for review.
			if it.Subject == "" || it.Body == "" {
				t.Errorf("follow-up draft should be pre-rendered, got empty subject/body")
			}
			if it.Body == tt.item.Body {
				t.Errorf("follow-up body should differ from the previous message")
			}
		})
	}
}

func TestScheduleAfterSendLinkedInStaysSent(t *testing.T) {
	it := sentItem(0)
	it.Channel = ChannelLinkedIn
	ScheduleAfterSend(consentCfg(), &it, testNow)
	if it.Status != StatusSent {
		t.Errorf("LinkedIn item status = %q; want %q (no email follow-ups)", it.Status, StatusSent)
	}
}

func TestIsFollowUpDue(t *testing.T) {
	due := sentItem(1)
	due.Status = StatusFollowUpDue
	due.NextSendAt = testNow.Add(time.Hour)

	tests := []struct {
		name string
		item Item
		at   time.Time
		want bool
	}{
		{"before due time", due, testNow, false},
		{"exactly at due time", due, testNow.Add(time.Hour), true},
		{"after due time", due, testNow.Add(2 * time.Hour), true},
		{"plain sent item", sentItem(0), testNow.Add(48 * time.Hour), false},
		{"due status but no date", Item{Status: StatusFollowUpDue}, testNow, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFollowUpDue(tt.item, tt.at); got != tt.want {
				t.Errorf("IsFollowUpDue() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestNextPendingAtSkipsFutureFollowUps(t *testing.T) {
	future := sentItem(1)
	future.ID = "future"
	future.Status = StatusFollowUpDue
	future.NextSendAt = testNow.Add(72 * time.Hour)

	// Nothing actionable yet -> future follow-up must not fire early.
	if it, ok := NextPendingAt([]Item{future}, ChannelEmail, testNow); ok {
		t.Errorf("NextPendingAt picked %q before its due time; want none", it.ID)
	}
	// After the due time it becomes the pick.
	if it, ok := NextPendingAt([]Item{future}, ChannelEmail, testNow.Add(73*time.Hour)); !ok || it.ID != "future" {
		t.Errorf("NextPendingAt after due = %v, %v; want future, true", it.ID, ok)
	}
}

func TestNextPendingAtPrefersReadyOverFollowUp(t *testing.T) {
	dueFU := sentItem(1)
	dueFU.ID = "fu"
	dueFU.Status = StatusFollowUpDue
	dueFU.NextSendAt = testNow.Add(-time.Hour) // already due

	ready := sentItem(0)
	ready.ID = "ready"
	ready.Status = StatusReady

	it, ok := NextPendingAt([]Item{dueFU, ready}, ChannelEmail, testNow)
	if !ok || it.ID != "ready" {
		t.Errorf("NextPendingAt = %q, %v; want fresh ready email first", it.ID, ok)
	}
}

func TestMarkRepliedStopsSequence(t *testing.T) {
	it := sentItem(1)
	it.Status = StatusFollowUpDue
	it.NextSendAt = testNow.Add(72 * time.Hour)

	MarkReplied(&it)
	if it.Status != StatusReplied {
		t.Errorf("status = %q; want %q", it.Status, StatusReplied)
	}
	if !it.NextSendAt.IsZero() {
		t.Errorf("NextSendAt should be cleared, got %v", it.NextSendAt)
	}
	if _, ok := NextPendingAt([]Item{it}, ChannelEmail, testNow.Add(100*24*time.Hour)); ok {
		t.Errorf("replied item must never be picked again")
	}
}

func TestFollowUpDraftSteps(t *testing.T) {
	cfg := consentCfg()
	cfg.LinkedInID = "adalovelace"

	var bodies []string
	for step := 1; step <= MaxFollowUps; step++ {
		it := sentItem(step)
		subj, body := FollowUpDraft(cfg, it)
		if !strings.HasPrefix(strings.ToLower(subj), "re:") {
			t.Errorf("step %d subject %q should thread with Re:", step, subj)
		}
		if strings.Contains(body, "{{") {
			t.Errorf("step %d body has unrendered template var: %s", step, body)
		}
		if !strings.Contains(body, "Acme") || !strings.Contains(body, "Backend Engineer") {
			t.Errorf("step %d body missing company/role: %s", step, body)
		}
		if !strings.Contains(body, "Grace") {
			t.Errorf("step %d body missing contact name: %s", step, body)
		}
		bodies = append(bodies, body)
	}
	// Every step must be a distinct message.
	for i := range bodies {
		for j := i + 1; j < len(bodies); j++ {
			if bodies[i] == bodies[j] {
				t.Errorf("follow-up %d and %d have identical bodies", i+1, j+1)
			}
		}
	}
}

func TestReSubject(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Hello there", "Re: Hello there"},
		{"Re: Hello", "Re: Hello"},
		{"re: hello", "re: hello"},
		{"", "Re: following up"},
		{"  spaced  ", "Re: spaced"},
	}
	for _, tt := range tests {
		if got := reSubject(tt.in); got != tt.want {
			t.Errorf("reSubject(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func TestFollowUpDueIn(t *testing.T) {
	it := sentItem(1)
	it.Status = StatusFollowUpDue

	tests := []struct {
		name string
		due  time.Time
		want string
	}{
		{"overdue", testNow.Add(-time.Hour), "due now"},
		{"hours away", testNow.Add(5 * time.Hour), "due in 5h"},
		{"days away", testNow.Add(72 * time.Hour), "due in 3d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it.NextSendAt = tt.due
			if got := FollowUpDueIn(it, testNow); got != tt.want {
				t.Errorf("FollowUpDueIn() = %q; want %q", got, tt.want)
			}
		})
	}

	it.Status = StatusSent
	if got := FollowUpDueIn(it, testNow); got != "" {
		t.Errorf("non-followup item: FollowUpDueIn() = %q; want empty", got)
	}
}

func TestCountedAsSent(t *testing.T) {
	yes := []Status{StatusSent, StatusFollowUpDue, StatusSequenceDone, StatusReplied, StatusOpened}
	no := []Status{StatusDraft, StatusReady, StatusFailed, StatusSkipped, StatusFinding, StatusDrafting, StatusBounced}
	for _, s := range yes {
		if !countedAsSent(s) {
			t.Errorf("countedAsSent(%q) = false; want true (daily cap)", s)
		}
	}
	for _, s := range no {
		if countedAsSent(s) {
			t.Errorf("countedAsSent(%q) = true; want false", s)
		}
	}
}
