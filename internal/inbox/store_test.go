package inbox

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreUpsertDedupByMessageID(t *testing.T) {
	p := filepath.Join(t.TempDir(), "highlights.json")
	h := Highlight{MessageID: "m1", From: "a@b.com", Subject: "s", Date: time.Now()}
	if err := Upsert(p, h); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	h2 := h
	h2.Subject = "updated"
	if err := Upsert(p, h2); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	hs, err := LoadAll(p)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(hs) != 1 {
		t.Fatalf("expected 1 highlight after dedup, got %d", len(hs))
	}
	if hs[0].Subject != "updated" {
		t.Errorf("expected updated subject, got %q", hs[0].Subject)
	}
}

func TestStoreLoadMissingIsEmpty(t *testing.T) {
	hs, err := LoadAll(filepath.Join(t.TempDir(), "none.json"))
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(hs) != 0 {
		t.Errorf("expected empty, got %d", len(hs))
	}
}
