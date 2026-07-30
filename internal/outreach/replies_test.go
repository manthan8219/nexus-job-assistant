package outreach

import (
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func appliedApp(id int64, company string) store.Application {
	return store.Application{
		ID:        id,
		Provider:  "greenhouse",
		Company:   company,
		Role:      "Backend Engineer",
		URL:       "https://boards.greenhouse.io/x/jobs/1",
		Status:    store.StatusApplied,
		AppliedAt: time.Now(),
	}
}

func emailItem(id, contactEmail, company string) Item {
	return Item{
		ID:           id,
		Channel:      ChannelEmail,
		Company:      company,
		Role:         "Backend Engineer",
		JobURL:       "https://boards.greenhouse.io/x/jobs/1",
		ContactEmail: contactEmail,
		Status:       StatusFollowUpDue,
		SentAt:       time.Now(),
	}
}

func TestMatchReplies(t *testing.T) {
	items := []Item{
		emailItem("it1", "jane@acme.com", "Acme"),
		{ID: "li1", Channel: ChannelLinkedIn, Company: "LinkedInCo", Status: StatusSent, SentAt: time.Now()},
	}
	apps := []store.Application{
		appliedApp(1, "Acme"),
		appliedApp(2, "Globex Inc"),
	}
	domains := map[string]string{"globex inc": "globex.com"}

	tests := []struct {
		name       string
		msg        Reply
		wantMatch  bool
		wantKind   MatchKind
		wantItemID string
		wantAppID  int64
	}{
		{
			name:      "exact contact email matches item",
			msg:       Reply{From: "jane@acme.com", Subject: "Re: hello"},
			wantMatch: true, wantKind: MatchHumanReply, wantItemID: "it1",
		},
		{
			name:      "same company domain, different person",
			msg:       Reply{From: "bob@acme.com", Subject: "Re: hello"},
			wantMatch: true, wantKind: MatchHumanReply, wantItemID: "it1",
		},
		{
			name:      "generic domain never domain-matches",
			msg:       Reply{From: "someone@gmail.com", Subject: "Re: hello"},
			wantMatch: false,
		},
		{
			name:      "linkedin channel items are ignored",
			msg:       Reply{From: "recruiter@linkedinco.com", Subject: "Re: hi"},
			wantMatch: false,
		},
		{
			name:      "ATS rejection matched to company in subject",
			msg:       Reply{From: "no-reply@greenhouse.io", Subject: "Unfortunately, Acme will not be moving forward"},
			wantMatch: true, wantKind: MatchATSRejection, wantAppID: 1,
		},
		{
			name:      "ATS confirmation is not a rejection",
			msg:       Reply{From: "no-reply@greenhouse.io", Subject: "Thank you for applying to Acme"},
			wantMatch: false,
		},
		{
			name:      "ATS rejection without known company is ignored",
			msg:       Reply{From: "no-reply@lever.co", Subject: "Unfortunately we regret to inform you"},
			wantMatch: false,
		},
		{
			name:      "rejection matched by first word of multi-word company",
			msg:       Reply{From: "no-reply@lever.co", Subject: "Globex update: unfortunately, other candidates"},
			wantMatch: true, wantKind: MatchATSRejection, wantAppID: 2,
		},
		{
			name:      "known company domain matches applied app without item",
			msg:       Reply{From: "hiring.manager@globex.com", Subject: "chat this week?"},
			wantMatch: true, wantKind: MatchHumanReply, wantAppID: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchReplies([]Reply{tt.msg}, items, apps, domains)
			if !tt.wantMatch {
				if len(got) != 0 {
					t.Fatalf("expected no match, got %+v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("expected 1 match, got %d: %+v", len(got), got)
			}
			m := got[0]
			if m.Kind != tt.wantKind {
				t.Errorf("Kind = %v; want %v", m.Kind, tt.wantKind)
			}
			if m.ItemID != tt.wantItemID {
				t.Errorf("ItemID = %q; want %q", m.ItemID, tt.wantItemID)
			}
			if m.AppID != tt.wantAppID {
				t.Errorf("AppID = %d; want %d", m.AppID, tt.wantAppID)
			}
		})
	}
}

func TestMatchRepliesSkipsRepliedItems(t *testing.T) {
	it := emailItem("it1", "jane@acme.com", "Acme")
	it.Status = StatusReplied // sequence already stopped
	got := MatchReplies(
		[]Reply{{From: "jane@acme.com", Subject: "Re: hello"}},
		[]Item{it},
		[]store.Application{appliedApp(1, "Acme")},
		nil,
	)
	if len(got) != 0 {
		t.Errorf("already-replied item must not match again, got %+v", got)
	}
}

func TestMatchRepliesSkipsNonAppliedApps(t *testing.T) {
	rejected := appliedApp(1, "Acme")
	rejected.Outcome = store.OutcomeRejected
	msg := Reply{From: "no-reply@greenhouse.io", Subject: "Unfortunately, Acme will not be moving forward"}
	got := MatchReplies([]Reply{msg}, nil, []store.Application{rejected}, nil)
	if len(got) != 0 {
		t.Errorf("app with an outcome already set must not be re-matched, got %+v", got)
	}
}

func TestCompanyInText(t *testing.T) {
	tests := []struct {
		company, text string
		want          bool
	}{
		{"Acme", "update from acme about your application", true},
		{"Globex Inc", "globex update", true},
		{"Globex Inc", "GLOBEX INC — decision", true},
		{"Initech", "unrelated message", false},
		{"", "anything", false},
		{"Go", "go fishing", false},
	}
	for _, tt := range tests {
		if got := companyInText(tt.company, tt.text); got != tt.want {
			t.Errorf("companyInText(%q, %q) = %v; want %v", tt.company, tt.text, got, tt.want)
		}
	}
}

func TestLooksLikeRejection(t *testing.T) {
	yes := []string{
		"Unfortunately, we will not be moving forward",
		"We regret to inform you",
		"Update: other candidates selected",
		"The position has been filled",
	}
	no := []string{
		"Thank you for applying",
		"Interview invitation",
		"Your application was received",
	}
	for _, s := range yes {
		if !looksLikeRejection(s) {
			t.Errorf("looksLikeRejection(%q) = false; want true", s)
		}
	}
	for _, s := range no {
		if looksLikeRejection(s) {
			t.Errorf("looksLikeRejection(%q) = true; want false", s)
		}
	}
}

func TestNewGmailIMAPFetcher(t *testing.T) {
	if NewGmailIMAPFetcher(nil) != nil {
		t.Errorf("nil config should give nil fetcher")
	}
	if NewGmailIMAPFetcher(&config.Config{Email: "u@gmail.com"}) != nil {
		t.Errorf("missing app password should give nil fetcher")
	}
	f := NewGmailIMAPFetcher(&config.Config{Email: "u@gmail.com", GmailAppPassword: "abcd"})
	if f == nil || f.User != "u@gmail.com" {
		t.Errorf("configured should give fetcher with user, got %+v", f)
	}
}

func TestCompanyDomainsNilStore(t *testing.T) {
	if got := CompanyDomains(nil); len(got) != 0 {
		t.Errorf("nil store should give empty map, got %v", got)
	}
}
