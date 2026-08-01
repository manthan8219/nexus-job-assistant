package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func TestHandleGetAnalytics(t *testing.T) {
	st, err := store.OpenPath(filepath.Join(t.TempDir(), "api-analytics.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer st.Close()
	if err := st.Insert(store.Application{
		Provider: "greenhouse", Company: "Acme", Role: "Engineer",
		URL: "https://example.com/a1", Status: store.StatusApplied,
		AppliedAt: time.Now(), Outcome: store.OutcomeInterview,
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	srv := &Server{store: st}
	req := httptest.NewRequest(http.MethodGet, "/api/analytics", nil)
	rr := httptest.NewRecorder()
	srv.handleGetAnalytics(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}
	var body struct {
		StatusTotals map[string]int `json:"statusTotals"`
		Funnel       struct {
			Applied int `json:"applied"`
		} `json:"funnel"`
		PerProvider []store.ProviderYield `json:"perProvider"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.StatusTotals["applied"] != 1 {
		t.Errorf("statusTotals = %v; want applied=1", body.StatusTotals)
	}
	if body.Funnel.Applied != 1 {
		t.Errorf("funnel.applied = %d; want 1", body.Funnel.Applied)
	}
	if len(body.PerProvider) != 1 || body.PerProvider[0].Provider != "greenhouse" {
		t.Errorf("perProvider = %+v; want [greenhouse]", body.PerProvider)
	}
}

func TestHandleGetAnalytics_StoreUnavailable(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/analytics", nil)
	rr := httptest.NewRecorder()
	srv.handleGetAnalytics(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("nil store status = %d; want 500", rr.Code)
	}
}

func TestHandleGetAnalytics_StoreError(t *testing.T) {
	st, err := store.OpenPath(filepath.Join(t.TempDir(), "api-analytics-closed.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	st.Close()
	srv := &Server{store: st}
	req := httptest.NewRequest(http.MethodGet, "/api/analytics", nil)
	rr := httptest.NewRecorder()
	srv.handleGetAnalytics(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("closed store status = %d; want 500", rr.Code)
	}
}
