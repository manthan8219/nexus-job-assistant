package smartrecruiters

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// testProfile returns a minimal applicant profile plus a real resume file
// in a temp dir.
func testProfile(t *testing.T) provider.Profile {
	t.Helper()
	resumePath := filepath.Join(t.TempDir(), "resume.pdf")
	if err := os.WriteFile(resumePath, []byte("pdf-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	return provider.Profile{
		FirstName:  "Ada",
		LastName:   "Lovelace",
		Email:      "ada@example.com",
		Phone:      "+15551234567",
		LinkedInID: "adalovelace",
		ResumePath: resumePath,
	}
}

// testJob returns the job shape Search produces for a SmartRecruiters
// posting.
func testJob() provider.Job {
	return provider.Job{
		ID:       "posting-42",
		Title:    "Backend Engineer",
		Company:  "TestCo",
		Board:    "testco",
		URL:      "https://jobs.smartrecruiters.com/TestCo/744000000000000",
		Provider: "smartrecruiters",
	}
}

// capturedRequest is the raw apply request a mock board received.
type capturedRequest struct {
	req  *http.Request
	body []byte
}

// newMockSmartRecruiters returns a server that records every apply POST and
// answers with the given status code.
func newMockSmartRecruiters(t *testing.T, status int) (*httptest.Server, *capturedRequest) {
	t.Helper()
	got := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.req = r.Clone(r.Context())
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		got.body = body
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestApplyStatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		wantStatus string
	}{
		{"accepted", http.StatusOK, "applied"},
		{"accepted created", http.StatusCreated, "applied"},
		{"requires credentials 400", http.StatusBadRequest, "skipped"},
		{"requires credentials 401", http.StatusUnauthorized, "skipped"},
		{"requires credentials 403", http.StatusForbidden, "skipped"},
		{"validation error 422", http.StatusUnprocessableEntity, "failed"},
		{"server error", http.StatusInternalServerError, "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newMockSmartRecruiters(t, tt.status)
			c := &Client{http: srv.Client(), apiBase: srv.URL}

			res, err := c.Apply(context.Background(), testJob(), testProfile(t))
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if res.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q (reason: %s)", res.Status, tt.wantStatus, res.Reason)
			}
		})
	}
}

func TestApplyPostsJSONPayload(t *testing.T) {
	srv, got := newMockSmartRecruiters(t, http.StatusOK)
	c := &Client{http: srv.Client(), apiBase: srv.URL}

	res, err := c.Apply(context.Background(), testJob(), testProfile(t))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "applied" {
		t.Fatalf("status = %q, want applied", res.Status)
	}

	if got.req.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.req.Method)
	}
	if want := "/v1/companies/testco/postings/posting-42/candidates"; got.req.URL.Path != want {
		t.Errorf("path = %q, want %q", got.req.URL.Path, want)
	}

	var payload srCandidatePayload
	if err := json.Unmarshal(got.body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.FirstName != "Ada" || payload.LastName != "Lovelace" ||
		payload.Email != "ada@example.com" || payload.PhoneNumber != "+15551234567" {
		t.Errorf("payload identity = %+v, want Ada Lovelace / ada@example.com / +15551234567", payload)
	}
	if payload.Web.LinkedIn != "https://linkedin.com/in/adalovelace" {
		t.Errorf("linkedin = %q, want https://linkedin.com/in/adalovelace", payload.Web.LinkedIn)
	}
	if payload.Resume == nil {
		t.Fatal("resume must be attached when ResumePath is set")
	}
	if payload.Resume.FileName != "resume.pdf" {
		t.Errorf("resume filename = %q, want resume.pdf", payload.Resume.FileName)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.Resume.Data)
	if err != nil {
		t.Fatalf("resume data is not base64: %v", err)
	}
	if string(decoded) != "pdf-bytes" {
		t.Errorf("resume content = %q, want pdf-bytes", string(decoded))
	}
}

func TestApplyWithoutResumeOmitsResumeField(t *testing.T) {
	srv, got := newMockSmartRecruiters(t, http.StatusOK)
	c := &Client{http: srv.Client(), apiBase: srv.URL}

	profile := testProfile(t)
	profile.ResumePath = ""

	res, err := c.Apply(context.Background(), testJob(), profile)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "applied" {
		t.Fatalf("status = %q, want applied", res.Status)
	}
	var payload srCandidatePayload
	if err := json.Unmarshal(got.body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.Resume != nil {
		t.Error("resume must be omitted when ResumePath is empty")
	}
}

func TestApplyFailsOnUnreadableResume(t *testing.T) {
	srv, _ := newMockSmartRecruiters(t, http.StatusOK)
	c := &Client{http: srv.Client(), apiBase: srv.URL}

	profile := testProfile(t)
	profile.ResumePath = filepath.Join(t.TempDir(), "missing.pdf") // does not exist

	res, err := c.Apply(context.Background(), testJob(), profile)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "failed" {
		t.Fatalf("status = %q, want failed", res.Status)
	}
}

func TestApplyFailsOnTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := srv.URL
	srv.Close()

	c := &Client{http: srv.Client(), apiBase: closedURL}
	res, err := c.Apply(context.Background(), testJob(), testProfile(t))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "failed" {
		t.Fatalf("status = %q, want failed", res.Status)
	}
}
