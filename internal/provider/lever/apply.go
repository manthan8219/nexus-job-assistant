package lever

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// submitApplication attempts to apply to a Lever job on behalf of the user.
func (c *Client) submitApplication(ctx context.Context, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
	applyURL := fmt.Sprintf("%s/%s/%s/apply", c.baseURL, job.Board, job.ID)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	fields := map[string]string{
		"name":           profile.FirstName + " " + profile.LastName,
		"email":          profile.Email,
		"phone":          profile.Phone,
		"org":            profile.City, // company/org field — use city as fallback
		"urls[LinkedIn]": "https://linkedin.com/in/" + profile.LinkedInID,
	}

	for name, value := range fields {
		if err := w.WriteField(name, value); err != nil {
			return provider.ApplyResult{}, err
		}
	}

	if profile.ResumePath != "" {
		if err := attachFile(w, "resume", profile.ResumePath); err != nil {
			return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
		}
	}

	if err := w.Close(); err != nil {
		return provider.ApplyResult{}, err
	}

	// Lever follows redirect on success — use a client that does NOT follow redirects
	// so we can detect the 302 ourselves.
	noRedirectClient := &http.Client{
		Timeout: c.http.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, applyURL, &body)
	if err != nil {
		return provider.ApplyResult{}, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

	resp, err := noRedirectClient.Do(req)
	if err != nil {
		return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
	}
	defer resp.Body.Close()

	// 200, 201, or 302 redirect all indicate success on Lever
	if resp.StatusCode == http.StatusOK ||
		resp.StatusCode == http.StatusCreated ||
		resp.StatusCode == http.StatusFound {
		return provider.ApplyResult{Status: "applied"}, nil
	}

	// 4xx (except 429) — direct applicant to apply manually. 429 is
	// transient and reported as a failure the engine can retry later.
	if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
		return provider.ApplyResult{
			Status: "skipped",
			Reason: fmt.Sprintf("apply manually at %s", job.URL),
		}, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return provider.ApplyResult{
		Status: "failed",
		Reason: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody)),
	}, nil
}

// attachFile writes a file field to the multipart writer.
func attachFile(w *multipart.Writer, fieldName, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open resume %q: %w", path, err)
	}
	defer f.Close()

	part, err := w.CreateFormFile(fieldName, filepath.Base(path))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, f)
	return err
}
