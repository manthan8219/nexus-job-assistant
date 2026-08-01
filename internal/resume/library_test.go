package resume

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRegisterAndGetVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NEXUS_HOME", "")

	dir := filepath.Join(home, ".nexus", "resumes")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	pdf := filepath.Join(dir, "improved-20260101-120000.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-fake"), 0600); err != nil {
		t.Fatal(err)
	}
	v := Version{
		ID:        "20260101-120000",
		CreatedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		Label:     "Senior Backend Engineer",
		Template:  "classic",
		PDFPath:   pdf,
	}
	if err := RegisterVersion(v); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, err := GetVersion("20260101-120000")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != v.ID || got.PDFPath != pdf {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if _, err := GetVersion("missing"); err == nil {
		t.Error("GetVersion(missing) should error")
	}
}
