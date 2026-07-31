// Package tailor generates job-tailored CVs and cover letters, reviewed by
// an HR agent in a feedback loop, and renders the winning draft to
// LaTeX/PDF. It is built on the shared Eino agent foundation
// (internal/agentx): a writer agent drafts, an HR agent reviews, and the
// loop repeats with the HR feedback until the application passes or the
// round budget runs out.
package tailor

import (
	"context"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
	"github.com/manthan8219/nexus-job-assistant/internal/workcontext"
)

// Pass thresholds for the HR review loop. A draft also passes immediately
// when the HR agent's verdict is "pass" regardless of score.
const (
	DefaultMaxRounds = 3
	passATSScore     = 75
	passHRScore      = 70
)

// WriterInput feeds the writer agents (CV and cover letter) for one round.
type WriterInput struct {
	Job          provider.Job
	ResumeText   string
	Profile      *resume.Profile
	Projects     []workcontext.Project
	TailoredCVMD string // cover-letter writer only: this round's tailored CV
	Feedback     string // HR feedback from the previous round ("" on round 1)
	Round        int
}

// ReviewInput feeds the HR reviewer agent for one round.
type ReviewInput struct {
	Job     provider.Job
	CVMD    string // tailored CV rendered as Markdown — what the recruiter reads
	CoverMD string // cover letter rendered as Markdown
	Round   int
}

// The three agent interfaces the orchestrator composes. *agentx.Agent
// satisfies each directly; tests inject fakes.
type cvWriter interface {
	Run(context.Context, WriterInput) (resume.ImprovedDoc, error)
}
type coverWriter interface {
	Run(context.Context, WriterInput) (resume.CoverLetter, error)
}
type hrReviewer interface {
	Run(context.Context, ReviewInput) (HRReview, error)
}

// agents is the trio the pipeline drives per round.
type agents struct {
	cv    cvWriter
	cover coverWriter
	hr    hrReviewer
}
