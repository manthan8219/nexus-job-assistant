package inbox

import (
	"context"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

type fakeSource struct {
	msgs []outreach.Reply
}

func (f *fakeSource) FetchMessagesWithBodies(ctx context.Context, since time.Time, max int) ([]outreach.Reply, error) {
	return f.msgs, nil
}

type fakeLister struct {
	apps []store.Application
}

func (f *fakeLister) List() ([]store.Application, error) { return f.apps, nil }

func TestScanFiltersAndLinks(t *testing.T) {
	src := &fakeSource{msgs: []outreach.Reply{
		{From: "recruiter@databricks.com", Subject: "Interview invitation - Senior Backend", Date: time.Now(), MessageID: "m1", Body: "We would love to schedule an interview with the Databricks team."},
		{From: "news@somewhere.com", Subject: "Weekly digest", Date: time.Now(), Body: "hi there"},
		{From: "hr@acme.io", Subject: "Rejection", Date: time.Now(), Body: "Unfortunately we are not moving forward."},
	}}
	lister := &fakeLister{apps: []store.Application{{ID: 7, Company: "Databricks"}}}

	hs, err := Scan(context.Background(), 10, 10, src, lister)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(hs) != 2 {
		t.Fatalf("expected 2 highlights (newsletter filtered), got %d", len(hs))
	}
	var found bool
	for _, h := range hs {
		if h.Signal == SignalInterview {
			found = true
			if h.Company != "Databricks" || h.AppID != 7 {
				t.Errorf("interview highlight not linked: %+v", h)
			}
		}
	}
	if !found {
		t.Error("no interview highlight found")
	}
}

func TestScanNilSourceReturnsError(t *testing.T) {
	if _, err := Scan(context.Background(), 10, 10, nil, nil); err == nil {
		t.Error("expected error for nil source")
	}
}

func TestScanEmptyIsEmpty(t *testing.T) {
	hs, err := Scan(context.Background(), 10, 10, &fakeSource{}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(hs) != 0 {
		t.Errorf("expected no highlights, got %d", len(hs))
	}
}

func TestRootDomain(t *testing.T) {
	cases := []struct{ in, want string }{
		{"jane@acme.com", "acme.com"},
		{"jane@www.acme.com", "acme.com"},
		{"not-an-email", ""},
		{"x@", ""},
	}
	for _, c := range cases {
		if got := rootDomain(c.in); got != c.want {
			t.Errorf("rootDomain(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}
