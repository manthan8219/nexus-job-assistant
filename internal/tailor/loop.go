package tailor

import (
	"context"
	"fmt"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

// generate is the testable orchestration core: draft → review → feedback,
// repeated until the HR agent passes the application or rounds run out.
func generate(ctx context.Context, ag agents, in Input) (*Output, error) {
	if strings.TrimSpace(in.Job.Title) == "" {
		return nil, fmt.Errorf("tailor: job title is required")
	}
	if strings.TrimSpace(in.Job.Company) == "" {
		return nil, fmt.Errorf("tailor: company is required")
	}
	resumeText := strings.TrimSpace(in.ResumeText)
	if resumeText == "" && strings.TrimSpace(in.ResumePath) != "" {
		text, err := resume.ExtractText(in.ResumePath)
		if err != nil {
			return nil, fmt.Errorf("tailor: could not read resume: %w", err)
		}
		resumeText = text
	}
	if resumeText == "" {
		return nil, fmt.Errorf("tailor: no resume text — set a resume path in Config")
	}
	maxRounds := in.MaxRounds
	if maxRounds < 1 {
		maxRounds = DefaultMaxRounds
	}
	logf := in.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}

	var (
		feedback  string
		history   []HRReview
		bestScore = -1
		bestCV    resume.ImprovedDoc
		bestCover resume.CoverLetter
		final     HRReview
		passed    bool
	)
	for round := 1; round <= maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		logf("✍️  round %d/%d — writing tailored CV…", round, maxRounds)
		wIn := WriterInput{
			Job: in.Job, ResumeText: resumeText, Profile: in.Profile,
			Projects: in.Projects, Feedback: feedback, Round: round,
		}
		cv, err := ag.cv.Run(ctx, wIn)
		if err != nil {
			return nil, fmt.Errorf("tailor: round %d cv writer: %w", round, err)
		}
		if cv.TargetRole == "" {
			cv.TargetRole = in.Job.Title
		}

		logf("✉️  round %d/%d — writing cover letter…", round, maxRounds)
		wIn.TailoredCVMD = resume.RenderMarkdown(cv)
		cover, err := ag.cover.Run(ctx, wIn)
		if err != nil {
			return nil, fmt.Errorf("tailor: round %d cover writer: %w", round, err)
		}

		logf("🕵️  round %d/%d — HR agent reviewing…", round, maxRounds)
		rev, err := ag.hr.Run(ctx, ReviewInput{
			Job:     in.Job,
			CVMD:    wIn.TailoredCVMD,
			CoverMD: resume.RenderCoverLetterMarkdown(cover),
			Round:   round,
		})
		if err != nil {
			return nil, fmt.Errorf("tailor: round %d hr reviewer: %w", round, err)
		}
		history = append(history, rev)
		logf("    → ATS %d/100 · HR %d/100 · %s — %s",
			rev.ATSScore, rev.HRScore, strings.ToUpper(rev.Verdict), rev.Summary)

		// A passing round always wins over any earlier higher-scored draft.
		if rev.Pass() {
			bestCV, bestCover, final, passed = cv, cover, rev, true
			break
		}
		if score := rev.ATSScore + rev.HRScore; score > bestScore {
			bestScore = score
			bestCV, bestCover, final = cv, cover, rev
		}
		feedback = rev.feedbackBlock()
	}
	if len(history) == 0 {
		return nil, fmt.Errorf("tailor: no drafts produced")
	}

	out, err := writeKit(in, bestCV, bestCover, final, history, passed)
	if err != nil {
		return nil, err
	}
	return out, nil
}
