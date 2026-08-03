package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// isolateNexusHome points both HOME and NEXUS_HOME at a temp dir. NEXUS_HOME is
// required on Windows, where Go's os.UserHomeDir reads USERPROFILE and ignores
// $HOME — without it these tests would write the REAL ~/.nexus config.json.
func isolateNexusHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NEXUS_HOME", filepath.Join(home, ".nexus"))
}

// handleGetOutreachSetup/handlePutOutreachSetup round-trip the referral-ask
// variant knobs (KAN-28) alongside the existing outreach settings.
func TestOutreachSetupReferralRoundTrip(t *testing.T) {
	isolateNexusHome(t)
	srv := &Server{cfg: &config.Config{}}

	// PUT enables the referral-ask variant with custom templates.
	put := httptest.NewRequest(http.MethodPut, "/api/outreach/setup", strings.NewReader(`{
		"consent": true,
		"mode": "queue",
		"maxEmailsPerDay": 5,
		"maxLinkedInPerDay": 3,
		"aiCompose": true,
		"aiReview": false,
		"referralAsk": true,
		"referralSubjectTpl": "Intro for {{role}} at {{company}}",
		"referralBodyTpl": "Could you introduce me to the hiring team?"
	}`))
	rr := httptest.NewRecorder()
	srv.handlePutOutreachSetup(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status = %d; body %s", rr.Code, rr.Body.String())
	}

	// GET echoes the persisted referral knobs.
	get := httptest.NewRequest(http.MethodGet, "/api/outreach/setup", nil)
	rr2 := httptest.NewRecorder()
	srv.handleGetOutreachSetup(rr2, get)
	if rr2.Code != http.StatusOK {
		t.Fatalf("GET status = %d; body %s", rr2.Code, rr2.Body.String())
	}
	var body struct {
		Consent            bool   `json:"consent"`
		ReferralAsk        bool   `json:"referralAsk"`
		ReferralSubjectTpl string `json:"referralSubjectTpl"`
		ReferralBodyTpl    string `json:"referralBodyTpl"`
	}
	if err := json.Unmarshal(rr2.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.Consent || !body.ReferralAsk {
		t.Errorf("GET = %+v; want consent + referralAsk true", body)
	}
	if body.ReferralSubjectTpl != "Intro for {{role}} at {{company}}" {
		t.Errorf("subject = %q; want custom template", body.ReferralSubjectTpl)
	}
	if body.ReferralBodyTpl == "" {
		t.Errorf("body tpl missing after PUT")
	}

	// The values persist to disk through config.Save (no stale memory copy).
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if !cfg.OutreachReferralAsk || cfg.ReferralBodyTpl == "" {
		t.Errorf("persisted cfg = %+v; want referral ask on + body tpl", cfg)
	}
}

func TestOutreachSetupReferralDefaultsOff(t *testing.T) {
	isolateNexusHome(t)
	srv := &Server{cfg: &config.Config{}}

	get := httptest.NewRequest(http.MethodGet, "/api/outreach/setup", nil)
	rr := httptest.NewRecorder()
	srv.handleGetOutreachSetup(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d; body %s", rr.Code, rr.Body.String())
	}
	var body struct {
		ReferralAsk bool `json:"referralAsk"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.ReferralAsk {
		t.Errorf("referralAsk default = true; want false")
	}
}
