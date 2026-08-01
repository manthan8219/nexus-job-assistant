package resume

import (
	"path/filepath"
	"strings"
	"testing"
)

// bigDoc is deliberately over every template budget: 8 roles × 8 bullets,
// 25 skills, 5 education entries and a multi-line summary.
func bigDoc() ImprovedDoc {
	bullets := make([]string, 8)
	for i := range bullets {
		bullets[i] = strings.Repeat("Delivered ", 12) + "impact"
	}
	roles := make([]ImprovedRole, 8)
	for i := range roles {
		roles[i] = ImprovedRole{Title: "Senior Engineer", Org: "Acme", Period: "2020—2024", Bullets: bullets}
	}
	skills := make([]string, 25)
	for i := range skills {
		skills[i] = "Skill"
	}
	edu := make([]string, 5)
	for i := range edu {
		edu[i] = "Degree, University, Year"
	}
	return ImprovedDoc{
		FullName:   "Ada Lovelace",
		Headline:   "Senior Engineer",
		Summary:    strings.Repeat("A great summary sentence that quantifies impact and fits the target role. ", 8),
		Skills:     skills,
		Experience: roles,
		Education:  edu,
	}
}

func TestBudgetsForAllTemplates(t *testing.T) {
	for _, tmpl := range Templates() {
		b := tmpl.Budget
		if b.MaxRoles <= 0 || b.MaxBulletsPerRole <= 0 || b.MaxSkills <= 0 ||
			b.MaxEducation <= 0 || b.CharsPerLine <= 0 || b.MaxSummaryLines <= 0 {
			t.Errorf("%s: budget fields must be positive: %+v", tmpl.ID, b)
		}
		// Sidebar layouts have a narrower main column → fewer chars per line.
		if (tmpl.ID == TemplateSidebar || tmpl.ID == TemplateSplit) && b.CharsPerLine >= 80 {
			t.Errorf("%s: sidebar charsPerLine=%d; want < 80", tmpl.ID, b.CharsPerLine)
		}
	}
	compact, _ := GetTemplate(TemplateCompact)
	if compact.Budget.TargetPages != 1 {
		t.Errorf("compact targetPages = %d; want 1", compact.Budget.TargetPages)
	}
	classic, _ := GetTemplate(TemplateClassic)
	if classic.Budget.CharsPerLine != 95 {
		t.Errorf("classic charsPerLine = %d; want 95", classic.Budget.CharsPerLine)
	}
}

func TestPlanContentSectionOrdering(t *testing.T) {
	for _, tmpl := range Templates() {
		plan, _ := PlanContent(SampleResume(), tmpl)
		var got, want []SectionKey
		for _, s := range plan.Sections {
			got = append(got, s.Key)
		}
		for _, s := range tmpl.Sections {
			want = append(want, s.Key)
		}
		if len(got) != len(want) {
			t.Fatalf("%s: plan sections %v; want %v", tmpl.ID, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: plan order %v; want %v", tmpl.ID, got, want)
				break
			}
		}
	}
}

func TestPlanContentTrimsToBudget(t *testing.T) {
	tpl, _ := GetTemplate(TemplateClassic) // 5 roles · 4 bullets · 12 skills · 2 edu
	plan, fitted := PlanContent(bigDoc(), tpl)
	if len(fitted.Experience) != 5 {
		t.Errorf("roles = %d; want 5", len(fitted.Experience))
	}
	for _, r := range fitted.Experience {
		if len(r.Bullets) != 4 {
			t.Fatalf("bullets = %d; want 4", len(r.Bullets))
		}
	}
	if len(fitted.Skills) != 12 {
		t.Errorf("skills = %d; want 12", len(fitted.Skills))
	}
	if len(fitted.Education) != 2 {
		t.Errorf("education = %d; want 2", len(fitted.Education))
	}
	if plan.FitScore >= 100 {
		t.Errorf("fitScore = %d; want < 100 for an over-budget doc", plan.FitScore)
	}
	if len(plan.TrimmedSections) == 0 {
		t.Error("expected trimmed sections to be reported")
	}
	if len(plan.Warnings) == 0 {
		t.Error("expected at least one warning")
	}
	if plan.PlannedLines <= 0 || plan.EstimatedPages < 1 {
		t.Errorf("bad plan numbers: lines=%d pages=%.1f", plan.PlannedLines, plan.EstimatedPages)
	}
}

func TestPlanContentTruncatesOverlongSummary(t *testing.T) {
	tpl, _ := GetTemplate(TemplateClassic)
	doc := SampleResume()
	doc.Summary = strings.Repeat("Quantified impact sentence. ", 30) // ~870 chars > 285
	_, fitted := PlanContent(doc, tpl)
	if len(fitted.Summary) >= len(doc.Summary) {
		t.Error("summary was not shortened")
	}
	if !strings.Contains(fitted.Summary, "…") {
		t.Error("truncated summary should end with an ellipsis marker")
	}
}

func TestPlanContentFitsSmallDoc(t *testing.T) {
	tpl, _ := GetTemplate(TemplateClassic)
	plan, fitted := PlanContent(SampleResume(), tpl)
	if plan.FitScore != 100 {
		t.Errorf("fitScore = %d; want 100 for the sample resume", plan.FitScore)
	}
	if plan.EstimatedPages != 1 {
		t.Errorf("estimatedPages = %.1f; want 1", plan.EstimatedPages)
	}
	if len(plan.TrimmedSections) != 0 {
		t.Errorf("sample should need no trimming, got %v", plan.TrimmedSections)
	}
	if len(fitted.Experience) != len(SampleResume().Experience) {
		t.Error("sample resume content should be unchanged")
	}
}

func TestPlanContentReportsTargetPagesWarning(t *testing.T) {
	tpl, _ := GetTemplate(TemplateCompact) // targetPages = 1
	plan, _ := PlanContent(bigDoc(), tpl)
	found := false
	for _, w := range plan.Warnings {
		if strings.Contains(w, "ONE page") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("compact plan should warn about the one-page target, got %v", plan.Warnings)
	}
}

func TestRenderNativePDFForCountedReportsPages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "giant.pdf")
	tpl, _ := GetTemplate(TemplateClassic)
	pages, err := RenderNativePDFForCounted(bigDoc(), tpl, path)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if pages < 2 {
		t.Fatalf("giant doc rendered %d pages; want >= 2", pages)
	}
}

func TestSampleResumeFitsOnePageCompact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compact.pdf")
	tpl, _ := GetTemplate(TemplateCompact)
	pages, err := RenderNativePDFForCounted(SampleResume(), tpl, path)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if pages != 1 {
		t.Fatalf("sample resume in compact rendered %d pages; want 1", pages)
	}
}
