package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

func TestHandlePostRun(t *testing.T) {
	t.Run("unavailable when engine missing", func(t *testing.T) {
		s := &Server{} // eng nil
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/run", strings.NewReader(`{"dryRun":true,"autoApply":false}`))
		s.handlePostRun(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("code = %d; want 500 (engine not available)", rec.Code)
		}
	})

	t.Run("conflict when already running", func(t *testing.T) {
		s := &Server{runState: runState{status: StatusRunning}}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/run", strings.NewReader(`{"dryRun":true,"autoApply":false}`))
		s.handlePostRun(rec, req)
		if rec.Code != http.StatusConflict {
			t.Errorf("code = %d; want 409", rec.Code)
		}
	})
}

func TestHandlePostRunApplySelected(t *testing.T) {
	t.Run("rejects without apply consent", func(t *testing.T) {
		s := &Server{cfg: &config.Config{}} // ApplyConsent false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/run/apply-selected",
			strings.NewReader(`{"ids":[1]}`))
		s.handlePostRunApplySelected(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d; want 400 (consent required)", rec.Code)
		}
	})

	t.Run("rejects empty ids", func(t *testing.T) {
		s := &Server{cfg: &config.Config{ApplyConsent: true}}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/run/apply-selected",
			strings.NewReader(`{"ids":[]}`))
		s.handlePostRunApplySelected(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d; want 400", rec.Code)
		}
	})

	t.Run("engine missing still guarded by consent check", func(t *testing.T) {
		s := &Server{cfg: &config.Config{ApplyConsent: true}} // eng nil
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/run/apply-selected",
			strings.NewReader(`{"ids":[1]}`))
		s.handlePostRunApplySelected(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("code = %d; want 500 (engine not available after consent)", rec.Code)
		}
	})

	t.Run("conflict when already running", func(t *testing.T) {
		s := &Server{cfg: &config.Config{ApplyConsent: true}, runState: runState{status: StatusRunning}}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/run/apply-selected",
			strings.NewReader(`{"ids":[1]}`))
		s.handlePostRunApplySelected(rec, req)
		if rec.Code != http.StatusConflict {
			t.Errorf("code = %d; want 409", rec.Code)
		}
	})
}

func TestHandleDeleteRun(t *testing.T) {
	s := &Server{runState: runState{status: StatusRunning}}
	rec := httptest.NewRecorder()
	s.handleDeleteRun(rec, httptest.NewRequest(http.MethodDelete, "/api/run", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d; want 200", rec.Code)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.status != StatusStopped {
		t.Errorf("status = %q; want stopped after cancel", s.status)
	}
}
