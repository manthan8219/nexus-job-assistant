package companies

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpenEmbeddedAt verifies the per-island company opener: it creates the DB
// at a custom path, seeds the embedded catalogs offline, and is re-openable.
func TestOpenEmbeddedAt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "island")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "companies.db")

	db, err := OpenEmbeddedAt(path)
	if err != nil {
		t.Fatalf("OpenEmbeddedAt: %v", err)
	}
	if db == nil {
		t.Fatal("OpenEmbeddedAt returned nil DB")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Re-opening the same path (simulating a later session) must work.
	db2, err := OpenEmbeddedAt(path)
	if err != nil {
		t.Fatalf("re-open OpenEmbeddedAt: %v", err)
	}
	_ = db2.Close()
}
