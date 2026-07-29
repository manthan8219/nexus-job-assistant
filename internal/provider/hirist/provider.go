// Package hirist is a placeholder provider for hirist.com.
//
// Status: NOT YET IMPLEMENTED.
//
// No official public API, but its internal API is hit by browser-based
// scrapers and third-party tools (found via network-tab inspection), so
// an unauthenticated JSON endpoint likely exists. ToS scraping
// restrictions not confirmed either way at research time (2026-07-29) —
// verify before enabling.
//
// TODO before shipping:
//   - Find the internal listing endpoint and response shape.
//   - Confirm pagination and any rate limiting.
//   - Re-check ToS before enabling by default.
package hirist

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

// Client implements provider.Provider for Hirist.
type Client struct {
	http *http.Client
}

// New creates a Hirist client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "hirist" }

// Search is not yet implemented.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	return nil, errors.New("hirist: provider not yet implemented")
}

// Apply marks as skipped — jobs link to the Hirist posting page.
func (c *Client) Apply(ctx context.Context, j provider.Job, p provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually at " + j.URL}, nil
}
