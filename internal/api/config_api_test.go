package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// testConfig returns a fully-populated config for handler tests.
func testConfig() *config.Config {
	return &config.Config{
		FirstName:          "Ada",
		LastName:           "Lovelace",
		Email:              "ada@example.com",
		Phone:              "+44 20 7946 0958",
		ResumePath:         "/tmp/resume.pdf",
		City:               "London",
		TargetJobTitles:    "Engineer",
		TargetLocations:    "Remote",
		ApplyConsent:       true,
		ApplyConsentAt:     "2026-08-05T00:00:00Z",
		MaxAppsPerRun:      5,
		MaxAppsPerDay:      10,
		ApplyDelaySec:      12,
		MinFitScore:        60,
		DailyRunEnabled:    true,
		DailyRunAt:         "09:00",
		EmailNotifications: true,
		NotifyChannels:     []string{"discord"},
		AIAssist:           true,
		AIProvider:         "api",
	}
}

func TestHandleGetConfig(t *testing.T) {
	s := &Server{cfg: testConfig()}
	rec := httptest.NewRecorder()
	s.handleGetConfig(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d; want 200", rec.Code)
	}
	var got NexusConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not NexusConfig JSON: %v", err)
	}
	if got.FirstName != "Ada" || got.TargetJobTitles != "Engineer" {
		t.Errorf("got first=%q titles=%q; want Ada/Engineer", got.FirstName, got.TargetJobTitles)
	}
	if !got.ApplyConsent || got.MaxAppsPerRun != 5 {
		t.Errorf("consent=%v maxPerRun=%d; want true/5", got.ApplyConsent, got.MaxAppsPerRun)
	}
	if got.DailyRunEnabled != true || got.DailyRunAt != "09:00" {
		t.Errorf("daily=%v at=%q; want true/09:00", got.DailyRunEnabled, got.DailyRunAt)
	}
}

func TestHandlePutConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NEXUS_HOME", dir)
	s := &Server{cfg: &config.Config{}}

	t.Run("rejects malformed JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader("{"))
		s.handlePutConfig(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d; want 400", rec.Code)
		}
	})

	t.Run("applies and persists a valid config", func(t *testing.T) {
		body := `{"firstName":"Ada","lastName":"Lovelace","email":"ada@example.com",` +
			`"targetJobTitles":"Engineer","noticePeriodDays":30,"maxAppsPerRun":7}`
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body))
		s.handlePutConfig(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d; want 200", rec.Code)
		}
		if s.cfg.FirstName != "Ada" || s.cfg.NoticePeriodDays != "30" {
			t.Errorf("cfg.first=%q notice=%q; want Ada/30", s.cfg.FirstName, s.cfg.NoticePeriodDays)
		}
		saved, err := os.ReadFile(filepath.Join(dir, "config.json"))
		if err != nil {
			t.Fatalf("config not persisted: %v", err)
		}
		if !strings.Contains(string(saved), "ada@example.com") {
			t.Errorf("saved config does not contain email: %s", saved)
		}
	})
}

func TestHandlePatchConfig(t *testing.T) {
	t.Setenv("NEXUS_HOME", t.TempDir())
	s := &Server{cfg: testConfig()}

	for name, body := range map[string]string{
		"dry run on":     `{"dry_run":true}`,
		"auto apply off": `{"auto_apply":false}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/api/config", strings.NewReader(body))
			s.handlePatchConfig(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("code = %d; want 200", rec.Code)
			}
		})
	}
	if !s.dryRun {
		t.Error("dryRun = false; want true after PATCH")
	}
	if s.autoApply {
		t.Error("autoApply = true; want false after PATCH")
	}
}

func TestHandleGetConfigComplete(t *testing.T) {
	t.Run("incomplete profile lists missing fields", func(t *testing.T) {
		s := &Server{cfg: &config.Config{}}
		rec := httptest.NewRecorder()
		s.handleGetConfigComplete(rec, httptest.NewRequest(http.MethodGet, "/api/config/complete", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d; want 200", rec.Code)
		}
		var body struct {
			Complete bool     `json:"complete"`
			Missing  []string `json:"missing"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body not JSON: %v", err)
		}
		if body.Complete {
			t.Error("complete = true; want false for empty config")
		}
		for _, want := range []string{"First Name", "Email", "Target Job Titles"} {
			if !contains(strings.Join(body.Missing, "|"), want) {
				t.Errorf("missing list %v does not mention %q", body.Missing, want)
			}
		}
	})

	t.Run("complete profile reports complete", func(t *testing.T) {
		s := &Server{cfg: testConfig()}
		rec := httptest.NewRecorder()
		s.handleGetConfigComplete(rec, httptest.NewRequest(http.MethodGet, "/api/config/complete", nil))
		var body struct {
			Complete bool `json:"complete"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body not JSON: %v", err)
		}
		if !body.Complete {
			t.Errorf("complete = false; want true for full profile: %s", rec.Body.String())
		}
	})
}
