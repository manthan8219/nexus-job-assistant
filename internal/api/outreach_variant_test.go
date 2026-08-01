package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
)

// handlePutOutreachItemVariant tags an item for A/B testing (KAN-27) and the
// value round-trips through the item store.
func TestOutreachItemVariantRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	item := outreach.Item{Channel: outreach.ChannelEmail, Company: "Acme", Role: "Engineer"}
	if err := outreach.Upsert(item); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	items, err := outreach.Load()
	if err != nil || len(items) != 1 {
		t.Fatalf("load = %d items, err %v; want 1", len(items), err)
	}
	id := items[0].ID
	if id == "" {
		t.Fatal("upsert did not assign an id")
	}

	srv := &Server{}
	put := httptest.NewRequest(http.MethodPut, "/api/outreach/items/"+id+"/variant", strings.NewReader(`{"variant":"A"}`))
	put.SetPathValue("id", id)
	rr := httptest.NewRecorder()
	srv.handlePutOutreachItemVariant(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status = %d; body %s", rr.Code, rr.Body.String())
	}
	var body struct {
		ID      string `json:"id"`
		Variant string `json:"variant"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.ID != id || body.Variant != "A" {
		t.Errorf("response = %+v; want id %s + variant A", body, id)
	}

	// Persisted, not just echoed.
	items, _ = outreach.Load()
	if items[0].Variant != "A" {
		t.Errorf("stored variant = %q; want A", items[0].Variant)
	}

	// Clearing the variant works.
	clear := httptest.NewRequest(http.MethodPut, "/api/outreach/items/"+id+"/variant", strings.NewReader(`{"variant":""}`))
	clear.SetPathValue("id", id)
	rr2 := httptest.NewRecorder()
	srv.handlePutOutreachItemVariant(rr2, clear)
	if rr2.Code != http.StatusOK {
		t.Fatalf("clear status = %d; body %s", rr2.Code, rr2.Body.String())
	}
	items, _ = outreach.Load()
	if items[0].Variant != "" {
		t.Errorf("stored variant after clear = %q; want empty", items[0].Variant)
	}
}

func TestOutreachItemVariantNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := &Server{}
	req := httptest.NewRequest(http.MethodPut, "/api/outreach/items/nope/variant", strings.NewReader(`{"variant":"A"}`))
	req.SetPathValue("id", "nope")
	rr := httptest.NewRecorder()
	srv.handlePutOutreachItemVariant(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", rr.Code)
	}
}
