package enrich

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchDescription_Greenhouse_HappyPath(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"content":"<p>Hello world</p>"}`))
	}))
	defer ts.Close()

	orig := ghAPIBase
	ghAPIBase = ts.URL
	defer func() { ghAPIBase = orig }()

	got, err := FetchDescription(context.Background(), "greenhouse", "https://boards.greenhouse.io/acme/jobs/123")
	if err != nil {
		t.Fatalf("FetchDescription: %v", err)
	}
	if !strings.Contains(got, "Hello world") {
		t.Errorf("description = %q; want it to contain \"Hello world\"", got)
	}
}

func TestFetchDescription_Greenhouse_EmptyContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"content":""}`))
	}))
	defer ts.Close()

	orig := ghAPIBase
	ghAPIBase = ts.URL
	defer func() { ghAPIBase = orig }()

	if _, err := FetchDescription(context.Background(), "greenhouse", "https://boards.greenhouse.io/acme/jobs/123"); err == nil {
		t.Fatal("expected error for empty greenhouse content")
	}
}

func TestFetchDescription_Greenhouse_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	orig := ghAPIBase
	ghAPIBase = ts.URL
	defer func() { ghAPIBase = orig }()

	if _, err := FetchDescription(context.Background(), "greenhouse", "https://boards.greenhouse.io/acme/jobs/123"); err == nil {
		t.Fatal("expected error for greenhouse HTTP 500")
	}
}

func TestFetchDescription_Greenhouse_BadURL(t *testing.T) {
	if _, err := FetchDescription(context.Background(), "greenhouse", "https://example.com/notgreenhouse"); err == nil {
		t.Fatal("expected error for unrecognized greenhouse URL")
	}
}

func TestFetchDescription_Lever_HappyPath(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"descriptionPlain":"Lever job desc"}`))
	}))
	defer ts.Close()

	orig := leverAPIBase
	leverAPIBase = ts.URL
	defer func() { leverAPIBase = orig }()

	got, err := FetchDescription(context.Background(), "lever", "https://jobs.lever.co/acme/a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	if err != nil {
		t.Fatalf("FetchDescription: %v", err)
	}
	if !strings.Contains(got, "Lever job desc") {
		t.Errorf("description = %q; want it to contain \"Lever job desc\"", got)
	}
}

func TestFetchDescription_Lever_FallsBackToDescription(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"description":"<p>HTML desc</p>"}`))
	}))
	defer ts.Close()

	orig := leverAPIBase
	leverAPIBase = ts.URL
	defer func() { leverAPIBase = orig }()

	got, err := FetchDescription(context.Background(), "lever", "https://jobs.lever.co/acme/abc123def")
	if err != nil {
		t.Fatalf("FetchDescription: %v", err)
	}
	if !strings.Contains(got, "HTML desc") {
		t.Errorf("description = %q; want it to contain \"HTML desc\"", got)
	}
}

func TestFetchDescription_Lever_AdditionalAppended(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"descriptionPlain":"Main body","additionalPlain":"Extra notes"}`))
	}))
	defer ts.Close()

	orig := leverAPIBase
	leverAPIBase = ts.URL
	defer func() { leverAPIBase = orig }()

	got, err := FetchDescription(context.Background(), "lever", "https://jobs.lever.co/acme/abc123def")
	if err != nil {
		t.Fatalf("FetchDescription: %v", err)
	}
	if !strings.Contains(got, "Main body") || !strings.Contains(got, "Extra notes") {
		t.Errorf("description = %q; want both main and additional", got)
	}
}

func TestFetchDescription_Lever_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	orig := leverAPIBase
	leverAPIBase = ts.URL
	defer func() { leverAPIBase = orig }()

	if _, err := FetchDescription(context.Background(), "lever", "https://jobs.lever.co/acme/abc123def"); err == nil {
		t.Fatal("expected error for lever HTTP 404")
	}
}

func TestFetchDescription_UnsupportedProvider(t *testing.T) {
	if _, err := FetchDescription(context.Background(), "workday", "https://example.com/jobs/1"); err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestFetchDescription_EmptyURL(t *testing.T) {
	if _, err := FetchDescription(context.Background(), "greenhouse", ""); err == nil {
		t.Fatal("expected error for empty job URL")
	}
}

func TestFetchDescription_URLSniffFallback_Greenhouse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"content":"<p>Sniffed greenhouse</p>"}`))
	}))
	defer ts.Close()

	orig := ghAPIBase
	ghAPIBase = ts.URL
	defer func() { ghAPIBase = orig }()

	got, err := FetchDescription(context.Background(), "", "https://boards.greenhouse.io/acme/jobs/123")
	if err != nil {
		t.Fatalf("FetchDescription: %v", err)
	}
	if !strings.Contains(got, "Sniffed greenhouse") {
		t.Errorf("description = %q; want it to contain \"Sniffed greenhouse\"", got)
	}
}

func TestFetchDescription_ContextCancelled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"content":"<p>x</p>"}`))
	}))
	defer ts.Close()

	orig := ghAPIBase
	ghAPIBase = ts.URL
	defer func() { ghAPIBase = orig }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := FetchDescription(ctx, "greenhouse", "https://boards.greenhouse.io/acme/jobs/123"); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
