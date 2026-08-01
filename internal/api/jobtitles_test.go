package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

func TestPostJobTitlesSuggestOfflineIncludesProfession(t *testing.T) {
	cfg := &config.Config{} // AI Assist off by default → offline catalog path
	srv := &Server{cfg: cfg}

	body, _ := json.Marshal(map[string]any{"intent": "I'm a cardiologist, remote"})
	req := httptest.NewRequest(http.MethodPost, "/api/job-titles/suggest", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.handlePostJobTitlesSuggest(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Titles     []string `json:"titles"`
		Intent     string   `json:"intent"`
		Profession string   `json:"profession"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Titles) == 0 {
		t.Error("expected offline title suggestions")
	}
	if resp.Profession != "Healthcare" {
		t.Errorf("profession = %q; want %q", resp.Profession, "Healthcare")
	}
}

func TestPostJobTitlesSuggestOfflineUnknownProfessionEmpty(t *testing.T) {
	cfg := &config.Config{}
	srv := &Server{cfg: cfg}

	body, _ := json.Marshal(map[string]any{"intent": "Life Coach, Wellness"})
	req := httptest.NewRequest(http.MethodPost, "/api/job-titles/suggest", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.handlePostJobTitlesSuggest(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Profession string `json:"profession"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Profession != "" {
		t.Errorf("profession = %q; want empty for unknown intents", resp.Profession)
	}
}
