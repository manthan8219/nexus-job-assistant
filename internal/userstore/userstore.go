// Package userstore owns per-user Nexus data islands: one directory per user
// under NEXUS_HOME/users/<userID>/ holding that user's config.json, SQLite
// databases, and artifacts.
//
// The API resolves the authenticated user's island per request — the identity
// token's `sub` claim is the directory key, so isolation is by construction:
// different user IDs resolve to different directories and nothing in the
// registry ever crosses between them. Local non-auth mode keeps the legacy
// single-directory layout untouched; the islands tree is only used when an
// identity provider is configured.
//
// Islands are opened lazily, held in a bounded registry, and evicted (closed)
// when idle so the per-process file-handle count stays sane. The first session
// of an admin email may claim (copy, never move) the legacy single-user data
// from NEXUS_HOME once, per island.
package userstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/companies"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/contacts"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// ErrNoUser is returned when For is called without a user ID or with one that
// would escape the islands root (defense in depth — IDs are UUIDs).
var ErrNoUser = errors.New("userstore: missing or unsafe user id")

// legacyFiles are the single-user data files that an admin may claim.
var legacyFiles = []string{"config.json", "applications.db", "contacts.db", "companies.db"}

// Store is one authenticated user's open data island.
type Store struct {
	UserID    string
	Dir       string
	Cfg       *config.Config
	Apps      *store.Store
	Contacts  *contacts.DB
	Companies *companies.DB
}

// Close releases every database in the island. Safe to call more than once.
func (s *Store) Close() {
	if s == nil {
		return
	}
	if s.Apps != nil {
		_ = s.Apps.Close()
		s.Apps = nil
	}
	if s.Contacts != nil {
		_ = s.Contacts.Close()
		s.Contacts = nil
	}
	if s.Companies != nil {
		_ = s.Companies.Close()
		s.Companies = nil
	}
}

// Registry lazily opens and evicts per-user islands under a base directory.
// Owned by the API server; safe for concurrent use.
type Registry struct {
	baseDir string
	admins  map[string]bool // admin emails allowed to claim legacy data
	maxOpen int

	mu    sync.Mutex
	open  map[string]*entry
	order []string // user IDs in recency order (LRU head at index 0)
}

type entry struct {
	st      *Store
	lastUse time.Time
}

// NewRegistry returns a Registry rooted at baseDir (normally NEXUS_HOME/users).
// admins are emails that may claim the legacy single-user data once; maxOpen
// bounds concurrently-open islands (<=0 means 32).
func NewRegistry(baseDir string, admins []string, maxOpen int) *Registry {
	if maxOpen <= 0 {
		maxOpen = 32
	}
	am := make(map[string]bool, len(admins))
	for _, e := range admins {
		am[strings.ToLower(strings.TrimSpace(e))] = true
	}
	return &Registry{
		baseDir: baseDir,
		admins:  am,
		maxOpen: maxOpen,
		open:    make(map[string]*entry),
	}
}

// For returns the open island for userID, opening it on first use. The first
// session of an admin email imports the legacy NEXUS_HOME data into the island
// once (non-destructive). The returned Store is owned by the Registry and must
// not be closed by callers.
func (r *Registry) For(userID, email string) (*Store, error) {
	if !safeID(userID) {
		return nil, ErrNoUser
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if e, ok := r.open[userID]; ok {
		e.lastUse = time.Now()
		r.touch(userID)
		return e.st, nil
	}

	dir := filepath.Join(r.baseDir, userID)
	st := &Store{UserID: userID, Dir: dir}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("userstore: mkdir island for %s: %w", userID, err)
	}

	if r.admins[strings.ToLower(strings.TrimSpace(email))] {
		if err := r.importLegacyOnce(dir); err != nil {
			return nil, err
		}
	}

	if err := openIsland(st); err != nil {
		return nil, err
	}
	r.open[userID] = &entry{st: st, lastUse: time.Now()}
	r.order = append(r.order, userID)
	r.evictLocked()
	return st, nil
}

// Close closes every open island and clears the registry. Used on server
// shutdown; callers must not use the registry afterwards.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, e := range r.open {
		e.st.Close()
		delete(r.open, id)
	}
	r.order = nil
}

// touch moves userID to the end of the recency order (MRU).
func (r *Registry) touch(userID string) {
	for i, id := range r.order {
		if id == userID {
			r.order = append(r.order[:i], r.order[i+1:]...)
			r.order = append(r.order, id)
			return
		}
	}
}

// evictLocked closes the least-recently-used island once open count exceeds
// maxOpen. Callers hold r.mu.
func (r *Registry) evictLocked() {
	for len(r.open) > r.maxOpen {
		oldest := r.order[0]
		r.order = r.order[1:]
		if e, ok := r.open[oldest]; ok {
			e.st.Close()
			delete(r.open, oldest)
		}
	}
}

// importLegacyOnce copies the legacy single-user data files from the parent of
// the islands root (NEXUS_HOME) into the island on first use. A marker file
// makes the import idempotent; originals are never deleted.
func (r *Registry) importLegacyOnce(islandDir string) error {
	marker := filepath.Join(islandDir, ".legacy-imported")
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	srcDir := filepath.Dir(r.baseDir)
	for _, name := range legacyFiles {
		src := filepath.Join(srcDir, name)
		data, err := os.ReadFile(src)
		if err != nil {
			continue // absent legacy file is fine
		}
		if err := os.WriteFile(filepath.Join(islandDir, name), data, 0600); err != nil {
			return fmt.Errorf("userstore: import %s: %w", name, err)
		}
	}
	if err := os.WriteFile(marker, []byte("true"), 0600); err != nil {
		return fmt.Errorf("userstore: write import marker: %w", err)
	}
	return nil
}

// openIsland loads the island's config and opens its databases.
func openIsland(st *Store) error {
	cfg, err := config.LoadFrom(filepath.Join(st.Dir, "config.json"))
	if err != nil {
		return fmt.Errorf("userstore: load config for %s: %w", st.UserID, err)
	}
	st.Cfg = cfg

	apps, err := store.OpenAt(filepath.Join(st.Dir, "applications.db"))
	if err != nil {
		st.Close()
		return fmt.Errorf("userstore: open applications for %s: %w", st.UserID, err)
	}
	st.Apps = apps

	ct, err := contacts.Open(filepath.Join(st.Dir, "contacts.db"))
	if err != nil {
		st.Close()
		return fmt.Errorf("userstore: open contacts for %s: %w", st.UserID, err)
	}
	st.Contacts = ct

	co, err := companies.OpenEmbeddedAt(filepath.Join(st.Dir, "companies.db"))
	if err != nil {
		st.Close()
		return fmt.Errorf("userstore: open companies for %s: %w", st.UserID, err)
	}
	st.Companies = co
	return nil
}

// safeID reports whether id is a single, short, path-safe segment. Supabase
// user IDs are UUIDs; the check is defense in depth against path traversal.
func safeID(id string) bool {
	if id == "" || id == "." || id == ".." || len(id) > 128 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}
