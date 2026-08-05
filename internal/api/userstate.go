// Per-request user-state resolution: when auth is enabled, withAuth attaches
// the authenticated user's data island (internal/userstore) to the request
// context, and the accessors below route every handler's data reads/writes to
// it. When auth is disabled the accessors fall back to the legacy process-level
// singletons, so local/TUI/docker-compose mode is byte-for-byte unchanged.

package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/companies"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/contacts"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
	"github.com/manthan8219/nexus-job-assistant/internal/userstore"
)

type userStateKey struct{}

// withUserState attaches st (the authenticated user's island) to ctx.
func withUserState(ctx context.Context, st *userstore.Store) context.Context {
	return context.WithValue(ctx, userStateKey{}, st)
}

// userState returns the request's user island, or nil in legacy mode.
func (s *Server) userState(r *http.Request) *userstore.Store {
	st, _ := r.Context().Value(userStateKey{}).(*userstore.Store)
	return st
}

// cfgFor returns the config this request should read/write: the user's island
// config when auth is enabled, the legacy singleton otherwise.
func (s *Server) cfgFor(r *http.Request) *config.Config {
	if st := s.userState(r); st != nil {
		return st.Cfg
	}
	return s.cfg
}

// storeFor returns the store this request should read/write.
func (s *Server) storeFor(r *http.Request) *store.Store {
	if st := s.userState(r); st != nil {
		return st.Apps
	}
	return s.store
}

// contactsFor returns the saved-contacts DB for this request.
func (s *Server) contactsFor(r *http.Request) *contacts.DB {
	if st := s.userState(r); st != nil {
		return st.Contacts
	}
	return s.contacts
}

// companiesFor returns the company footprint DB for this request.
func (s *Server) companiesFor(r *http.Request) *companies.DB {
	if st := s.userState(r); st != nil {
		return st.Companies
	}
	return s.companies
}

// saveConfigFor persists cfg to the requesting user's island config path (or
// the legacy default location when auth is disabled).
func (s *Server) saveConfigFor(r *http.Request, cfg *config.Config) error {
	if st := s.userState(r); st != nil {
		return config.SaveTo(cfg, filepath.Join(st.Dir, "config.json"))
	}
	return config.Save(cfg)
}

// adminEmails parses the comma-separated NEXUS_ADMIN_EMAILS env var. These
// emails may claim (once, non-destructively) the legacy single-user data.
func adminEmails() []string {
	var out []string
	for _, e := range strings.Split(os.Getenv("NEXUS_ADMIN_EMAILS"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
}
