package userstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func TestRegistryForOpensIsland(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	reg := NewRegistry(filepath.Join(root, "users"), nil, 0)

	st, err := reg.For("11111111-aaaa-4bbb-8ccc-000000000001", "a@example.com")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if st.UserID != "11111111-aaaa-4bbb-8ccc-000000000001" {
		t.Errorf("UserID = %q; want the user id", st.UserID)
	}
	if st.Cfg == nil || st.Apps == nil || st.Contacts == nil || st.Companies == nil {
		t.Fatalf("island handles = cfg:%v apps:%v contacts:%v companies:%v; want all non-nil",
			st.Cfg, st.Apps, st.Contacts, st.Companies)
	}
	for _, name := range []string{"applications.db", "contacts.db", "companies.db"} {
		if _, err := os.Stat(filepath.Join(st.Dir, name)); err != nil {
			t.Errorf("island missing %s: %v", name, err)
		}
	}
	// config.json is created lazily on first save, not at open.
	if _, err := os.Stat(filepath.Join(st.Dir, "config.json")); !os.IsNotExist(err) {
		t.Errorf("config.json exists before any save: %v", err)
	}
	if err := config.SaveTo(st.Cfg, filepath.Join(st.Dir, "config.json")); err != nil {
		t.Fatalf("save island config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(st.Dir, "config.json")); err != nil {
		t.Errorf("config.json still missing after save: %v", err)
	}

	// The island store is genuinely usable.
	if err := st.Apps.Insert(store.Application{
		Provider: "manual", Company: "Acme", Role: "Engineer",
		URL: "https://jobs.example.com/1", Status: store.StatusQueued,
		AppliedAt: time.Now().UTC(), PostedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("insert into island: %v", err)
	}
	apps, err := st.Apps.List()
	if err != nil || len(apps) != 1 {
		t.Errorf("island apps = %d, err=%v; want 1 row", len(apps), err)
	}
}

func TestRegistryForReturnsSameStore(t *testing.T) {
	reg := NewRegistry(filepath.Join(t.TempDir(), "users"), nil, 0)
	st1, err := reg.For("user-1", "a@x.com")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	st2, err := reg.For("user-1", "a@x.com")
	if err != nil {
		t.Fatalf("For again: %v", err)
	}
	if st1 != st2 {
		t.Error("For twice returned different Store pointers; want the same open island")
	}
}

func TestRegistryForRejectsUnsafeIDs(t *testing.T) {
	reg := NewRegistry(filepath.Join(t.TempDir(), "users"), nil, 0)
	bad := []string{"", ".", "..", "../escape", "a/b", "a b", "ab\x00",
		"11111111-aaaa-4bbb-8ccc-000000000001-000000000002-000000000003-000000000004-000000000005-000000000006-000000000007-000000000008-x"}
	for _, id := range bad {
		if _, err := reg.For(id, "a@x.com"); !errors.Is(err, ErrNoUser) {
			t.Errorf("For(%q) err = %v; want ErrNoUser", id, err)
		}
	}
}

func TestRegistryEviction(t *testing.T) {
	reg := NewRegistry(filepath.Join(t.TempDir(), "users"), nil, 2)

	first, err := reg.For("user-1", "a@x.com")
	if err != nil {
		t.Fatalf("For user-1: %v", err)
	}
	for _, id := range []string{"user-2", "user-3"} {
		if _, err := reg.For(id, "a@x.com"); err != nil {
			t.Fatalf("For %s: %v", id, err)
		}
	}
	if len(reg.open) != 2 {
		t.Fatalf("open islands = %d; want 2 (bounded by maxOpen)", len(reg.open))
	}
	if _, ok := reg.open["user-1"]; ok {
		t.Error("user-1 still open after eviction; want evicted (LRU)")
	}

	// Touching user-1 reopens it and evicts user-2 (the new LRU).
	reopened, err := reg.For("user-1", "a@x.com")
	if err != nil {
		t.Fatalf("reopen user-1: %v", err)
	}
	if reopened == first {
		t.Error("reopened user-1 = old pointer; want a fresh island after eviction")
	}
	if _, ok := reg.open["user-2"]; ok {
		t.Error("user-2 still open; want evicted")
	}
}

func TestLegacyImportAdminClaimsOnce(t *testing.T) {
	root := t.TempDir()
	legacyCfg := `{"first_name":"Ada","email":"ada@example.com"}`
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte(legacyCfg), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	legacyStore, err := store.OpenAt(filepath.Join(root, "applications.db"))
	if err != nil {
		t.Fatalf("open legacy store: %v", err)
	}
	if err := legacyStore.Insert(store.Application{
		Provider: "greenhouse", Company: "Legacy Co", Role: "Old Role",
		URL: "https://jobs.example.com/legacy", Status: store.StatusApplied,
		AppliedAt: time.Now().UTC(), PostedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed legacy app: %v", err)
	}
	_ = legacyStore.Close()

	islands := filepath.Join(root, "users")
	admins := []string{"Ada@Example.com"} // case-insensitive admin match
	reg := NewRegistry(islands, admins, 0)

	// Admin claims the legacy data on first login.
	st, err := reg.For("user-admin", "ada@example.com")
	if err != nil {
		t.Fatalf("For admin: %v", err)
	}
	apps, err := st.Apps.List()
	if err != nil || len(apps) != 1 || apps[0].Company != "Legacy Co" {
		t.Errorf("admin island apps = %d (err %v); want the legacy row", len(apps), err)
	}
	if st.Cfg.FirstName != "Ada" {
		t.Errorf("admin island config first_name = %q; want Ada", st.Cfg.FirstName)
	}
	if _, err := os.Stat(filepath.Join(st.Dir, ".legacy-imported")); err != nil {
		t.Errorf("import marker missing: %v", err)
	}
	// Legacy originals are untouched.
	if _, err := os.Stat(filepath.Join(root, "config.json")); err != nil {
		t.Errorf("legacy config was moved/deleted: %v", err)
	}

	// A non-admin never sees legacy data (their island starts empty).
	st2, err := reg.For("user-nonadmin", "bob@example.com")
	if err != nil {
		t.Fatalf("For non-admin: %v", err)
	}
	if st2.Cfg.FirstName != "" {
		t.Errorf("non-admin island first_name = %q; want empty (no legacy import)", st2.Cfg.FirstName)
	}
	if _, err := os.Stat(filepath.Join(st2.Dir, "config.json")); !os.IsNotExist(err) {
		t.Errorf("non-admin island config exists; want no import: %v", err)
	}

	// Reopening the SAME island through a fresh registry is a no-op: the marker
	// prevents the (now corrupted) legacy source from being re-imported.
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte("corrupt"), 0o600); err != nil {
		t.Fatalf("corrupt legacy config: %v", err)
	}
	reg2 := NewRegistry(islands, admins, 0)
	reopened, err := reg2.For("user-admin", "ada@example.com")
	if err != nil {
		t.Fatalf("reopen claimed island after corrupting source: %v", err)
	}
	if reopened.Cfg.FirstName != "Ada" {
		t.Errorf("reopened admin island first_name = %q; want Ada (marker held)", reopened.Cfg.FirstName)
	}
	reg2.Close()
}

func TestSafeID(t *testing.T) {
	ok := []string{"user-1", "abc", "11111111-aaaa-4bbb-8ccc-000000000001", "A.B_c-9"}
	for _, id := range ok {
		if !safeID(id) {
			t.Errorf("safeID(%q) = false; want true", id)
		}
	}
	bad := []string{"", ".", "..", "a/b", "a\\b", "a b", "ab\x00",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	for _, id := range bad {
		if safeID(id) {
			t.Errorf("safeID(%q) = true; want false", id)
		}
	}
}

func TestRegistryClose(t *testing.T) {
	reg := NewRegistry(filepath.Join(t.TempDir(), "users"), nil, 0)
	if _, err := reg.For("user-1", "a@x.com"); err != nil {
		t.Fatalf("For: %v", err)
	}
	reg.Close()
	if len(reg.open) != 0 {
		t.Errorf("open islands after Close = %d; want 0", len(reg.open))
	}
}
