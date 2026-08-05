package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleFSAutocomplete(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"resume.pdf", "notes.docx", "secret.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
	get := func(t *testing.T, prefix string) []string {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet,
			"/api/fs/autocomplete?path="+url.QueryEscape(prefix), nil)
		(&Server{}).handleFSAutocomplete(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d; want 200", rec.Code)
		}
		var out []string
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("body not JSON array: %v", err)
		}
		return out
	}

	t.Run("empty prefix returns empty array", func(t *testing.T) {
		if got := get(t, ""); len(got) != 0 {
			t.Errorf("suggestions = %v; want none", got)
		}
	})

	t.Run("matches resume.pdf and excludes txt", func(t *testing.T) {
		got := get(t, filepath.Join(dir, "res"))
		if len(got) != 1 || !strings.HasSuffix(got[0], "resume.pdf") {
			t.Errorf("suggestions = %v; want only resume.pdf", got)
		}
		if got := get(t, filepath.Join(dir, "secret")); len(got) != 0 {
			t.Errorf("txt suggestions = %v; want none", got)
		}
	})

	t.Run("unreadable dir returns empty array", func(t *testing.T) {
		if got := get(t, filepath.Join(dir, "missing")); len(got) != 0 {
			t.Errorf("suggestions = %v; want none", got)
		}
	})
}

func TestHandleGeoSearch(t *testing.T) {
	get := func(t *testing.T, q string) []struct {
		Label   string `json:"label"`
		Country string `json:"country"`
	} {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/geo/search?q="+url.QueryEscape(q), nil)
		(&Server{}).handleGeoSearch(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d; want 200", rec.Code)
		}
		var out []struct {
			Label   string `json:"label"`
			Country string `json:"country"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("body not JSON: %v", err)
		}
		return out
	}

	t.Run("empty query returns empty array", func(t *testing.T) {
		if got := get(t, ""); len(got) != 0 {
			t.Errorf("hits = %v; want none", got)
		}
	})

	t.Run("matches a known city", func(t *testing.T) {
		got := get(t, "new york")
		if len(got) == 0 {
			t.Fatal("hits = 0; want a New York match")
		}
		found := false
		for _, h := range got {
			if strings.Contains(strings.ToLower(h.Label), "new york") {
				found = true
			}
		}
		if !found {
			t.Errorf("hits = %v; want one containing New York", got)
		}
	})

	t.Run("unknown place returns empty array", func(t *testing.T) {
		if got := get(t, "zzz-nowhere-zzz"); len(got) != 0 {
			t.Errorf("hits = %v; want none", got)
		}
	})
}
