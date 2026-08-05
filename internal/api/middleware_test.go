package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithMiddlewareCORSAndOptions(t *testing.T) {
	s := &Server{}
	h := s.withMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	t.Run("preflight returns 204 with CORS headers", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/api/config", nil)
		req.Header.Set("Origin", "https://app.example.com")
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Errorf("code = %d; want 204", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
			t.Errorf("Allow-Origin = %q; want echoed origin", got)
		}
		if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Error("Allow-Credentials != true")
		}
	})

	t.Run("origin request gets CORS headers", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
		req.Header.Set("Origin", "https://app.example.com")
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Errorf("code = %d; want 204", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
			t.Error("Allow-Methods header missing")
		}
	})
}

func TestWithMiddlewarePanicRecovery(t *testing.T) {
	s := &Server{}
	h := s.withMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("code = %d; want 500", rec.Code)
	}
	if !contains(rec.Body.String(), `"ok":false`) {
		t.Errorf("body = %s; want ok:false error envelope", rec.Body.String())
	}
}
