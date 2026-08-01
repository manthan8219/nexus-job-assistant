package recruitee

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// submitApplication applies to a Recruitee job through the public Careers
// Site API (no authentication required):
//
//	POST https://{board}.recruitee.com/api/offers/{offerSlug}/candidates
//
// The candidate payload is multipart/form-data (name, email, phone and the
// resume file) mirroring the board's own apply form. A 2xx response means
// the application was accepted; a 4xx rejection is mapped to "skipped" so
// the applicant is directed to finish the form manually (e.g. when the
// offer requires screening questions this tool cannot answer); 5xx and
// transport errors are "failed".
func (c *Client) submitApplication(ctx context.Context, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
	offerSlug := extractOfferSlug(job.URL)
	if offerSlug == "" {
		return provider.ApplyResult{
			Status: "skipped",
			Reason: fmt.Sprintf("could not determine offer slug — apply manually at %s", job.URL),
		}, nil
	}

	base := c.applyHost
	if base == "" {
		base = fmt.Sprintf("https://%s.recruitee.com", job.Board)
	}
	applyURL := fmt.Sprintf("%s/api/offers/%s/candidates", base, offerSlug)

	if strings.TrimSpace(profile.ResumePath) == "" {
		// The Careers Site API requires a CV by default; without one the
		// submission would be rejected, so fail fast and hand off manually.
		return provider.ApplyResult{
			Status: "skipped",
			Reason: fmt.Sprintf("no resume configured — apply manually at %s", job.URL),
		}, nil
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	for name, value := range map[string]string{
		"candidate[name]":  strings.TrimSpace(profile.FirstName + " " + profile.LastName),
		"candidate[email]": strings.TrimSpace(profile.Email),
		"candidate[phone]": strings.TrimSpace(profile.Phone),
	} {
		if value == "" {
			continue
		}
		if err := w.WriteField(name, value); err != nil {
			return provider.ApplyResult{}, err
		}
	}

	// candidate[cv] carries the resume file — the same field the hosted
	// apply form posts.
	if err := attachResume(w, profile.ResumePath); err != nil {
		return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
	}

	if err := w.Close(); err != nil {
		return provider.ApplyResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, applyURL, &body)
	if err != nil {
		return provider.ApplyResult{}, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return provider.ApplyResult{Status: "applied"}, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	// 4xx rejections (except 429) mean the board won't accept this payload —
	// hand off to the applicant to finish manually. 429 is transient, so it
	// is reported as a failure the engine can retry later.
	if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
		return provider.ApplyResult{
			Status: "skipped",
			Reason: fmt.Sprintf("recruitee rejected the application (HTTP %d) — apply manually at %s", resp.StatusCode, job.URL),
		}, nil
	}
	return provider.ApplyResult{
		Status: "failed",
		Reason: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody))),
	}, nil
}

// extractOfferSlug parses the offer slug out of a Recruitee careers URL.
// Supported formats:
//
//	https://{company}.recruitee.com/o/{slug}
//	https://{company}.recruitee.com/l/offers/{slug}
//
// Query parameters (e.g. ?token=...) are ignored. The empty string is
// returned when no slug can be found.
func extractOfferSlug(careersURL string) string {
	u := strings.TrimSpace(careersURL)
	if i := strings.Index(u, "?"); i != -1 {
		u = u[:i]
	}
	u = strings.TrimSuffix(u, "/")

	var slug string
	if i := strings.LastIndex(u, "/l/offers/"); i != -1 {
		slug = u[i+len("/l/offers/"):]
	} else if i := strings.LastIndex(u, "/o/"); i != -1 {
		slug = u[i+len("/o/"):]
	}
	return strings.TrimSpace(slug)
}

// attachResume streams the resume file into the multipart body under the
// field name Recruitee's Careers Site API expects.
func attachResume(w *multipart.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open resume %q: %w", path, err)
	}
	defer f.Close()

	part, err := w.CreateFormFile("candidate[cv]", filepath.Base(path))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, f)
	return err
}
