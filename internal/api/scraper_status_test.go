package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleScraperStatus exercises the status endpoint hermetically: the
// scraper package's Installed/Running checks are pure filesystem probes (the
// Python venv may or may not exist on the runner — either way the response is
// valid JSON with real booleans).
func TestHandleScraperStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	(&Server{}).handleScraperStatus(rec, httptest.NewRequest(http.MethodGet, "/api/scraper/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d; want 200", rec.Code)
	}
	var body ScraperStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not ScraperStatus JSON: %v", err)
	}
	// Both fields must be JSON booleans; Installed is environment-dependent,
	// so only assert shape, not a specific value.
	_ = body.Installed
	_ = body.Running
}
