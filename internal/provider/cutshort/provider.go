// Package cutshort is a placeholder provider for cutshort.io.
//
// Status: NOT YET IMPLEMENTED.
//
// No public API found; only third-party scraper products (e.g. Apify
// actors) exist, with no clear evidence of a stable accessible endpoint
// at research time (2026-07-29). Lowest-confidence candidate of the
// India job boards investigated — confirm feasibility before investing
// in a full implementation.
//
// TODO before shipping:
//   - Confirm whether a reachable internal endpoint exists at all.
//   - Find the internal listing endpoint and response shape.
//   - Re-check ToS before enabling by default.
package cutshort

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// Client implements provider.Provider for CutShort.
type Client struct {
	http *http.Client
}

// New creates a CutShort client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "cutshort" }

// Search is not yet implemented.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	return nil, errors.New("cutshort: provider not yet implemented")
}

// Apply marks as skipped — jobs link to the CutShort posting page.
func (c *Client) Apply(ctx context.Context, j provider.Job, p provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually at " + j.URL}, nil
}
