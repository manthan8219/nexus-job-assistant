package api

import (
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestWithMiddlewareCORSAllowList(t *testing.T) {
	s := &Server{allowedOrigins: []string{"https://app.example.com"}}
	h := s.withMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	t.Run("denies a disallowed origin", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("code = %d; want 403 for disallowed origin", rec.Code)
		}
	})

	t.Run("allows a listed origin", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
		req.Header.Set("Origin", "https://app.example.com/")
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Errorf("code = %d; want 204 for allowed origin", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
			t.Errorf("Allow-Origin = %q; want the allowed origin", got)
		}
	})

	t.Run("no origin header is unaffected", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
		if rec.Code != http.StatusNoContent {
			t.Errorf("code = %d; want 204", rec.Code)
		}
	})
}

func TestParseOrigins(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "single", in: "https://app.example.com", want: []string{"https://app.example.com"}},
		{name: "trims and drops empties", in: " https://a.com , ,https://b.com ", want: []string{"https://a.com", "https://b.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOrigins(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseOrigins(%q) = %v; want %v", tt.in, got, tt.want)
			}
		})
	}
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
