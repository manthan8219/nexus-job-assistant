package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

func TestListAIModels_HappyPath_StripsModelsPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %q; want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization = %q; want Bearer sk-test", got)
		}
		w.Write([]byte(`{"data":[
			{"id":"gemini-2.5-flash"},
			{"id":"models/gemini-2.5-pro"},
			{"id":"models/gemini-2.5-pro"},
			{"id":"gemini-2.5-flash"}
		]}`))
	}))
	defer srv.Close()

	got, err := listAIModels(context.Background(), "google", "sk-test", srv.URL)
	if err != nil {
		t.Fatalf("listAIModels: %v", err)
	}
	want := []string{"gemini-2.5-flash", "gemini-2.5-pro"}
	if len(got) != len(want) {
		t.Fatalf("got %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q; want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestListAIModels_AnthropicUsesXAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "sk-ant" {
			t.Errorf("x-api-key = %q; want sk-ant", got)
		}
		w.Write([]byte(`{"data":[{"id":"claude-3-5-haiku-latest"}]}`))
	}))
	defer srv.Close()

	got, err := listAIModels(context.Background(), "anthropic", "sk-ant", srv.URL)
	if err != nil {
		t.Fatalf("listAIModels: %v", err)
	}
	if len(got) != 1 || got[0] != "claude-3-5-haiku-latest" {
		t.Errorf("got %v; want [claude-3-5-haiku-latest]", got)
	}
}

func TestListAIModels_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	_, err := listAIModels(context.Background(), "openai", "k", srv.URL)
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("err = %v; want HTTP 401", err)
	}
}

func TestListAIModels_MalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{not json`))
	}))
	defer srv.Close()

	if _, err := listAIModels(context.Background(), "openai", "k", srv.URL); err == nil {
		t.Fatal("expected error for malformed body")
	}
}

func TestListAIModels_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := listAIModels(ctx, "openai", "k", srv.URL); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestHandleGetAIModels_MissingProvider(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/ai/models", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.handleGetAIModels(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400; body %s", rr.Code, rr.Body.String())
	}
}

func TestHandleGetAIModels_NoKeyConfigured(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/api/ai/models", strings.NewReader(`{"provider":"google"}`))
	rr := httptest.NewRecorder()
	s.handleGetAIModels(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400; body %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "google") {
		t.Errorf("body %q should mention provider", rr.Body.String())
	}
}
