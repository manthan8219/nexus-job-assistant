package greenhouse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// Greenhouse exposes two public surfaces:
//
//  1. The Job Board API (boards-api.greenhouse.io) — read-only for job
//     seekers. Its application-submission endpoint requires the *employer's*
//     Job Board API key (HTTP Basic); job seekers cannot use it (verified:
//     unauthenticated POSTs are answered 401).
//
//  2. The job-board renderer (boards.greenhouse.io / job-boards.greenhouse.io)
//     — the Remix web app humans use to apply. Its form-loader and
//     submission endpoints are public and need no API key. This file and
//     apply.go/upload.go implement that flow:
//
//     GET  /embed/job_app?for={board}&token={id}&_data=routes/embed.job_app
//     → form schema (questions), submitPath, anti-replay "fingerprint"
//     GET  /uncacheable_attributes/presigned_fields?fields[]=resume
//     → S3 presigned POST data for the resume upload (see upload.go)
//     POST {submitPath}  JSON {"job_application": {...}, "fingerprint": ...}
//     → submits the application (see apply.go)
//
// Most boards additionally enforce an invisible reCAPTCHA Enterprise check at
// submit time (HTTP 400 when the token is missing, 428 when it is invalid).
// Boards with captcha enabled can only be submitted through a real browser —
// see browser.go.
var (
	// boardsBaseURL serves the public job-board renderer (embed forms,
	// presigned-field endpoint, submission). A var so tests can swap it.
	boardsBaseURL = "https://boards.greenhouse.io"

	// jobBoardsBaseURL is the newer hosted board UI; used as a fallback
	// loader source when a board has the legacy embed form disabled.
	jobBoardsBaseURL = "https://job-boards.greenhouse.io"
)

// rendererUA identifies us as a regular browser to the renderer endpoints —
// the same requests Greenhouse's own front-end makes.
const rendererUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// FormInfo is everything discovered about a Greenhouse job's public apply form.
type FormInfo struct {
	Board       string
	JobID       string
	Title       string
	Company     string
	Location    string
	SubmitPath  string // where the application JSON is POSTed
	Fingerprint string // anti-replay token that must accompany the POST
	Questions   []ghQuestion
}

// FetchForm loads the public application form schema for a job — the same
// JSON the browser receives from the Remix loader when a human opens the
// apply page. It is a read-only GET and submits nothing.
func FetchForm(ctx context.Context, client *http.Client, board, jobID string) (*FormInfo, error) {
	urls := []string{
		// Legacy embed form (still the canonical apply surface for most boards).
		fmt.Sprintf("%s/embed/job_app?for=%s&token=%s&_data=%s",
			boardsBaseURL, url.QueryEscape(board), url.QueryEscape(jobID),
			url.QueryEscape("routes/embed.job_app")),
		// Newer hosted board page.
		fmt.Sprintf("%s/%s/jobs/%s?_data=%s",
			jobBoardsBaseURL, url.PathEscape(board), url.PathEscape(jobID),
			url.QueryEscape("routes/$url_token_.jobs_.$job_post_id")),
	}

	var errs []error
	for _, u := range urls {
		info, err := fetchLoader(ctx, client, u, board, jobID)
		if err == nil {
			return info, nil
		}
		errs = append(errs, err)
	}
	return nil, errors.Join(errs...)
}

func fetchLoader(ctx context.Context, client *http.Client, u, board, jobID string) (*FormInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", rendererUA)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("greenhouse form loader %s/%s: HTTP %d (%s)", board, jobID, resp.StatusCode, resp.Request.URL)
	}

	var lr loaderResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, fmt.Errorf("greenhouse form loader %s/%s: decode: %w", board, jobID, err)
	}
	if lr.SubmitPath == "" || lr.JobPost.Fingerprint == "" || len(lr.JobPost.Questions) == 0 {
		return nil, fmt.Errorf("greenhouse form loader %s/%s: incomplete form data", board, jobID)
	}

	return &FormInfo{
		Board:       board,
		JobID:       jobID,
		Title:       lr.JobPost.Title,
		Company:     lr.JobPost.CompanyName,
		Location:    lr.JobPost.Location,
		SubmitPath:  lr.SubmitPath,
		Fingerprint: lr.JobPost.Fingerprint,
		Questions:   lr.JobPost.Questions,
	}, nil
}

// EmbedFormURL returns the user-visible apply form URL (used by the browser flow).
func EmbedFormURL(board, jobID string) string {
	return fmt.Sprintf("%s/embed/job_app?for=%s&token=%s",
		jobBoardsBaseURL, url.QueryEscape(board), url.QueryEscape(jobID))
}
