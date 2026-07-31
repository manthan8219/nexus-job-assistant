package tailor

import (
	"context"
	"fmt"

	"github.com/manthan8219/nexus-job-assistant/internal/agentx"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
	"github.com/manthan8219/nexus-job-assistant/internal/workcontext"
)

// Input is everything the tailor pipeline needs for one job application.
type Input struct {
	Job             provider.Job
	ResumePath      string // used when ResumeText is empty
	ResumeText      string
	Profile         *resume.Profile
	Projects        []workcontext.Project
	MaxRounds       int                              // 0 → config tailor_max_rounds → DefaultMaxRounds
	OutDir          string                           // empty → ~/.nexus/tailored/<company-role-ts>
	RegisterLibrary bool                             // also register the final CV in the resume library
	Logf            func(format string, args ...any) // optional progress logger
}

// Output is the generated application kit on disk plus the final HR verdict.
type Output struct {
	Dir        string
	ResumePDF  string
	ResumeTeX  string
	ResumeMD   string
	ResumeJSON string
	CoverPDF   string
	CoverTeX   string
	CoverMD    string
	ReviewJSON string
	Review     HRReview   // passing review, or best-scoring one if never passed
	History    []HRReview // every round's review, in order
	Rounds     int
	Passed     bool // HR agent approved within MaxRounds
	PDFNote    string
}

// Generate builds the Eino chat model and agents from config, then runs the
// writer → HR-review loop and writes the winning kit to disk.
func Generate(ctx context.Context, cfg *config.Config, in Input) (*Output, error) {
	if cfg == nil {
		return nil, fmt.Errorf("tailor: nil config")
	}
	m, err := agentx.NewChatModel(ctx, cfg)
	if err != nil {
		return nil, err
	}
	ag, err := newAgents(m)
	if err != nil {
		return nil, err
	}
	if in.MaxRounds < 1 && cfg.TailorMaxRounds > 0 {
		in.MaxRounds = cfg.TailorMaxRounds
	}
	return generate(ctx, ag, in)
}
