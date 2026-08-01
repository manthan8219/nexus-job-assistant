package ashby

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// errOrgKeyNotFound marks a board page that does not expose the org API key
// (a content condition — distinct from a transport failure, which should be
// reported as a hard failure rather than handed off for manual apply).
var errOrgKeyNotFound = errors.New("ashby: org apiKey not found in page HTML")

// fetchOrgAPIKey fetches the Ashby org API key embedded in the job board page HTML.
// The key appears in the __NEXT_DATA__ script as "apiKey":"<key>".
func (c *Client) fetchOrgAPIKey(ctx context.Context, slug string) (string, error) {
	url := fmt.Sprintf("%s/%s", c.jobsHost, slug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	html := string(body)
	marker := `"apiKey":"`
	idx := strings.Index(html, marker)
	if idx == -1 {
		return "", errOrgKeyNotFound
	}

	start := idx + len(marker)
	end := strings.Index(html[start:], `"`)
	if end == -1 {
		return "", errOrgKeyNotFound
	}

	key := html[start : start+end]
	if key == "" {
		return "", errOrgKeyNotFound
	}
	return key, nil
}

// submitApplication attempts to apply to an Ashby job on behalf of the user.
func (c *Client) submitApplication(ctx context.Context, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
	apiKey, err := c.fetchOrgAPIKey(ctx, job.Board)
	if err != nil {
		if errors.Is(err, errOrgKeyNotFound) {
			return provider.ApplyResult{
				Status: "skipped",
				Reason: fmt.Sprintf("could not fetch org API key — apply manually at: %s", job.URL),
			}, nil
		}
		return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	fields := map[string]string{
		"apiKey":       apiKey,
		"jobPostingId": job.ID,
		"applicationForm[_systemfield_firstName]": profile.FirstName,
		"applicationForm[_systemfield_lastName]":  profile.LastName,
		"applicationForm[_systemfield_email]":     profile.Email,
		"applicationForm[_systemfield_phone]":     profile.Phone,
		"applicationForm[_systemfield_linkedIn]":  "https://linkedin.com/in/" + profile.LinkedInID,
	}

	for name, value := range fields {
		if err := w.WriteField(name, value); err != nil {
			return provider.ApplyResult{}, err
		}
	}

	if profile.ResumePath != "" {
		if err := attachFile(w, "applicationForm[_systemfield_resume]", profile.ResumePath); err != nil {
			return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
		}
	}

	if err := w.Close(); err != nil {
		return provider.ApplyResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiHost+"/applicationForm.submit", &body)
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

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return provider.ApplyResult{Status: "applied"}, nil
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
