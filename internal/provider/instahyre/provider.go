// Package instahyre is a placeholder provider for instahyre.com.
//
// Status: NOT YET IMPLEMENTED.
//
// No official public API. Weak evidence of a scrapeable internal JSON
// endpoint (less community tooling than Internshala/Hirist at research
// time, 2026-07-29). ToS scraping restrictions not confirmed either way —
// verify before enabling.
//
// TODO before shipping:
//   - Confirm whether a reachable internal endpoint exists at all.
//   - Find the internal listing endpoint and response shape.
//   - Re-check ToS before enabling by default.
package instahyre

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

// Client implements provider.Provider for Instahyre.
type Client struct {
	http *http.Client
}

// New creates an Instahyre client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "instahyre" }

// Search is not yet implemented.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	return nil, errors.New("instahyre: provider not yet implemented")
}

// Apply marks as skipped — jobs link to the Instahyre posting page.
func (c *Client) Apply(ctx context.Context, j provider.Job, p provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually at " + j.URL}, nil
}
