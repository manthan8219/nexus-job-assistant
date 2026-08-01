package resume

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestTemplatesRegistry(t *testing.T) {
	tmpls := Templates()
	if len(tmpls) != 8 {
		t.Fatalf("registry has %d templates; want 8", len(tmpls))
	}
	seen := map[string]bool{}
	for _, tmpl := range tmpls {
		if tmpl.ID == "" || tmpl.Name == "" {
			t.Errorf("template with empty id/name: %+v", tmpl)
		}
		if seen[tmpl.ID] {
			t.Errorf("duplicate template id %q", tmpl.ID)
		}
		seen[tmpl.ID] = true
		if len(tmpl.Sections) == 0 {
			t.Errorf("template %q has no sections", tmpl.ID)
		}
		if tmpl.AccentHex == "" {
			t.Errorf("template %q has no accent hex", tmpl.ID)
		}
		if tmpl.Source == "" {
			t.Errorf("template %q should credit its real open-source source", tmpl.ID)
		}
	}

	// GetTemplate resolves every registry id; empty defaults to jake; classic
	// stays as a legacy alias for stored versions/links.
	for _, tmpl := range tmpls {
		got, err := GetTemplate(tmpl.ID)
		if err != nil || got.ID != tmpl.ID {
			t.Errorf("GetTemplate(%q) = %q, %v", tmpl.ID, got.ID, err)
		}
	}
	def, err := GetTemplate("")
	if err != nil || def.ID != TemplateJake {
		t.Errorf("GetTemplate(\"\") should default to jake, got %q (%v)", def.ID, err)
	}
	alias, err := GetTemplate(TemplateClassic)
	if err != nil || alias.ID != TemplateJake {
		t.Errorf("GetTemplate(\"classic\") should alias to jake, got %q (%v)", alias.ID, err)
	}
	if _, err := GetTemplate("nope"); err == nil {
		t.Error("GetTemplate(\"nope\") should error")
	}

	// Real-design tokens are set where the designs demand them.
	jake, _ := GetTemplate(TemplateJake)
	if jake.SectionStyle != SectionStyleCaps {
		t.Errorf("jake sectionStyle = %q; want caps", jake.SectionStyle)
	}
	acv, _ := GetTemplate(TemplateAwesomeCV)
	if acv.SectionStyle != SectionStyleMarker {
		t.Errorf("awesome-cv sectionStyle = %q; want marker", acv.SectionStyle)
	}
	deedy, _ := GetTemplate(TemplateDeedy)
	if !deedy.OnePage || deedy.ColumnRatio < 0.7 || deedy.Layout != LayoutSidebar {
		t.Errorf("deedy should be a one-page asymmetric sidebar: %+v", deedy)
	}
	mcd, _ := GetTemplate(TemplateMcDowell)
	if mcd.SectionStyle != SectionStyleSoft {
		t.Errorf("mcdowell sectionStyle = %q; want soft", mcd.SectionStyle)
	}
	br, _ := GetTemplate(TemplateBillRyan)
	if br.BodyFont != "serif" {
		t.Errorf("billryan bodyFont = %q; want serif", br.BodyFont)
	}
	ken, _ := GetTemplate(TemplateKendall)
	if ken.RailBackground != "dark" || ken.Layout != LayoutSidebar {
		t.Errorf("kendall should be a dark-sidebar layout: %+v", ken)
	}
	mac, _ := GetTemplate(TemplateMacchiato)
	if mac.RailBackground != "accent" || mac.NameStyle != NameStyleColored {
		t.Errorf("macchiato should have an accent rail + colored name: %+v", mac)
	}
	bank, _ := GetTemplate(TemplateBanking)
	if !bank.ContactLine || bank.SectionStyle != SectionStyleRuleAbove || bank.NameStyle != NameStyleCentered {
		t.Errorf("banking should have a contact line + ruled centered header: %+v", bank)
	}

	// Every template declares a positive space budget (the AI + planner use it).
	for _, tmpl := range Templates() {
		b := tmpl.Budget
		if b.MaxRoles == 0 || b.MaxBulletsPerRole == 0 || b.MaxSkills == 0 ||
			b.MaxEducation == 0 || b.CharsPerLine == 0 || b.MaxSummaryLines == 0 {
			t.Errorf("template %q missing budget fields: %+v", tmpl.ID, b)
		}
	}
}

func TestTemplateManifestSerializesShowRule(t *testing.T) {
	// `false` must round-trip through JSON (no omitempty) — the UI decides
	// whether to draw the header rule from this token.
	bank, _ := GetTemplate(TemplateBanking)
	data, err := json.Marshal(bank)
	if err != nil {
		t.Fatalf("marshal banking: %v", err)
	}
	if !strings.Contains(string(data), `"showRule":false`) {
		t.Errorf("banking manifest should carry explicit showRule:false, got %s", data)
	}
	bill, _ := GetTemplate(TemplateBillRyan)
	dataB, _ := json.Marshal(bill)
	if !strings.Contains(string(dataB), `"showRule":true`) {
		t.Errorf("billryan manifest should carry explicit showRule:true, got %s", dataB)
	}
}

func TestRenderLaTeXForTemplates(t *testing.T) {
	doc := ImprovedDoc{
		FullName: "Ada Lovelace",
		Headline: "Systems Engineer",
		Summary:  "Builds reliable platforms.",
		Skills:   []string{"Go", "SQL"},
		Experience: []ImprovedRole{{
			Title: "Engineer", Org: "Analytical Engine", Period: "1843",
			Bullets: []string{"Designed algorithms"},
		}},
		Education: []string{"Self-taught mathematics"},
	}

	jake, _ := GetTemplate(TemplateJake)
	texJ := RenderLaTeXFor(doc, jake)
	if !containsAll(texJ, `\newcommand{\rsec}[1]{\par\vspace{0.55em}`, `\noindent{\LARGE\textbf{Ada Lovelace}}`) {
		t.Fatalf("jake latex missing caps sections / left header:\n%s", texJ)
	}

	awesome, _ := GetTemplate(TemplateAwesomeCV)
	texA := RenderLaTeXFor(doc, awesome)
	if !containsAll(texA, `\definecolor{accent}{HTML}{00539B}`, `\raisebox{1.5pt}{\rule{2.6mm}{2.6mm}}`) {
		t.Fatalf("awesome-cv latex missing accent marker style:\n%s", texA)
	}

	deedy, _ := GetTemplate(TemplateDeedy)
	texD := RenderLaTeXFor(doc, deedy)
	if !containsAll(texD, `\documentclass[10pt,a4paper]{article}`, `margin=0.5in`) {
		t.Fatalf("deedy latex should be 10pt + 0.5in:\n%s", texD)
	}
	if !strings.Contains(texD, `0.76\textwidth`) || !strings.Contains(texD, `0.22\textwidth`) {
		t.Fatalf("deedy latex should use asymmetric 0.76/0.22 columns:\n%s", texD)
	}

	kendall, _ := GetTemplate(TemplateKendall)
	texK := RenderLaTeXFor(doc, kendall)
	if !containsAll(texK, `\definecolor{railbg}{HTML}{111827}`, `\colorbox{railbg}`) {
		t.Fatalf("kendall latex should wrap the rail in a dark colorbox:\n%s", texK)
	}

	macchiato, _ := GetTemplate(TemplateMacchiato)
	texM := RenderLaTeXFor(doc, macchiato)
	if !containsAll(texM, `\colorbox{railbg}`, `\color{accent}\textbf{Ada Lovelace}`) {
		t.Fatalf("macchiato latex should accent the name + color the rail:\n%s", texM)
	}

	banking, _ := GetTemplate(TemplateBanking)
	texB := RenderLaTeXFor(doc, banking)
	if !contains(texB, `{\color{accent}\rule{\textwidth}{0.5pt}}`) {
		t.Fatalf("banking latex should have ruled sections:\n%s", texB)
	}
	// The contact line only renders when the doc carries contact details.
	if strings.Contains(texB, `\textcolor{gray}`) {
		t.Fatalf("banking latex should omit the contact line without contact data:\n%s", texB)
	}
	doc.Email = "ada@example.com"
	texB2 := RenderLaTeXFor(doc, banking)
	if !containsAll(texB2, `\textcolor{gray}`, `ada@example.com`) {
		t.Fatalf("banking latex should render the email contact line:\n%s", texB2)
	}
}

func TestRenderNativePDFForAllTemplates(t *testing.T) {
	doc := ImprovedDoc{
		FullName: "Ada Lovelace",
		Headline: "Systems Engineer",
		Summary:  "Builds reliable platforms.",
		Skills:   []string{"Go", "SQL", "Kubernetes"},
		Experience: []ImprovedRole{{
			Title: "Engineer", Org: "Analytical Engine", Period: "1843",
			Bullets: []string{"Designed algorithms", "Documented the machine"},
		}},
		Education: []string{"Self-taught mathematics"},
	}
	for _, tmpl := range Templates() {
		path := t.TempDir() + "/" + tmpl.ID + ".pdf"
		if err := RenderNativePDFFor(doc, tmpl, path); err != nil {
			t.Errorf("RenderNativePDFFor(%s): %v", tmpl.ID, err)
			continue
		}
		st, err := os.Stat(path)
		if err != nil {
			t.Errorf("RenderNativePDFFor(%s): stat: %v", tmpl.ID, err)
			continue
		}
		if st.Size() == 0 {
			t.Errorf("RenderNativePDFFor(%s): empty pdf", tmpl.ID)
		}
	}
}
