package resume

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/nexusdir"
	"github.com/manthan8219/nexus-job-assistant/internal/workcontext"
)

// Format is an export target for an improved resume.
type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatLaTeX    Format = "latex"
	FormatPDF      Format = "pdf"
)

// SupportedFormats is the user-facing order in the Improve UI.
var SupportedFormats = []Format{FormatMarkdown, FormatLaTeX, FormatPDF}

func (f Format) Label() string {
	switch f {
	case FormatMarkdown:
		return "Markdown"
	case FormatLaTeX:
		return "LaTeX"
	case FormatPDF:
		return "PDF"
	default:
		return string(f)
	}
}

func (f Format) Ext() string {
	switch f {
	case FormatMarkdown:
		return ".md"
	case FormatLaTeX:
		return ".tex"
	case FormatPDF:
		return ".pdf"
	default:
		return ".txt"
	}
}

// ImprovedDoc is structured resume content the model returns; we render formats locally.
type ImprovedDoc struct {
	FullName     string         `json:"full_name"`
	Headline     string         `json:"headline"`
	Summary      string         `json:"summary"`
	Skills       []string       `json:"skills"`
	Experience   []ImprovedRole `json:"experience"`
	Education    []string       `json:"education"`
	Notes        []string       `json:"notes"`
	TargetRole   string         `json:"target_role,omitempty"`
	GeneratedAt  time.Time      `json:"generated_at"`
	SourcePath   string         `json:"source_resume,omitempty"`
	ProjectCount int            `json:"project_count"`
}

// ImprovedRole is one job / project block on the rewritten resume.
type ImprovedRole struct {
	Title   string   `json:"title"`
	Org     string   `json:"org"`
	Period  string   `json:"period"`
	Bullets []string `json:"bullets"`
}

// ImproveInput is everything the rewriter needs.
type ImproveInput struct {
	ResumePath string
	ResumeText string
	Profile    *Profile
	Projects   []workcontext.Project
	Skills     []string
	TargetRole string
	Formats    []Format
	TemplateID string // empty → classic
}

// ImproveOutput is files written under ~/.nexus/resumes/.
type ImproveOutput struct {
	Doc          ImprovedDoc
	Review       PolishReview
	Dir          string
	Files        map[Format]string
	PDFNote      string
	PreviewMD    string
	TemplateID   string
	TemplateName string
	Fit          FitPlan // content→template plan + verified page count
	VersionID    string  // library id the PDF was registered under (for inline preview)
}

// GenerateImproved builds a stronger resume from analysis + work context, then exports.
func GenerateImproved(ctx context.Context, ai AIOptions, in ImproveInput) (*ImproveOutput, error) {
	if !ai.Enabled {
		return nil, fmt.Errorf("turn on AI Assist in Config first")
	}
	if strings.TrimSpace(in.ResumeText) == "" && in.ResumePath != "" {
		text, err := ExtractText(in.ResumePath)
		if err != nil {
			return nil, fmt.Errorf("could not read resume: %w", err)
		}
		in.ResumeText = text
	}
	if strings.TrimSpace(in.ResumeText) == "" {
		return nil, fmt.Errorf("no resume text — set a resume path in Config")
	}
	if len(in.Projects) == 0 {
		return nil, fmt.Errorf("add at least one Work project first (Resume → Work)")
	}
	formats := in.Formats
	if len(formats) == 0 {
		formats = []Format{FormatMarkdown, FormatLaTeX}
	}

	tpl, err := GetTemplate(in.TemplateID)
	if err != nil {
		return nil, err
	}

	// Polish loop: AI writes content that follows the template's sections,
	// order, layout, and budget (see polishTemplateBlock). Then the
	// deterministic planner fits the produced doc to the template's caps and
	// estimates the page cost — the pipeline renders the trimmed doc so the
	// final PDF really matches the chosen design.
	doc, review, err := polishGenerate(ctx, ai, in, tpl, nil)
	if err != nil {
		return nil, err
	}
	fit, doc := PlanContent(doc, tpl)
	doc.GeneratedAt = time.Now()
	doc.SourcePath = in.ResumePath
	doc.ProjectCount = len(in.Projects)

	dir, err := resumesDir()
	if err != nil {
		return nil, err
	}
	stamp := doc.GeneratedAt.Format("20060102-150405")
	base := filepath.Join(dir, "improved-"+stamp)
	out := &ImproveOutput{
		Doc:          doc,
		Review:       review,
		Dir:          dir,
		Files:        map[Format]string{},
		PreviewMD:    RenderMarkdownFor(doc, tpl),
		TemplateID:   tpl.ID,
		TemplateName: tpl.Name,
		Fit:          fit,
	}

	want := formatSet(formats)
	// Always write MD + LaTeX + JSON so converters / Config library have sources.
	mdPath := base + ".md"
	if err := os.WriteFile(mdPath, []byte(RenderMarkdownFor(doc, tpl)), 0600); err != nil {
		return nil, err
	}
	texPath := base + ".tex"
	if err := os.WriteFile(texPath, []byte(RenderLaTeXFor(doc, tpl)), 0600); err != nil {
		return nil, err
	}
	jsonPath := base + ".json"
	meta, _ := json.MarshalIndent(doc, "", "  ")
	_ = os.WriteFile(jsonPath, meta, 0600)

	if want[FormatMarkdown] {
		out.Files[FormatMarkdown] = mdPath
	}
	if want[FormatLaTeX] {
		out.Files[FormatLaTeX] = texPath
	}

	// Always produce a PDF for applying (LaTeX → pandoc → native fallback).
	pdfPath := base + ".pdf"
	conv, err := EnsurePDFFor(doc, tpl, mdPath, texPath, pdfPath)
	if err != nil {
		out.PDFNote = err.Error()
		return out, nil
	}
	out.Files[FormatPDF] = conv.PDFPath
	if conv.Note != "" {
		out.PDFNote = conv.Note
	}
	out.VersionID = stamp

	// Verify the real render's page count using the deterministic native
	// renderer (always available, same manifest geometry). Count on a temp
	// path so an existing LaTeX/pandoc PDF is never overwritten.
	if tmp, terr := os.CreateTemp("", "nexus-fit-check-*.pdf"); terr == nil {
		tmpPath := tmp.Name()
		_ = tmp.Close()
		defer os.Remove(tmpPath)
		if pages, perr := RenderNativePDFForCounted(doc, tpl, tmpPath); perr == nil {
			fit.Pages = pages
			if tpl.OnePage && pages > 1 {
				fit.Warnings = append(fit.Warnings,
					fmt.Sprintf("Rendered %d pages — this template targets one page. Shorten the summary or trim experience.", pages))
			}
			out.Fit = fit
		}
	}

	label := doc.TargetRole
	if label == "" {
		label = doc.Headline
	}
	if label == "" {
		label = "Improved resume"
	}
	_ = RegisterVersion(Version{
		ID:         stamp,
		CreatedAt:  doc.GeneratedAt,
		Label:      label,
		TargetRole: doc.TargetRole,
		Template:   tpl.ID,
		PDFPath:    conv.PDFPath,
		MDPath:     mdPath,
		TeXPath:    texPath,
		JSONPath:   jsonPath,
		Source:     "nexus",
		Notes:      "method=" + conv.Method,
	})
	return out, nil
}

func formatSet(formats []Format) map[Format]bool {
	m := make(map[Format]bool, len(formats))
	for _, f := range formats {
		m[f] = true
	}
	return m
}

func resumesDir() (string, error) {
	dir := filepath.Join(nexusdir.Home(), "resumes")
	return dir, os.MkdirAll(dir, 0700)
}

// FormatProjects renders verified work-context projects as a prompt-ready
// text block. Shared by the resume rewriter and the tailor agents.
func FormatProjects(projects []workcontext.Project) string {
	var b strings.Builder
	for i, p := range projects {
		b.WriteString(fmt.Sprintf("\n### Project %d: %s\n", i+1, p.Name))
		if p.Role != "" {
			b.WriteString("Role: " + p.Role + "\n")
		}
		if p.Period != "" {
			b.WriteString("Period: " + p.Period + "\n")
		}
		if p.Repo != "" {
			b.WriteString("Repo: " + p.Repo + "\n")
		}
		b.WriteString(p.Summary + "\n")
		if len(p.Bullets) > 0 {
			b.WriteString("Bullets:\n")
			for _, bullet := range p.Bullets {
				b.WriteString("- " + bullet + "\n")
			}
		}
	}
	return b.String()
}

// TrimForPrompt caps a prompt section at max characters with an explicit
// truncation marker. Shared by every prompt builder in this module.
func TrimForPrompt(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n…[truncated]"
}
