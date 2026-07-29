package resume

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
	"github.com/manthanmanthan/nexus/internal/textutil"
)

// FitResult is how likely a resume gets shortlisted for a job.
type FitResult struct {
	Score   int    `json:"score"`   // 0-100
	Summary string `json:"summary"` // why high/low
}

// ScoreJobFit calls the LLM once for one job (keep sequential in the engine).
func ScoreJobFit(ctx context.Context, ai AIOptions, resumeText string, job provider.Job) (FitResult, error) {
	if !ai.Enabled {
		return FitResult{}, fmt.Errorf("AI Assist is off")
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
	}
	raw, err := complete(ctx, ai, fitPrompt(resumeText, job))
	if err != nil {
		return FitResult{}, err
	}
	return parseFit(raw)
}

func fitPrompt(resumeText string, job provider.Job) string {
	// Send as much as practical — score should use full JD + full resume, not a stub.
	resumeText = trimForPrompt(resumeText, 24000)
	desc := strings.TrimSpace(job.Description)
	if desc == "" {
		desc = "(no full job description available — score from title, company, and location only)"
	} else {
		desc = trimForPrompt(textutil.HTMLToPlain(desc), 20000)
	}
	loc := strings.TrimSpace(job.Location)
	remote := "no"
	if job.Remote {
		remote = "yes"
		if loc == "" {
			loc = "Remote"
		}
	}
	return fmt.Sprintf(`You are a recruiter screening resumes for shortlist.

Score how likely THIS candidate's resume gets past the first screen / shortlisted for THIS job.
Use ALL of the job fields and the FULL job description and resume below — do not ignore sections.
Be honest and calibrated — most matches are 40-75; 90+ only when clearly strong.

Job:
- Title: %s
- Company: %s
- Location: %s
- Remote: %s
- Provider: %s

Job description (full posting):
"""
%s
"""

Candidate resume (full text):
"""
%s
"""

Return ONLY compact JSON (no markdown):
{"score":0-100,"summary":"2-4 sentences: main reasons for the score — skills match, gaps, seniority, domain fit."}
`, job.Title, job.Company, loc, remote, job.Provider, desc, resumeText)
}

func parseFit(raw string) (FitResult, error) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}
	var doc FitResult
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return FitResult{}, fmt.Errorf("invalid fit JSON: %w", err)
	}
	if doc.Score < 0 {
		doc.Score = 0
	}
	if doc.Score > 100 {
		doc.Score = 100
	}
	doc.Summary = strings.TrimSpace(doc.Summary)
	if doc.Summary == "" {
		return FitResult{}, fmt.Errorf("empty fit summary")
	}
	return doc, nil
}
