// Package workatastartup is a placeholder provider for workatastartup.com
// (Y Combinator's startup jobs board).
//
// Status: NOT YET IMPLEMENTED.
//
// YC has deliberately not shipped an official public API (per a public
// "Ask HN" thread, partly to keep application quality/volume in check).
// Community tools (jwc20/waasuapi, ycombinator-scraper, Parse.bot) reach
// it via reverse-engineered endpoints, generally through plain requests
// rather than needing stealth-browser tooling — no Cloudflare/DataDome-
// grade defenses were found at research time (2026-07-29), unlike
// Wellfound. Full job details require a logged-in session.
//
// This is meaningfully more fragile than the rest of this codebase's
// providers: it is not a documented API, ToS scraping stance was not
// directly verified, and the endpoint can change or be blocked without
// notice. Treat as best-effort: keep request volume low, cache results,
// and re-check ToS/behavior before relying on it.
//
// TODO before shipping:
//   - Decide how to handle the login requirement (session cookie from
//     config? interactive login flow?) — this needs a YC account.
//   - Find/confirm the internal listing endpoint and response shape.
//   - Keep request volume low; this is not a bulk-friendly target.
//   - Re-verify ToS and endpoint stability periodically.
package workatastartup

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

// Client implements provider.Provider for Work at a Startup.
type Client struct {
	http *http.Client
}

// New creates a Work at a Startup client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "workatastartup" }

// Search is not yet implemented.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	return nil, errors.New("workatastartup: provider not yet implemented")
}

// Apply marks as skipped — jobs link to the Work at a Startup posting page.
func (c *Client) Apply(ctx context.Context, j provider.Job, p provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually at " + j.URL}, nil
}
