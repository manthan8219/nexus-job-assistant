package smartrecruiters

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

type srCandidatePayload struct {
	FirstName   string     `json:"firstName"`
	LastName    string     `json:"lastName"`
	Email       string     `json:"email"`
	PhoneNumber string     `json:"phoneNumber"`
	Web         srWebLinks `json:"web"`
	Resume      *srResume  `json:"resume,omitempty"`
}

type srWebLinks struct {
	LinkedIn string `json:"linkedin,omitempty"`
}

type srResume struct {
	FileName string `json:"fileName"`
	Data     string `json:"data"` // base64-encoded
}

// submitApplication POSTs a candidate application to the SmartRecruiters API.
func (c *Client) submitApplication(ctx context.Context, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
	payload := srCandidatePayload{
		FirstName:   profile.FirstName,
		LastName:    profile.LastName,
		Email:       profile.Email,
		PhoneNumber: profile.Phone,
		Web: srWebLinks{
			LinkedIn: "https://linkedin.com/in/" + profile.LinkedInID,
		},
	}

	if profile.ResumePath != "" {
		resume, err := encodeResume(profile.ResumePath)
		if err != nil {
			return provider.ApplyResult{Status: "failed", Reason: fmt.Sprintf("encode resume: %v", err)}, nil
		}
		payload.Resume = resume
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return provider.ApplyResult{}, err
	}

	url := fmt.Sprintf("%s/v1/companies/%s/postings/%s/candidates", c.apiBase, job.Board, job.ID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return provider.ApplyResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return provider.ApplyResult{Status: "applied"}, nil
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
		return provider.ApplyResult{
			Status: "skipped",
			Reason: fmt.Sprintf("SmartRecruiters apply requires employer credentials — apply manually at: %s", job.URL),
		}, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return provider.ApplyResult{
		Status: "failed",
		Reason: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody)),
	}, nil
}

// encodeResume reads a resume file and returns a base64-encoded srResume struct.
func encodeResume(path string) (*srResume, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open resume %q: %w", path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read resume %q: %w", path, err)
	}

	return &srResume{
		FileName: filepath.Base(path),
		Data:     base64.StdEncoding.EncodeToString(data),
	}, nil
}
