package api

import (
	"path/filepath"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// openTestStore opens a hermetic store in a temp dir so handler tests never
// touch ~/.nexus. Used by several handler test files in this package.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.OpenAt(filepath.Join(t.TempDir(), "apps.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}
