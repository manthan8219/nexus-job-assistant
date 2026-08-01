package resume

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestTemplatesRegistry(t *testing.T) {
	tmpls := Templates()
	if len(tmpls) != 12 {
		t.Fatalf("registry has %d templates; want 12", len(tmpls))
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
	}

	// GetTemplate resolves every registry id and falls back for empty.
	for _, tmpl := range tmpls {
		got, err := GetTemplate(tmpl.ID)
		if err != nil || got.ID != tmpl.ID {
			t.Errorf("GetTemplate(%q) = %q, %v", tmpl.ID, got.ID, err)
		}
	}
	def, err := GetTemplate("")
	if err != nil || def.ID != TemplateClassic {
		t.Errorf("GetTemplate(\"\") should default to classic, got %q (%v)", def.ID, err)
	}
	if _, err := GetTemplate("nope"); err == nil {
		t.Error("GetTemplate(\"nope\") should error")
	}

	// Font + rail tokens are set on the templates that need them.
	exec, _ := GetTemplate(TemplateExecutive)
	if exec.BodyFont != "serif" {
		t.Errorf("executive bodyFont = %q; want serif", exec.BodyFont)
	}
	dev, _ := GetTemplate(TemplateDeveloper)
	if dev.BodyFont != "mono" {
		t.Errorf("developer bodyFont = %q; want mono", dev.BodyFont)
	}
	split, _ := GetTemplate(TemplateSplit)
	if split.RailSide != "right" {
		t.Errorf("split railSide = %q; want right", split.RailSide)
	}

	// Header alignment + rule tokens are promoted onto the manifest so the web
	// gallery's miniature previews stay faithful to the real renderers.
	classic, _ := GetTemplate(TemplateClassic)
	if classic.HeaderAlign != "left" || !classic.ShowRule {
		t.Errorf("classic preview tokens = %q/%v; want left/true", classic.HeaderAlign, classic.ShowRule)
	}
	modern, _ := GetTemplate(TemplateModern)
	if modern.HeaderAlign != "center" || !modern.ShowRule {
		t.Errorf("modern preview tokens = %q/%v; want center/true", modern.HeaderAlign, modern.ShowRule)
	}
	exec, _ = GetTemplate(TemplateExecutive)
	if exec.HeaderAlign != "center" || exec.ShowRule {
		t.Errorf("executive preview tokens = %q/%v; want center/false", exec.HeaderAlign, exec.ShowRule)
	}
	mono, _ := GetTemplate(TemplateMonochrome)
	if mono.ShowRule {
		t.Error("monochrome showRule = true; want false (no rule)")
	}
}

func TestTemplateManifestSerializesShowRule(t *testing.T) {
	// `false` must round-trip through JSON (no omitempty) — the UI decides
	// whether to draw the header rule from this token.
	exec, _ := GetTemplate(TemplateExecutive)
	data, err := json.Marshal(exec)
	if err != nil {
		t.Fatalf("marshal executive: %v", err)
	}
	if !strings.Contains(string(data), `"showRule":false`) {
		t.Errorf("executive manifest should carry explicit showRule:false, got %s", data)
	}
	classic, _ := GetTemplate(TemplateClassic)
	dataC, err := json.Marshal(classic)
	if err != nil {
		t.Fatalf("marshal classic: %v", err)
	}
	if !strings.Contains(string(dataC), `"showRule":true`) {
		t.Errorf("classic manifest should carry explicit showRule:true, got %s", dataC)
	}
}

func TestRenderMarkdownForSectionOrder(t *testing.T) {
	doc := ImprovedDoc{
		FullName: "Ada Lovelace",
		Summary:  "Builds reliable platforms.",
		Skills:   []string{"Go", "SQL"},
		Experience: []ImprovedRole{{
			Title: "Engineer", Org: "Analytical Engine", Period: "1843",
			Bullets: []string{"Designed algorithms"},
		}},
		Education: []string{"Self-taught mathematics"},
	}

	sidebar, _ := GetTemplate(TemplateSidebar)
	md := RenderMarkdownFor(doc, sidebar)
	if !strings.Contains(md, "## Skills") || !strings.Contains(md, "## Summary") {
		t.Fatalf("sidebar markdown missing sections:\n%s", md)
	}
	if strings.Index(md, "## Skills") > strings.Index(md, "## Summary") {
		t.Fatalf("sidebar markdown should lead with Skills (the rail):\n%s", md)
	}

	compact, _ := GetTemplate(TemplateCompact)
	mdC := RenderMarkdownFor(doc, compact)
	if !strings.Contains(mdC, "## Experience") || !strings.Contains(mdC, "## Skills") {
		t.Fatalf("compact markdown missing sections:\n%s", mdC)
	}
	if strings.Index(mdC, "## Experience") > strings.Index(mdC, "## Skills") {
		t.Fatalf("compact markdown should list Experience before Skills:\n%s", mdC)
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

	modern, _ := GetTemplate(TemplateModern)
	tex := RenderLaTeXFor(doc, modern)
	if !containsAll(tex, `\definecolor{accent}{HTML}{8B5CF6}`, `\begin{center}`, `\rsec{Summary}`) {
		t.Fatalf("modern latex missing design markers:\n%s", tex)
	}
	if !contains(tex, `{\color{accent}\textbf{Systems Engineer}}`) {
		t.Fatalf("modern latex should accent the headline:\n%s", tex)
	}

	sidebar, _ := GetTemplate(TemplateSidebar)
	texS := RenderLaTeXFor(doc, sidebar)
	if !containsAll(texS, `\begin{minipage}[t]{0.30\textwidth}`, `\begin{minipage}[t]{0.66\textwidth}`) {
		t.Fatalf("sidebar latex should render two minipage columns:\n%s", texS)
	}

	compact, _ := GetTemplate(TemplateCompact)
	texC := RenderLaTeXFor(doc, compact)
	if !containsAll(texC, `\documentclass[10pt,a4paper]{article}`, `margin=0.5in`) {
		t.Fatalf("compact latex should use 10pt + 0.5in margin:\n%s", texC)
	}

	classic, _ := GetTemplate(TemplateClassic)
	texK := RenderLaTeXFor(doc, classic)
	if !containsAll(texK, `\documentclass[11pt,a4paper]{article}`, `margin=0.75in`) {
		t.Fatalf("classic latex should use 11pt + 0.75in margin:\n%s", texK)
	}

	developer, _ := GetTemplate(TemplateDeveloper)
	texD := RenderLaTeXFor(doc, developer)
	if !contains(texD, `\renewcommand{\familydefault}{\ttdefault}`) {
		t.Fatalf("developer latex should set a monospace family:\n%s", texD)
	}

	executive, _ := GetTemplate(TemplateExecutive)
	texE := RenderLaTeXFor(doc, executive)
	if strings.Contains(texE, `\usepackage{helvet}`) {
		t.Fatalf("executive latex should keep the serif default (no helvet):\n%s", texE)
	}

	split, _ := GetTemplate(TemplateSplit)
	texSp := RenderLaTeXFor(doc, split)
	if !containsAll(texSp, `\begin{minipage}[t]{0.66\textwidth}`, `\begin{minipage}[t]{0.30\textwidth}`) {
		t.Fatalf("split latex should render two minipage columns:\n%s", texSp)
	}
	// Split puts experience (the wide column) BEFORE the skills rail.
	mainIdx := strings.Index(texSp, `\begin{minipage}[t]{0.66\textwidth}`)
	railIdx := strings.Index(texSp, `\begin{minipage}[t]{0.30\textwidth}`)
	if mainIdx == -1 || railIdx == -1 || mainIdx > railIdx {
		t.Fatalf("split latex should lead with the main column (right rail):\n%s", texSp)
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

func TestRenderMarkdownForClassicBackCompat(t *testing.T) {
	doc := ImprovedDoc{
		FullName: "Ada Lovelace",
		Summary:  "Builds reliable platforms.",
		Skills:   []string{"Go", "SQL"},
		Experience: []ImprovedRole{{
			Title: "Engineer", Org: "Analytical Engine",
			Bullets: []string{"Designed algorithms"},
		}},
	}
	// The Classic wrapper output must still contain the standard sections.
	md := RenderMarkdown(doc)
	if !containsAll(md, "Ada Lovelace", "Go · SQL", "Analytical Engine") {
		t.Fatalf("classic markdown missing pieces:\n%s", md)
	}
}
