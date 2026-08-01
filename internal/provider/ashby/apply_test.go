package ashby

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
		LinkedInID: "adalovelace",
		ResumePath: resumePath,
	}
}

// testJob returns the job shape Search produces for an Ashby posting.
func testJob() provider.Job {
	return provider.Job{
		ID:       "posting-42",
		Title:    "Backend Engineer",
		Company:  "TestCo",
		Board:    "testco",
		URL:      "https://jobs.ashbyhq.com/testco/posting-42",
		Provider: "ashby",
	}
}

// capturedRequest is the raw apply request a mock board received.
type capturedRequest struct {
	req  *http.Request
	body []byte
}

// newMockAshby returns a server that serves the board page (with an embedded
// org API key) and records the apply POST, answering with the given status.
func newMockAshby(t *testing.T, applyStatus int) (*httptest.Server, *capturedRequest) {
	t.Helper()
	got := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Board page: contains the org API key inside __NEXT_DATA__.
			if r.URL.Path != "/testco" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, `<html><script id="__NEXT_DATA__" type="application/json">{"props":{"apiKey":"test-key"}}</script></html>`)
			return
		}
		// Apply POST.
		got.req = r.Clone(r.Context())
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		got.body = body
		w.WriteHeader(applyStatus)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
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
		{"validation error", http.StatusBadRequest, "failed"},
		{"server error", http.StatusInternalServerError, "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newMockAshby(t, tt.status)
			c := &Client{http: srv.Client(), jobsHost: srv.URL, apiHost: srv.URL}

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
	srv, got := newMockAshby(t, http.StatusOK)
	c := &Client{http: srv.Client(), jobsHost: srv.URL, apiHost: srv.URL}

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
	if want := "/applicationForm.submit"; got.req.URL.Path != want {
		t.Errorf("path = %q, want %q", got.req.URL.Path, want)
	}

	form := parseForm(t, got)
	wantFields := map[string]string{
		"apiKey":       "test-key",
		"jobPostingId": "posting-42",
		"applicationForm[_systemfield_firstName]": "Ada",
		"applicationForm[_systemfield_lastName]":  "Lovelace",
		"applicationForm[_systemfield_email]":     "ada@example.com",
		"applicationForm[_systemfield_phone]":     "+15551234567",
		"applicationForm[_systemfield_linkedIn]":  "https://linkedin.com/in/adalovelace",
	}
	for field, want := range wantFields {
		gotVals := form.Value[field]
		if len(gotVals) != 1 || gotVals[0] != want {
			t.Errorf("field %q = %v, want [%q]", field, gotVals, want)
		}
	}

	fileParts := form.File["applicationForm[_systemfield_resume]"]
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

func TestApplySkipsWhenOrgAPIKeyMissing(t *testing.T) {
	// Board page without the apiKey marker → the apply must be skipped and
	// nothing POSTed.
	posted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, `<html><body>no key here</body></html>`)
			return
		}
		posted = true
		t.Error("apply POST must not be sent when the API key is missing")
	}))
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), jobsHost: srv.URL, apiHost: srv.URL}

	res, err := c.Apply(context.Background(), testJob(), testProfile(t))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "skipped" {
		t.Fatalf("status = %q, want skipped", res.Status)
	}
	_ = posted
}

func TestApplyFailsOnTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := srv.URL
	srv.Close()

	c := &Client{http: srv.Client(), jobsHost: closedURL, apiHost: closedURL}
	res, err := c.Apply(context.Background(), testJob(), testProfile(t))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "failed" {
		t.Fatalf("status = %q, want failed", res.Status)
	}
}

func TestFetchOrgAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		want    string
		wantErr bool
	}{
		{"valid key", `<script>{"apiKey":"abc123"}</script>`, "abc123", false},
		{"missing marker", `<html>no key</html>`, "", true},
		{"unterminated", `<script>{"apiKey":"abc`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, tt.html)
			}))
			t.Cleanup(srv.Close)
			c := &Client{http: srv.Client(), jobsHost: srv.URL}

			got, err := c.fetchOrgAPIKey(context.Background(), "testco")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got key %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("fetchOrgAPIKey: %v", err)
			}
			if got != tt.want {
				t.Errorf("key = %q, want %q", got, tt.want)
			}
		})
	}
}
