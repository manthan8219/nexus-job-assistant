package recruitee

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// testProfile returns a minimal applicant profile plus a real resume file
// in a temp dir (hermetic — no network, no ~/.nexus writes).
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
		ResumePath: resumePath,
	}
}

// testJob returns the job shape Search produces for a Recruitee offer.
func testJob() provider.Job {
	return provider.Job{
		Title:    "Backend Engineer",
		Company:  "TestCo",
		Board:    "testco",
		URL:      "https://testco.recruitee.com/o/backend-engineer",
		Provider: "recruitee",
	}
}

// capturedApplication is the raw apply request a mock board received.
type capturedApplication struct {
	method      string
	path        string
	contentType string
	body        []byte
	hits        int
}

// newMockRecruitee returns a board that records every apply request and
// answers with the given status code.
func newMockRecruitee(t *testing.T, status int) (*httptest.Server, *capturedApplication) {
	t.Helper()
	got := &capturedApplication{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.hits++
		got.method = r.Method
		got.path = r.URL.Path
		got.contentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		got.body = body
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

// parseForm re-reads a captured multipart body using the boundary from the
// captured Content-Type header.
func parseForm(t *testing.T, got *capturedApplication) *multipart.Form {
	t.Helper()
	_, params, err := mime.ParseMediaType(got.contentType)
	if err != nil {
		t.Fatalf("parse content-type %q: %v", got.contentType, err)
	}
	mr := multipart.NewReader(bytes.NewReader(got.body), params["boundary"])
	form, err := mr.ReadForm(10 << 20)
	if err != nil {
		t.Fatalf("read multipart form: %v", err)
	}
	return form
}

func TestApplyStatusMapping(t *testing.T) {
	resume := testProfile(t)

	tests := []struct {
		name       string
		status     int
		wantStatus string
	}{
		{"accepted", http.StatusOK, "applied"},
		{"accepted created", http.StatusCreated, "applied"},
		{"offer gone", http.StatusNotFound, "skipped"},
		{"form rejected", http.StatusUnprocessableEntity, "skipped"},
		{"server error", http.StatusInternalServerError, "failed"},
		{"rate limited", http.StatusTooManyRequests, "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, got := newMockRecruitee(t, tt.status)
			c := &Client{http: srv.Client(), applyHost: srv.URL}

			res, err := c.Apply(context.Background(), testJob(), resume)
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if res.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q (reason: %s)", res.Status, tt.wantStatus, res.Reason)
			}
			if got.hits != 1 {
				t.Errorf("request hits = %d, want 1", got.hits)
			}
		})
	}
}

func TestApplyPostsWellFormedMultipart(t *testing.T) {
	srv, got := newMockRecruitee(t, http.StatusOK)
	c := &Client{http: srv.Client(), applyHost: srv.URL}

	res, err := c.Apply(context.Background(), testJob(), testProfile(t))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "applied" {
		t.Fatalf("status = %q, want applied", res.Status)
	}

	if got.method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.method)
	}
	if want := "/api/offers/backend-engineer/candidates"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	if !strings.HasPrefix(got.contentType, "multipart/form-data") {
		t.Errorf("content-type = %q, want multipart/form-data", got.contentType)
	}

	form := parseForm(t, got)
	if gotName := form.Value["candidate[name]"]; len(gotName) != 1 || gotName[0] != "Ada Lovelace" {
		t.Errorf("candidate[name] = %v, want [Ada Lovelace]", gotName)
	}
	if gotEmail := form.Value["candidate[email]"]; len(gotEmail) != 1 || gotEmail[0] != "ada@example.com" {
		t.Errorf("candidate[email] = %v, want [ada@example.com]", gotEmail)
	}
	if gotPhone := form.Value["candidate[phone]"]; len(gotPhone) != 1 || gotPhone[0] != "+15551234567" {
		t.Errorf("candidate[phone] = %v, want [+15551234567]", gotPhone)
	}

	fileParts := form.File["candidate[cv]"]
	if len(fileParts) != 1 {
		t.Fatalf("candidate[cv] file parts = %d, want 1", len(fileParts))
	}
	if fileParts[0].Filename != "resume.pdf" {
		t.Errorf("cv filename = %q, want resume.pdf", fileParts[0].Filename)
	}
	f, err := fileParts[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "pdf-bytes" {
		t.Errorf("cv content = %q, want pdf-bytes", string(content))
	}
}

func TestApplySkipsWithoutResume(t *testing.T) {
	srv, got := newMockRecruitee(t, http.StatusOK)
	c := &Client{http: srv.Client(), applyHost: srv.URL}

	profile := testProfile(t)
	profile.ResumePath = ""

	res, err := c.Apply(context.Background(), testJob(), profile)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "skipped" {
		t.Fatalf("status = %q, want skipped", res.Status)
	}
	if got.hits != 0 {
		t.Errorf("request hits = %d, want 0 — nothing must be posted without a resume", got.hits)
	}
}

func TestApplySkipsWhenOfferSlugMissing(t *testing.T) {
	srv, got := newMockRecruitee(t, http.StatusOK)
	c := &Client{http: srv.Client(), applyHost: srv.URL}

	job := testJob()
	job.URL = "https://testco.recruitee.com" // no /o/ or /l/offers/ segment

	res, err := c.Apply(context.Background(), job, testProfile(t))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "skipped" {
		t.Fatalf("status = %q, want skipped", res.Status)
	}
	if got.hits != 0 {
		t.Errorf("request hits = %d, want 0", got.hits)
	}
}

func TestApplyFailsOnTransportError(t *testing.T) {
	// Point the client at a server that is already closed so the POST
	// fails with a connection error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := srv.URL
	srv.Close()

	c := &Client{http: srv.Client(), applyHost: closedURL}
	res, err := c.Apply(context.Background(), testJob(), testProfile(t))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "failed" {
		t.Fatalf("status = %q, want failed", res.Status)
	}
}

func TestExtractOfferSlug(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"classic url", "https://testco.recruitee.com/o/backend-engineer", "backend-engineer"},
		{"classic trailing slash", "https://testco.recruitee.com/o/backend-engineer/", "backend-engineer"},
		{"modern url", "https://testco.recruitee.com/l/offers/backend-engineer", "backend-engineer"},
		{"modern with token", "https://testco.recruitee.com/l/offers/backend-engineer?token=abc123", "backend-engineer"},
		{"empty", "", ""},
		{"host only", "https://testco.recruitee.com", ""},
		{"slash only", "/o/", ""},
		{"whitespace", "  ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractOfferSlug(tt.in)
			if got != tt.want {
				t.Errorf("extractOfferSlug(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
