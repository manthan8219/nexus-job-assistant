package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

func TestHandleGetLogs(t *testing.T) {
	s := &Server{}
	s.logLine("provider greenhouse started")
	s.logLine("applied Software Engineer @ Acme")
	s.logLine("skipped — below fit threshold")

	get := func(t *testing.T, filter string) []string {
		t.Helper()
		path := "/api/logs"
		if filter != "" {
			path += "?filter=" + filter
		}
		rec := httptest.NewRecorder()
		s.handleGetLogs(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d; want 200", rec.Code)
		}
		var body LogLine
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body not LogLine JSON: %v", err)
		}
		return body.Lines
	}

	if got := get(t, ""); len(got) != 3 {
		t.Errorf("unfiltered lines = %v; want 3", got)
	}
	if got := get(t, "applied"); len(got) != 1 || !strings.Contains(got[0], "applied") {
		t.Errorf("filtered lines = %v; want the applied line", got)
	}
	if got := get(t, "no-such-token"); len(got) != 0 {
		t.Errorf("matching lines = %v; want none", got)
	}
}

func TestHandleGetLogsEmptyBufferEmitsArray(t *testing.T) {
	rec := httptest.NewRecorder()
	(&Server{}).handleGetLogs(rec, httptest.NewRequest(http.MethodGet, "/api/logs", nil))
	if body := strings.TrimSpace(rec.Body.String()); !strings.Contains(body, `"lines":[]`) {
		t.Errorf("empty logs body = %s; want lines:[] (not null)", body)
	}
}

func TestHandleDeleteLogs(t *testing.T) {
	s := &Server{}
	s.logLine("to be cleared")
	rec := httptest.NewRecorder()
	s.handleDeleteLogs(rec, httptest.NewRequest(http.MethodDelete, "/api/logs", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d; want 200", rec.Code)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.logLines) != 0 {
		t.Errorf("logLines len = %d; want 0 after delete", len(s.logLines))
	}
}

func TestLogLineBounded(t *testing.T) {
	s := &Server{}
	const n = 2000
	for i := 0; i < n; i++ {
		s.logLine("L" + itoa(i))
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.logLines) > 1001 {
		t.Errorf("logLines len = %d; want <= 1001 (buffer is capped)", len(s.logLines))
	}
	last := s.logLines[len(s.logLines)-1]
	if last != "L"+itoa(n-1) {
		t.Errorf("last line = %q; want %q", last, "L"+itoa(n-1))
	}
}

func TestHandleGetUsage(t *testing.T) {
	t.Run("ai off when no cfg", func(t *testing.T) {
		rec := httptest.NewRecorder()
		(&Server{}).handleGetUsage(rec, httptest.NewRequest(http.MethodGet, "/api/usage", nil))
		var body UsageSnapshot
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body not UsageSnapshot JSON: %v", err)
		}
		if body.AIMode != "off" || body.CollectedAt == "" {
			t.Errorf("usage = %+v; want AIMode=off and a timestamp", body)
		}
	})

	t.Run("ai mode reflects provider", func(t *testing.T) {
		s := &Server{cfg: &config.Config{AIAssist: true, AIProvider: "api"}}
		rec := httptest.NewRecorder()
		s.handleGetUsage(rec, httptest.NewRequest(http.MethodGet, "/api/usage", nil))
		var body UsageSnapshot
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body not UsageSnapshot JSON: %v", err)
		}
		if body.AIMode != "api" {
			t.Errorf("AIMode = %q; want api", body.AIMode)
		}
	})
}
