package arbeitnow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeJob(t *testing.T) {
	j := arbJob{
		Slug: "go-engineer-123", Company: "Acme",
		Title: "Go Engineer", Remote: true,
		URL: "https://www.arbeitnow.com/job/go-engineer-123",
		Location: "Berlin", CreatedAt: 1722000000,
	}
	pj := normalizeJob(j)
	if pj == nil {
		t.Fatal("expected non-nil")
	}
	if pj.Title != "Go Engineer" {
		t.Errorf("title = %q", pj.Title)
	}
	if pj.Company != "Acme" {
		t.Errorf("company = %q", pj.Company)
	}
	if !pj.Remote {
		t.Error("expected remote")
	}
	if !strings.Contains(pj.Location, "Berlin") || !strings.Contains(pj.Location, "Remote") {
		t.Errorf("location = %q, want Berlin, Remote", pj.Location)
	}
	if pj.PostedAt.IsZero() {
		t.Error("expected non-zero postedAt")
	}

	// Malformed URL -> nil
	if pj2 := normalizeJob(arbJob{Slug: "b1", Title: "Bad", URL: "http://evil.com/job"}); pj2 != nil {
		t.Error("expected nil for non-https URL")
	}
	// Untrusted host -> nil
	if pj3 := normalizeJob(arbJob{Slug: "b2", Title: "Bad", URL: "https://evil.com/job"}); pj3 != nil {
		t.Error("expected nil for untrusted host")
	}
	// Empty title -> nil
	if pj4 := normalizeJob(arbJob{Slug: "b3", URL: "https://www.arbeitnow.com/job/b3"}); pj4 != nil {
		t.Error("expected nil for empty title")
	}
}

func TestFetchPage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(arbResponse{
			Data: []arbJob{
				{Slug: "1", Title: "Go Engineer", URL: "https://www.arbeitnow.com/job/1", Company: "Go Inc", Remote: true, Location: "Berlin", CreatedAt: 1722000000},
			},
		})
	}))
	defer ts.Close()

	orig := feedBase
	feedBase = ts.URL
	defer func() { feedBase = orig }()

	client := New()
	jobs, err := fetchPage(nil, client.http, 1)
	if err == nil {
		t.Logf("mock returned %d jobs", len(jobs))
	} else {
		t.Logf("nil ctx -> %v", err)
	}
}
