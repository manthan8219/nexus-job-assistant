package resume

import (
	"fmt"
	"strings"
)

// plan.go — the deterministic content→template planner. It answers "does this
// content fit this template?" by slotting the document into the template's
// declared sections, estimating line usage from the budget's chars-per-line,
// trimming anything over the caps, and scoring how comfortably the result fits
// the template's target page count. The AI creator writes to the same budget
// (see polishTemplateBlock) and the renderer verifies the real page count
// afterwards, so the plan, the writing, and the rendered PDF stay honest.

// PlannedSection is one content slot in the plan with its estimated line cost.
type PlannedSection struct {
	Key   SectionKey `json:"key"`
	Label string     `json:"label"`
	Lines int        `json:"lines"`
}

// FitPlan is the plan for fitting a document into a template.
type FitPlan struct {
	TemplateID      string           `json:"templateId"`
	Layout          TemplateLayout   `json:"layout"`
	Budget          SpaceBudget      `json:"budget"`
	PlannedLines    int              `json:"plannedLines"`
	TargetLines     int              `json:"targetLines"`
	EstimatedPages  float64          `json:"estimatedPages"`
	Pages           int              `json:"pages,omitempty"` // real native-render page count (filled post-render)
	FitScore        int              `json:"fitScore"`        // 0-100
	Warnings        []string         `json:"warnings"`
	TrimmedSections []string         `json:"trimmedSections"`
	Sections        []PlannedSection `json:"sections"`
}

const (
	sectionOverheadLines = 3 // section title + rule + spacing
	headerBlockLines     = 6 // name + headline + rule
)

// targetContentLines approximates how many body lines the template's usable
// page area holds (A4 height minus margins and the header block, divided by
// the native line leading).
func targetContentLines(tpl Template) int {
	lead := tpl.Design.NativeLead
	if lead <= 0 {
		lead = 5
	}
	body := tpl.Design.NativeSize
	lines := int((297.0 - 28.0 - 22.0) / lead) // A4 - top/bottom margins - header block
	if lines < 30 {
		lines = 30
	}
	if body >= 11 {
		lines -= 4 // larger type eats more space
	}
	return lines
}

// estimateLines wraps s across the budget's chars-per-line, always >= 1 for
// non-empty input and counting explicit newlines as their own line.
func estimateLines(s string, charsPerLine int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if charsPerLine <= 0 {
		charsPerLine = 95
	}
	lines := 0
	for _, part := range strings.Split(s, "\n") {
		n := len(part)
		if n == 0 {
			lines++
			continue
		}
		lines += (n + charsPerLine - 1) / charsPerLine
	}
	return lines
}

// PlanContent fits doc into the template: it slots content into the template's
// declared sections in order, estimates line usage from the budget, trims
// anything over the caps (roles, bullets, skills, education, summary), and
// scores how comfortably the result fits the target page count. The trimmed
// doc is returned alongside the plan so the pipeline renders content that
// actually fits the chosen design.
func PlanContent(doc ImprovedDoc, tpl Template) (FitPlan, ImprovedDoc) {
	budget := budgetFor(tpl)
	plan := FitPlan{
		TemplateID:  tpl.ID,
		Layout:      tpl.Layout,
		Budget:      budget,
		TargetLines: targetContentLines(tpl),
	}
	trimmed := make([]string, 0, 4)
	mark := func(what string) {
		if len(trimmed) == 0 || trimmed[len(trimmed)-1] != what {
			trimmed = append(trimmed, what)
		}
	}

	// Deterministic caps — top-most content wins (the rewriter already orders
	// by relevance, so "keep the first N" keeps the strongest evidence).
	if len(doc.Experience) > budget.MaxRoles {
		doc.Experience = doc.Experience[:budget.MaxRoles]
		mark(fmt.Sprintf("experience kept to top %d roles", budget.MaxRoles))
	}
	for i := range doc.Experience {
		if len(doc.Experience[i].Bullets) > budget.MaxBulletsPerRole {
			doc.Experience[i].Bullets = doc.Experience[i].Bullets[:budget.MaxBulletsPerRole]
			mark(fmt.Sprintf("each role capped at %d bullets", budget.MaxBulletsPerRole))
		}
	}
	if len(doc.Skills) > budget.MaxSkills {
		doc.Skills = doc.Skills[:budget.MaxSkills]
		mark(fmt.Sprintf("skills kept to top %d", budget.MaxSkills))
	}
	if len(doc.Education) > budget.MaxEducation {
		doc.Education = doc.Education[:budget.MaxEducation]
		mark(fmt.Sprintf("education kept to top %d entries", budget.MaxEducation))
	}
	if summaryMax := budget.MaxSummaryLines * budget.CharsPerLine; summaryMax > 0 && len(doc.Summary) > summaryMax {
		doc.Summary = truncateWords(doc.Summary, summaryMax)
		mark("summary shortened to fit the template")
	}
	plan.TrimmedSections = trimmed

	// Estimate per-section lines in the template's declared order.
	rail := tpl.Layout == LayoutSidebar
	total := headerBlockLines
	for _, sec := range tpl.Sections {
		ps := PlannedSection{Key: sec.Key, Label: sec.Label}
		switch sec.Key {
		case SectionSummary:
			if doc.Summary != "" {
				ps.Lines = estimateLines(doc.Summary, budget.CharsPerLine)
				if ps.Lines > budget.MaxSummaryLines {
					ps.Lines = budget.MaxSummaryLines
				}
			}
		case SectionSkills:
			if len(doc.Skills) > 0 {
				if rail {
					ps.Lines = 0
					for _, s := range doc.Skills {
						ps.Lines += estimateLines(s, budget.CharsPerLine)
					}
				} else {
					ps.Lines = estimateLines(strings.Join(doc.Skills, " · "), budget.CharsPerLine)
				}
			}
		case SectionExperience:
			if len(doc.Experience) > 0 {
				ps.Lines = 0
				for _, role := range doc.Experience {
					ps.Lines++ // role header (+ period)
					for _, b := range role.Bullets {
						ps.Lines += estimateLines(b, budget.CharsPerLine)
					}
					ps.Lines++ // spacing between roles
				}
			}
		case SectionEducation:
			if len(doc.Education) > 0 {
				if rail {
					ps.Lines = len(doc.Education)
				} else {
					ps.Lines = 0
					for _, line := range doc.Education {
						ps.Lines += estimateLines(line, budget.CharsPerLine)
					}
				}
			}
		}
		if ps.Lines > 0 {
			ps.Lines += sectionOverheadLines
			total += ps.Lines
			plan.Sections = append(plan.Sections, ps)
		}
	}

	plan.PlannedLines = total
	if plan.TargetLines <= 0 {
		plan.TargetLines = 55
	}
	est := float64(total) / float64(plan.TargetLines)
	if est < 1 {
		est = 1
	}
	plan.EstimatedPages = est
	if total <= plan.TargetLines {
		plan.FitScore = 100
	} else {
		plan.FitScore = int(100 * float64(plan.TargetLines) / float64(total))
	}

	if budget.TargetPages == 1 && est > 1 {
		plan.Warnings = append(plan.Warnings,
			fmt.Sprintf("This template targets ONE page — the content estimates ~%.1f pages. Use the Compact budget, fewer roles, or shorter bullets.", est))
	} else if est > 2 {
		plan.Warnings = append(plan.Warnings,
			fmt.Sprintf("Estimated ~%.1f pages — consider a tighter template or trimming experience.", est))
	}
	if len(plan.TrimmedSections) > 0 {
		plan.Warnings = append(plan.Warnings,
			"Content was trimmed to fit the template: "+strings.Join(plan.TrimmedSections, "; ")+".")
	}
	return plan, doc
}

// truncateWords cuts s to at most max chars on a word boundary, with an
// ellipsis marker so the user can see the summary was shortened.
func truncateWords(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "…"
}
