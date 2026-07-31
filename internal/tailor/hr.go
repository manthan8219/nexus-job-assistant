package tailor

import (
	"fmt"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/agentx"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
	"github.com/manthan8219/nexus-job-assistant/internal/textutil"
)

// HRReview is the HR agent's verdict on one tailored application draft.
// Scores are 0-100; Feedback and Issues are consumed verbatim by the writer
// agents on the next round.
type HRReview struct {
	Verdict         string   `json:"verdict"` // "pass" | "revise"
	ATSScore        int      `json:"ats_score"`
	HRScore         int      `json:"hr_score"`
	ATSReady        bool     `json:"ats_ready"`
	WouldInterview  bool     `json:"would_interview"`
	MissingKeywords []string `json:"missing_keywords"`
	Issues          []string `json:"issues"`
	Feedback        string   `json:"feedback"`
	Summary         string   `json:"summary"`
}

// Pass reports whether the review clears the bar to ship the kit: an
// explicit pass verdict, or both scores above the pass thresholds.
func (r HRReview) Pass() bool {
	if strings.EqualFold(strings.TrimSpace(r.Verdict), "pass") {
		return true
	}
	return r.ATSScore >= passATSScore && r.HRScore >= passHRScore
}

// feedbackBlock renders the review as instructions the writer agents consume
// on the next round.
func (r HRReview) feedbackBlock() string {
	var b strings.Builder
	b.WriteString("HR AGENT FEEDBACK FROM THE PREVIOUS REVIEW — address EVERY point in this revision. Do not repeat the same mistakes.\n\n")
	fmt.Fprintf(&b, "Previous scores — ATS: %d/100, HR: %d/100\n", r.ATSScore, r.HRScore)
	if len(r.MissingKeywords) > 0 {
		fmt.Fprintf(&b, "Missing JD keywords (weave in only where truthful): %s\n", strings.Join(r.MissingKeywords, ", "))
	}
	if len(r.Issues) > 0 {
		b.WriteString("Issues to fix, ordered by impact:\n")
		for i, it := range r.Issues {
			fmt.Fprintf(&b, "%d. %s\n", i+1, it)
		}
	}
	if r.Feedback != "" {
		fmt.Fprintf(&b, "Guidance: %s\n", r.Feedback)
	}
	return b.String()
}

// parseHRReview tolerantly parses the HR agent's JSON verdict, normalizing
// the verdict and clamping scores.
func parseHRReview(raw string) (HRReview, error) {
	rev, err := agentx.ParseJSON[HRReview](raw)
	if err != nil {
		return HRReview{}, err
	}
	rev.ATSScore = clamp100(rev.ATSScore)
	rev.HRScore = clamp100(rev.HRScore)
	rev.Verdict = strings.ToLower(strings.TrimSpace(rev.Verdict))
	if rev.Verdict != "pass" && rev.Verdict != "revise" {
		if rev.ATSScore >= passATSScore && rev.HRScore >= passHRScore {
			rev.Verdict = "pass"
		} else {
			rev.Verdict = "revise"
		}
	}
	rev.MissingKeywords = cleanList(rev.MissingKeywords)
	rev.Issues = cleanList(rev.Issues)
	rev.Feedback = strings.TrimSpace(rev.Feedback)
	rev.Summary = strings.TrimSpace(rev.Summary)
	if rev.Summary == "" && rev.Feedback == "" && len(rev.Issues) == 0 {
		return HRReview{}, fmt.Errorf("empty HR review from model")
	}
	return rev, nil
}

// jobDescription returns the plain-text JD for prompts, or an explicit
// fallback so the model knows to judge from title/company alone.
func jobDescription(j provider.Job) string {
	desc := strings.TrimSpace(j.Description)
	if desc == "" {
		return "(no full job description available — judge from title and company only)"
	}
	return resume.TrimForPrompt(textutil.HTMLToPlain(desc), 16000)
}

func clamp100(n int) int {
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
