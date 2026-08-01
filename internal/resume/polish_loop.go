package resume

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/workcontext"
)

const (
	polishDefaultMaxRounds  = 3
	polishPassATS           = 78
	polishPassQuality       = 75
	polishCreatorMaxTokens  = 4096
	polishAssessorMaxTokens = 1024
)

// PolishReview is the assessor agent's verdict on one improved resume draft.
type PolishReview struct {
	Verdict        string   `json:"verdict"`
	ATSScore       int      `json:"ats_score"`
	QualityScore   int      `json:"quality_score"`
	WouldShortlist bool     `json:"would_shortlist"`
	AntiPatterns   []string `json:"anti_patterns_found"`
	MissingMetrics []string `json:"missing_metrics"`
	Issues         []string `json:"issues"`
	Feedback       string   `json:"feedback"`
	Summary        string   `json:"summary"`
}

// Pass reports whether the assessor verdict clears the bar.
func (r PolishReview) Pass() bool {
	if strings.EqualFold(strings.TrimSpace(r.Verdict), "pass") {
		return true
	}
	return r.ATSScore >= polishPassATS && r.QualityScore >= polishPassQuality
}

func (r PolishReview) feedbackBlock() string {
	var b strings.Builder
	b.WriteString("ASSESSOR FEEDBACK — address EVERY item below. Do not repeat the same patterns.\n\n")
	fmt.Fprintf(&b, "Previous scores — ATS: %d/100, Quality: %d/100\n", r.ATSScore, r.QualityScore)
	if len(r.AntiPatterns) > 0 {
		b.WriteString("Anti-patterns to eliminate:\n")
		for _, p := range r.AntiPatterns {
			fmt.Fprintf(&b, "  * %s\n", p)
		}
	}
	if len(r.MissingMetrics) > 0 {
		b.WriteString("Bullets to quantify (add numbers, scale, or time saved):\n")
		for _, m := range r.MissingMetrics {
			fmt.Fprintf(&b, "  * %s\n", m)
		}
	}
	if len(r.Issues) > 0 {
		b.WriteString("Issues ordered by impact:\n")
		for i, it := range r.Issues {
			fmt.Fprintf(&b, "%d. %s\n", i+1, it)
		}
	}
	if r.Feedback != "" {
		fmt.Fprintf(&b, "\nGuidance: %s\n", r.Feedback)
	}
	return b.String()
}

// polishGenerate runs the creator → assessor feedback loop and returns the
// best improved resume and its final assessment.
func polishGenerate(ctx context.Context, ai AIOptions, in ImproveInput, logf func(string, ...any)) (ImprovedDoc, PolishReview, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	var (
		feedback  string
		bestScore = -1
		bestDoc   ImprovedDoc
		final     PolishReview
	)

	for round := 1; round <= polishDefaultMaxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return ImprovedDoc{}, PolishReview{}, err
		}

		logf("round %d/%d — rewriting resume…", round, polishDefaultMaxRounds)
		raw, err := runCreator(ctx, ai, in, feedback)
		if err != nil {
			return ImprovedDoc{}, PolishReview{}, fmt.Errorf("polish round %d creator: %w", round, err)
		}
		doc, err := parseImproved(raw)
		if err != nil {
			return ImprovedDoc{}, PolishReview{}, fmt.Errorf("polish round %d parse: %w", round, err)
		}
		if doc.TargetRole == "" {
			doc.TargetRole = strings.TrimSpace(in.TargetRole)
		}

		logf("round %d/%d — assessing quality…", round, polishDefaultMaxRounds)
		rawRev, err := runAssessor(ctx, ai, strings.TrimSpace(in.TargetRole), RenderMarkdown(doc))
		if err != nil {
			logf("assessor error (keeping best draft so far): %v", err)
			if bestDoc.Summary == "" {
				bestDoc = doc
			}
			break
		}
		rev, err := parsePolishReview(rawRev)
		if err != nil {
			logf("assessor parse error: %v", err)
			if bestDoc.Summary == "" {
				bestDoc = doc
			}
			break
		}
		logf("ATS %d/100 · Quality %d/100 · %s — %s",
			rev.ATSScore, rev.QualityScore, strings.ToUpper(rev.Verdict), rev.Summary)

		if rev.Pass() {
			bestDoc, final = doc, rev
			break
		}
		if score := rev.ATSScore + rev.QualityScore; score > bestScore {
			bestScore = score
			bestDoc, final = doc, rev
		}
		feedback = rev.feedbackBlock()
	}

	if bestDoc.Summary == "" && len(bestDoc.Experience) == 0 {
		return ImprovedDoc{}, PolishReview{}, fmt.Errorf("polish: no usable draft produced")
	}
	return bestDoc, final, nil
}

func runCreator(ctx context.Context, ai AIOptions, in ImproveInput, feedback string) (string, error) {
	fb := strings.TrimSpace(feedback)
	if fb == "" {
		fb = "This is the first draft — no prior assessor feedback."
	}
	targetRole := strings.TrimSpace(in.TargetRole)
	if targetRole == "" {
		targetRole = "the strongest realistic role from the AI profile and resume"
	}
	user := fmt.Sprintf(polishCreatorUserFmt,
		targetRole,
		TrimForPrompt(in.ResumeText, 12000),
		polishProjectsBlock(in.Projects),
		polishProfileJSON(in.Profile),
		polishSkillsBlock(in.Skills),
		fb,
		polishCreatorContract,
	)
	return completeFull(ctx, ai, polishCreatorSystem, user, polishCreatorMaxTokens)
}

func polishSkillsBlock(skills []string) string {
	if len(skills) == 0 {
		return "(no skills explicitly listed — infer from resume and work context)"
	}
	return strings.Join(skills, " · ")
}

func runAssessor(ctx context.Context, ai AIOptions, targetRole, cvMD string) (string, error) {
	if targetRole == "" {
		targetRole = "software/engineering"
	}
	user := fmt.Sprintf(polishAssessorUserFmt,
		targetRole,
		TrimForPrompt(cvMD, 8000),
		polishAssessorContract,
	)
	return completeFull(ctx, ai, polishAssessorSystem, user, polishAssessorMaxTokens)
}

func parsePolishReview(raw string) (PolishReview, error) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}
	var rev PolishReview
	if err := json.Unmarshal([]byte(raw), &rev); err != nil {
		return PolishReview{}, fmt.Errorf("assessor returned invalid JSON: %w", err)
	}
	rev.Verdict = strings.ToLower(strings.TrimSpace(rev.Verdict))
	if rev.Verdict != "pass" && rev.Verdict != "revise" {
		if rev.ATSScore >= polishPassATS && rev.QualityScore >= polishPassQuality {
			rev.Verdict = "pass"
		} else {
			rev.Verdict = "revise"
		}
	}
	rev.ATSScore = polishClamp(rev.ATSScore)
	rev.QualityScore = polishClamp(rev.QualityScore)
	if rev.Summary == "" && rev.Feedback == "" && len(rev.Issues) == 0 {
		return PolishReview{}, fmt.Errorf("empty assessor review from model")
	}
	return rev, nil
}

func polishClamp(n int) int {
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}

func polishProjectsBlock(projects []workcontext.Project) string {
	if len(projects) == 0 {
		return "(no work context provided — rely on the source resume alone)"
	}
	return FormatProjects(projects)
}

func polishProfileJSON(p *Profile) string {
	if p == nil || p.Summary == "" {
		return "(no AI profile available — infer from the resume only)"
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "(AI profile unavailable)"
	}
	return string(b)
}
