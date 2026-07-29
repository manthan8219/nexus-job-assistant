package workable

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/manthanmanthan/nexus/internal/provider"
)

type workableCandidateRequest struct {
	FirstName string `json:"firstname"`
	LastName  string `json:"lastname"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	ResumeURL string `json:"resume_url"`
}

// submitApplication attempts to apply to a Workable job on behalf of the user.
func submitApplication(ctx context.Context, client *http.Client, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
	// Extract job shortcode from the URL (last path segment before trailing slash)
	jobShortcode := extractShortcode(job.URL)
	if jobShortcode == "" {
		return provider.ApplyResult{
			Status: "skipped",
			Reason: fmt.Sprintf("apply manually at %s", job.URL),
		}, nil
	}

	applyURL := fmt.Sprintf("%s/%s/jobs/%s/candidates", workableBaseURL, job.Board, jobShortcode)

	payload := workableCandidateRequest{
		FirstName: profile.FirstName,
		LastName:  profile.LastName,
		Email:     profile.Email,
		Phone:     profile.Phone,
		ResumeURL: "",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return provider.ApplyResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, applyURL, bytes.NewReader(body))
	if err != nil {
		return provider.ApplyResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return provider.ApplyResult{Status: "applied"}, nil
	}

	// Workable requires extensive form data — fall back to manual apply
	respBody, _ := io.ReadAll(resp.Body)
	_ = respBody

	return provider.ApplyResult{
		Status: "skipped",
		Reason: fmt.Sprintf("apply manually at %s", job.URL),
	}, nil
}

// extractShortcode parses the Workable job shortcode from the job URL.
// URL format: https://apply.workable.com/company/j/SHORTCODE/
func extractShortcode(jobURL string) string {
	// Remove trailing slash and split
	trimmed := strings.TrimSuffix(jobURL, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
