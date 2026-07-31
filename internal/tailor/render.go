package tailor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

// reviewRecord is the review.json document saved with every kit: the final
// verdict plus the full per-round history.
type reviewRecord struct {
	Job         string     `json:"job"`
	Company     string     `json:"company"`
	URL         string     `json:"url,omitempty"`
	GeneratedAt time.Time  `json:"generated_at"`
	Rounds      int        `json:"rounds"`
	Passed      bool       `json:"passed"`
	Final       HRReview   `json:"final"`
	History     []HRReview `json:"history"`
}

// writeKit renders and persists the winning draft: resume + cover letter as
// Markdown, LaTeX, and PDF, plus review.json. Optionally registers the CV in
// the Nexus resume library.
func writeKit(in Input, cv resume.ImprovedDoc, cover resume.CoverLetter, final HRReview, history []HRReview, passed bool) (*Output, error) {
	dir := strings.TrimSpace(in.OutDir)
	if dir == "" {
		base, err := tailoredDir()
		if err != nil {
			return nil, err
		}
		stamp := time.Now().Format("20060102-150405")
		dir = filepath.Join(base, slugify(in.Job.Company+"-"+in.Job.Title)+"-"+stamp)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("tailor: create kit dir: %w", err)
	}

	out := &Output{
		Dir: dir, Review: final, History: history,
		Rounds: len(history), Passed: passed,
	}
	write := func(name, content string) (string, error) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			return "", fmt.Errorf("tailor: write %s: %w", name, err)
		}
		return p, nil
	}

	var err error
	if out.ResumeMD, err = write("resume.md", resume.RenderMarkdown(cv)); err != nil {
		return nil, err
	}
	if out.ResumeTeX, err = write("resume.tex", resume.RenderLaTeX(cv)); err != nil {
		return nil, err
	}
	docJSON, _ := json.MarshalIndent(cv, "", "  ")
	if out.ResumeJSON, err = write("resume.json", string(docJSON)); err != nil {
		return nil, err
	}
	conv, err := resume.EnsurePDF(cv, out.ResumeMD, out.ResumeTeX, filepath.Join(dir, "resume.pdf"))
	if err != nil {
		return nil, fmt.Errorf("tailor: %w", err)
	}
	out.ResumePDF = conv.PDFPath
	out.PDFNote = conv.Note

	if out.CoverMD, err = write("cover_letter.md", resume.RenderCoverLetterMarkdown(cover)); err != nil {
		return nil, err
	}
	if out.CoverTeX, err = write("cover_letter.tex", resume.RenderCoverLetterLaTeX(cover)); err != nil {
		return nil, err
	}
	coverConv, err := resume.EnsureCoverPDF(cover, out.CoverTeX, filepath.Join(dir, "cover_letter.pdf"))
	if err != nil {
		return nil, fmt.Errorf("tailor: %w", err)
	}
	out.CoverPDF = coverConv.PDFPath

	rec := reviewRecord{
		Job: in.Job.Title, Company: in.Job.Company, URL: in.Job.URL,
		GeneratedAt: time.Now(), Rounds: len(history), Passed: passed,
		Final: final, History: history,
	}
	recJSON, _ := json.MarshalIndent(rec, "", "  ")
	if out.ReviewJSON, err = write("review.json", string(recJSON)); err != nil {
		return nil, err
	}

	if in.RegisterLibrary {
		// Best-effort: the kit is already safely on disk.
		_ = resume.RegisterVersion(resume.Version{
			CreatedAt:  time.Now(),
			Label:      fmt.Sprintf("%s — %s · ATS %d", in.Job.Company, in.Job.Title, final.ATSScore),
			TargetRole: in.Job.Title,
			PDFPath:    out.ResumePDF,
			MDPath:     out.ResumeMD,
			TeXPath:    out.ResumeTeX,
			JSONPath:   out.ResumeJSON,
			Source:     "tailor",
			Notes:      final.Summary,
		})
	}
	return out, nil
}

// tailoredDir is ~/.nexus/tailored, created if needed.
func tailoredDir() (string, error) {
	base, err := config.Dir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "tailored")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

// slugify makes a filesystem-safe slug like "acme-backend-engineer".
func slugify(s string) string {
	s = nonSlugChars.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	if s == "" {
		s = "job"
	}
	return s
}
