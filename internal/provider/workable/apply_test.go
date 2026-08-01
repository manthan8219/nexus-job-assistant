package workable

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// testProfile returns a minimal applicant profile.
func testProfile() provider.Profile {
	return provider.Profile{
		FirstName: "Ada",
		LastName:  "Lovelace",
		Email:     "ada@example.com",
		Phone:     "+15551234567",
	}
}

// testJob returns a Workable job whose URL carries the shortcode the apply
// path extracts.
func testJob() provider.Job {
	return provider.Job{
		ID:       "job-123",
		Title:    "Backend Engineer",
		Company:  "TestCo",
		Board:    "testco",
		URL:      "https://apply.workable.com/testco/j/ABC123DEF/",
		Provider: "workable",
	}
}

// capturedRequest is the raw apply request a mock board received.
type capturedRequest struct {
	req  *http.Request
	body []byte
}

// newMockWorkable returns a server that records every apply request and
// answers with the given status code.
func newMockWorkable(t *testing.T, status int) (*httptest.Server, *capturedRequest) {
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
		_, _ = io.WriteString(w, `{"status":"ok"}`)
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
		{"rejected 404", http.StatusNotFound, "skipped"},
		{"rejected 422", http.StatusUnprocessableEntity, "skipped"},
		{"server error", http.StatusInternalServerError, "failed"},
		{"rate limited", http.StatusTooManyRequests, "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newMockWorkable(t, tt.status)
			c := &Client{http: srv.Client(), baseURL: srv.URL}

			res, err := c.Apply(context.Background(), testJob(), testProfile())
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
	srv, got := newMockWorkable(t, http.StatusOK)
	c := &Client{http: srv.Client(), baseURL: srv.URL}

	res, err := c.Apply(context.Background(), testJob(), testProfile())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "applied" {
		t.Fatalf("status = %q, want applied", res.Status)
	}

	if got.req.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.req.Method)
	}
	if want := "/testco/jobs/ABC123DEF/candidates"; got.req.URL.Path != want {
		t.Errorf("path = %q, want %q", got.req.URL.Path, want)
	}
	if got := got.req.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("content-type = %q, want application/json", got)
	}

	var payload workableCandidateRequest
	if err := json.Unmarshal(got.body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.FirstName != "Ada" || payload.LastName != "Lovelace" ||
		payload.Email != "ada@example.com" || payload.Phone != "+15551234567" {
		t.Errorf("payload = %+v, want Ada Lovelace / ada@example.com / +15551234567", payload)
	}
}

func TestApplySkipsWhenShortcodeMissing(t *testing.T) {
	srv, got := newMockWorkable(t, http.StatusOK)
	c := &Client{http: srv.Client(), baseURL: srv.URL}

	job := testJob()
	job.URL = "" // no URL → no shortcode → must skip without sending

	res, err := c.Apply(context.Background(), job, testProfile())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "skipped" {
		t.Fatalf("status = %q, want skipped", res.Status)
	}
	if got.req != nil {
		t.Error("request must not be sent when the shortcode is missing")
	}
}

func TestApplyFailsOnTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := srv.URL
	srv.Close()

	c := &Client{http: srv.Client(), baseURL: closedURL}
	res, err := c.Apply(context.Background(), testJob(), testProfile())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "failed" {
		t.Fatalf("status = %q, want failed", res.Status)
	}
}

func TestExtractShortcode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"classic workable url", "https://apply.workable.com/testco/j/ABC123DEF/", "ABC123DEF"},
		{"trailing slash", "https://apply.workable.com/testco/j/ABC123DEF", "ABC123DEF"},
		{"empty", "", ""},
		{"root", "https://apply.workable.com/testco", "testco"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractShortcode(tt.in)
			if got != tt.want {
				t.Errorf("extractShortcode(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
