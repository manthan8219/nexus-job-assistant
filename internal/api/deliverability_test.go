package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// fakeTXTResolver serves canned TXT records for the deliverability handler test.
type fakeTXTResolver struct {
	records map[string][]string
}

func (f *fakeTXTResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return f.records[name], nil
}

func TestHandleGetDeliverabilityAudit(t *testing.T) {
	records := map[string][]string{
		"example.com":                   {"v=spf1 include:_spf.google.com ~all"},
		"_dmarc.example.com":            {"v=DMARC1; p=quarantine"},
		"google._domainkey.example.com": {"v=DKIM1; p=MIGfMA"},
	}
	srv := &Server{txtResolver: &fakeTXTResolver{records: records}}

	req := httptest.NewRequest(http.MethodGet, "/api/deliverability/audit?domain=example.com", nil)
	rr := httptest.NewRecorder()
	srv.handleGetDeliverabilityAudit(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}
	var report struct {
		Domain string `json:"domain"`
		SPF    struct {
			Present bool   `json:"present"`
			Verdict string `json:"verdict"`
		} `json:"spf"`
		DMARC struct {
			Verdict string `json:"verdict"`
		} `json:"dmarc"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if report.Domain != "example.com" {
		t.Errorf("domain = %q", report.Domain)
	}
	if !report.SPF.Present || report.SPF.Verdict != "softfail" {
		t.Errorf("SPF = %+v; want present softfail", report.SPF)
	}
	if report.DMARC.Verdict != "quarantine" {
		t.Errorf("DMARC verdict = %q; want quarantine", report.DMARC.Verdict)
	}
	if report.Summary == "" {
		t.Error("summary must be populated")
	}
}

func TestHandleGetDeliverabilityAudit_Errors(t *testing.T) {
	srv := &Server{txtResolver: &fakeTXTResolver{}}

	// Missing domain → 400.
	req := httptest.NewRequest(http.MethodGet, "/api/deliverability/audit", nil)
	rr := httptest.NewRecorder()
	srv.handleGetDeliverabilityAudit(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing domain status = %d; want 400", rr.Code)
	}

	// Invalid domain → 400.
	req = httptest.NewRequest(http.MethodGet, "/api/deliverability/audit?domain=https://evil.com/path", nil)
	rr = httptest.NewRecorder()
	srv.handleGetDeliverabilityAudit(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid domain status = %d; want 400", rr.Code)
	}
}

func TestHandleGetDeliverabilityAudit_NilResolverDefaults(t *testing.T) {
	// A nil resolver field is allowed (Audit falls back to net.DefaultResolver),
	// so the handler must not panic even before a real DNS call.
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/deliverability/audit?domain="+url.QueryEscape("invalid domain with spaces"), nil)
	rr := httptest.NewRecorder()
	srv.handleGetDeliverabilityAudit(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid domain status = %d; want 400", rr.Code)
	}
}
