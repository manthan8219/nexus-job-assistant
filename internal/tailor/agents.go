package tailor

import (
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/components/model"

	"github.com/manthan8219/nexus-job-assistant/internal/agentx"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
	"github.com/manthan8219/nexus-job-assistant/internal/workcontext"
)

// newAgents builds the writer + HR agent trio on one shared chat model.
func newAgents(m model.BaseChatModel) (agents, error) {
	cv, err := agentx.New("cv-writer", m, cvWriterSystem, cvWriterTemplate, cvWriterVars, resume.ParseImproved)
	if err != nil {
		return agents{}, err
	}
	cover, err := agentx.New("cover-writer", m, coverWriterSystem, coverWriterTemplate, coverWriterVars, parseCoverLetter)
	if err != nil {
		return agents{}, err
	}
	hr, err := agentx.New("hr-reviewer", m, hrReviewerSystem, hrReviewerTemplate, hrReviewerVars, parseHRReview)
	if err != nil {
		return agents{}, err
	}
	return agents{cv: cv, cover: cover, hr: hr}, nil
}

// ── Prompt variable mappers ────────────────────────────────────────────────
// Templates use {variable} placeholders only; anything containing literal
// braces (JSON contracts, prior feedback) is passed in as a variable value.

func cvWriterVars(in WriterInput) map[string]any {
	return map[string]any{
		"title":          in.Job.Title,
		"company":        in.Job.Company,
		"location":       jobLocation(in.Job),
		"remote":         yesNo(in.Job.Remote),
		"jd":             jobDescription(in.Job),
		"resume":         resume.TrimForPrompt(in.ResumeText, 12000),
		"projects":       projectsOrFallback(in.Projects),
		"profile":        profileBlock(in.Profile),
		"feedback_block": feedbackBlock(in.Feedback),
		"contract":       cvContract,
	}
}

func coverWriterVars(in WriterInput) map[string]any {
	cv := strings.TrimSpace(in.TailoredCVMD)
	if cv == "" {
		cv = resume.TrimForPrompt(in.ResumeText, 8000)
	}
	return map[string]any{
		"title":          in.Job.Title,
		"company":        in.Job.Company,
		"jd":             jobDescription(in.Job),
		"cv":             cv,
		"projects":       projectsOrFallback(in.Projects),
		"feedback_block": feedbackBlock(in.Feedback),
		"contract":       coverContract,
	}
}

func hrReviewerVars(in ReviewInput) map[string]any {
	return map[string]any{
		"title":    in.Job.Title,
		"company":  in.Job.Company,
		"jd":       jobDescription(in.Job),
		"cv_md":    resume.TrimForPrompt(in.CVMD, 12000),
		"cover_md": resume.TrimForPrompt(in.CoverMD, 4000),
		"contract": hrReviewContract,
	}
}

// ── Shared prompt helpers ──────────────────────────────────────────────────

func jobLocation(j provider.Job) string {
	loc := strings.TrimSpace(j.Location)
	if loc == "" && j.Remote {
		return "Remote"
	}
	if loc == "" {
		return "(unspecified)"
	}
	return loc
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func projectsOrFallback(projects []workcontext.Project) string {
	if len(projects) == 0 {
		return "(no work context recorded — rely on the resume alone)"
	}
	return resume.FormatProjects(projects)
}

func feedbackBlock(feedback string) string {
	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		return "This is the first draft — no prior HR feedback."
	}
	return feedback
}

func profileBlock(p *resume.Profile) string {
	if p == nil || p.Summary == "" {
		return "(no AI profile available — infer carefully from the resume only)"
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "(AI profile unavailable)"
	}
	return string(b)
}
