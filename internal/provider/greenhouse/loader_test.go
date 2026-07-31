package greenhouse

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestFetchForm_RealAPI hits the live Greenhouse job-board renderer to verify
// the loader endpoint, fingerprint, submit path, and question schema parsing.
// Read-only — it submits nothing. Skips gracefully if the network is down.
func TestFetchForm_RealAPI(t *testing.T) {
	client := &http.Client{Timeout: 15 * time.Second}

	// Find a live job via the public jobs API from a few large boards.
	var board string
	var jobs []ghJob
	for _, b := range []string{"airbnb", "duolingo", "discord", "notion"} {
		var err error
		jobs, err = fetchJobs(context.Background(), client, b)
		if err == nil && len(jobs) > 0 {
			board = b
			break
		}
	}
	if len(jobs) == 0 {
		t.Skip("no live jobs available to test form loading")
	}

	var (
		form *FormInfo
		fErr error
	)
	// Try a few postings — some may have the embed form disabled.
	for _, j := range jobs[:min(5, len(jobs))] {
		form, fErr = FetchForm(context.Background(), client, board, strconv.FormatInt(j.ID, 10))
		if fErr == nil {
			break
		}
	}
	if fErr != nil {
		t.Fatalf("FetchForm failed for 5 live jobs on %s: %v", board, fErr)
	}

	if form.Fingerprint == "" {
		t.Error("empty fingerprint")
	}
	if !strings.Contains(form.SubmitPath, "greenhouse.io") {
		t.Errorf("unexpected SubmitPath %q", form.SubmitPath)
	}
	if len(form.Questions) == 0 {
		t.Error("no questions parsed")
	}
	t.Logf("form: %q @ %q — %d questions, submit → %s",
		form.Title, form.Company, len(form.Questions), form.SubmitPath)
}
