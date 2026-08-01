package lever

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
		City:       "London",
		LinkedInID: "adalovelace",
		ResumePath: resumePath,
	}
}

// testJob returns the job shape Search produces for a Lever posting.
func testJob() provider.Job {
	return provider.Job{
		ID:       "abc-123-def",
		Title:    "Backend Engineer",
		Company:  "TestCo",
		Board:    "testco",
		URL:      "https://jobs.lever.co/testco/abc-123-def",
		Provider: "lever",
	}
}

// capturedRequest is the raw apply request a mock board received.
type capturedRequest struct {
	req  *http.Request
	body []byte
}

// newMockLever returns a server that records every apply POST and answers
// with the given status code.
func newMockLever(t *testing.T, status int) (*httptest.Server, *capturedRequest) {
	t.Helper()
	got := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.req = r.Clone(r.Context())
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		got.body = body
		if status == http.StatusFound {
			w.Header().Set("Location", "https://jobs.lever.co/success")
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

// parseForm re-reads a captured multipart body using the boundary from the
// captured Content-Type header.
func parseForm(t *testing.T, got *capturedRequest) *multipart.Form {
	t.Helper()
	_, params, err := mime.ParseMediaType(got.req.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parse content-type: %v", err)
	}
	mr := multipart.NewReader(bytes.NewReader(got.body), params["boundary"])
	form, err := mr.ReadForm(10 << 20)
	if err != nil {
		t.Fatalf("read multipart form: %v", err)
	}
	return form
}

func TestApplyStatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		wantStatus string
	}{
		{"accepted", http.StatusOK, "applied"},
		{"accepted created", http.StatusCreated, "applied"},
		{"accepted redirect", http.StatusFound, "applied"},
		{"rejected 400", http.StatusBadRequest, "skipped"},
		{"rejected 404", http.StatusNotFound, "skipped"},
		{"server error", http.StatusInternalServerError, "failed"},
		{"rate limited", http.StatusTooManyRequests, "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newMockLever(t, tt.status)
			c := &Client{http: srv.Client(), baseURL: srv.URL}

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

func TestApplyPostsWellFormedMultipart(t *testing.T) {
	srv, got := newMockLever(t, http.StatusOK)
	c := &Client{http: srv.Client(), baseURL: srv.URL}

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
	if want := "/testco/abc-123-def/apply"; got.req.URL.Path != want {
		t.Errorf("path = %q, want %q", got.req.URL.Path, want)
	}

	form := parseForm(t, got)
	wantFields := map[string]string{
		"name":           "Ada Lovelace",
		"email":          "ada@example.com",
		"phone":          "+15551234567",
		"org":            "London",
		"urls[LinkedIn]": "https://linkedin.com/in/adalovelace",
	}
	for field, want := range wantFields {
		gotVals := form.Value[field]
		if len(gotVals) != 1 || gotVals[0] != want {
			t.Errorf("field %q = %v, want [%q]", field, gotVals, want)
		}
	}

	fileParts := form.File["resume"]
	if len(fileParts) != 1 {
		t.Fatalf("resume file parts = %d, want 1", len(fileParts))
	}
	if fileParts[0].Filename != "resume.pdf" {
		t.Errorf("resume filename = %q, want resume.pdf", fileParts[0].Filename)
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
		t.Errorf("resume content = %q, want pdf-bytes", string(content))
	}
}

func TestApplyWithoutResumeStillPosts(t *testing.T) {
	srv, got := newMockLever(t, http.StatusOK)
	c := &Client{http: srv.Client(), baseURL: srv.URL}

	profile := testProfile(t)
	profile.ResumePath = "" // no resume configured

	res, err := c.Apply(context.Background(), testJob(), profile)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "applied" {
		t.Fatalf("status = %q, want applied", res.Status)
	}
	form := parseForm(t, got)
	if len(form.File["resume"]) != 0 {
		t.Error("no resume file should be attached when ResumePath is empty")
	}
}

func TestApplyFailsOnTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := srv.URL
	srv.Close()

	c := &Client{http: srv.Client(), baseURL: closedURL}
	res, err := c.Apply(context.Background(), testJob(), testProfile(t))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "failed" {
		t.Fatalf("status = %q, want failed", res.Status)
	}
}
