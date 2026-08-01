package resume

import (
	"fmt"
	"strings"
)

// RenderMarkdown turns an ImprovedDoc into a clean Markdown resume using the
// default Classic template (kept for TUI/tailor callers and tests).
func RenderMarkdown(doc ImprovedDoc) string {
	tpl, _ := GetTemplate(TemplateClassic)
	return RenderMarkdownFor(doc, tpl)
}

// RenderMarkdownFor renders the doc with the section order the template
// declares. The .md file is an editable source; real design differences (two
// columns, colors, spacing) appear in the LaTeX/PDF outputs.
func RenderMarkdownFor(doc ImprovedDoc, tpl Template) string {
	var b strings.Builder
	if doc.FullName != "" {
		fmt.Fprintf(&b, "# %s\n\n", doc.FullName)
	} else {
		b.WriteString("# Resume\n\n")
	}
	if doc.Headline != "" {
		fmt.Fprintf(&b, "**%s**\n\n", doc.Headline)
	}
	for _, sec := range tpl.Sections {
		switch sec.Key {
		case SectionSummary:
			if doc.Summary != "" {
				fmt.Fprintf(&b, "## %s\n\n%s\n\n", sec.Label, doc.Summary)
			}
		case SectionSkills:
			if len(doc.Skills) > 0 {
				fmt.Fprintf(&b, "## %s\n\n%s\n\n", sec.Label, strings.Join(doc.Skills, " · "))
			}
		case SectionExperience:
			if len(doc.Experience) > 0 {
				fmt.Fprintf(&b, "## %s\n\n", sec.Label)
				for _, role := range doc.Experience {
					head := role.Title
					if role.Org != "" {
						if head != "" {
							head += " — " + role.Org
						} else {
							head = role.Org
						}
					}
					fmt.Fprintf(&b, "### %s\n", head)
					if role.Period != "" {
						fmt.Fprintf(&b, "*%s*\n\n", role.Period)
					} else {
						b.WriteString("\n")
					}
					for _, bullet := range role.Bullets {
						fmt.Fprintf(&b, "- %s\n", bullet)
					}
					b.WriteString("\n")
				}
			}
		case SectionEducation:
			if len(doc.Education) > 0 {
				fmt.Fprintf(&b, "## %s\n\n", sec.Label)
				for _, line := range doc.Education {
					fmt.Fprintf(&b, "- %s\n", line)
				}
				b.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(b.String()) + "\n"
}

// RenderLaTeX turns an ImprovedDoc into a LaTeX resume using the default
// Classic template (kept for TUI/tailor callers and tests).
func RenderLaTeX(doc ImprovedDoc) string {
	tpl, _ := GetTemplate(TemplateClassic)
	return RenderLaTeXFor(doc, tpl)
}

// RenderLaTeXFor renders the doc into a LaTeX article whose design follows the
// template manifest: margins, font size, accent color, header alignment, and a
// two-column rail for the Sidebar template.
func RenderLaTeXFor(doc ImprovedDoc, tpl Template) string {
	esc := latexEscape
	design := tpl.Design
	size := design.LaTeXSize
	if size <= 0 {
		size = 11
	}
	margin := design.LaTeXMargin
	if margin == "" {
		margin = "0.75in"
	}
	itemSep, topSep := "0.15em", "0.2em"
	if tpl.OnePage {
		itemSep, topSep = "0.08em", "0.1em"
	}

	var b strings.Builder
	fmt.Fprintf(&b, `\documentclass[%dpt,a4paper]{article}
\usepackage[margin=%s]{geometry}
\usepackage[T1]{fontenc}
\usepackage{enumitem}
\usepackage{hyperref}
\usepackage{xcolor}
\definecolor{accent}{HTML}{%s}
\pagestyle{empty}
\setlist[itemize]{leftmargin=*,itemsep=%s,topsep=%s}
\newcommand{\rsec}[1]{\par\vspace{0.6em}\noindent{\color{accent}\textbf{\MakeUppercase{#1}}}\vspace{0.25em}\par}
`, size, margin, latexAccentHex(tpl.AccentHex), itemSep, topSep)

	// Body font family per the template manifest (must stay in the preamble).
	switch tpl.BodyFont {
	case "mono":
		b.WriteString(`\renewcommand{\familydefault}{\ttdefault}
`)
	case "serif":
		// article's default (Computer Modern) is already serif.
	default:
		b.WriteString(`\usepackage{helvet}
\renewcommand{\familydefault}{\sfdefault}
`)
	}
	b.WriteString(`\begin{document}
`)

	name := doc.FullName
	if name == "" {
		name = "Resume"
	}
	b.WriteString(`\begin{center}
{\LARGE\textbf{` + esc(name) + `}}\\[0.35em]
`)
	if doc.Headline != "" {
		if tpl.ID == TemplateModern || tpl.ID == TemplateSidebar {
			b.WriteString(`{\color{accent}\textbf{` + esc(doc.Headline) + `}}\\[0.4em]
`)
		} else {
			b.WriteString(esc(doc.Headline) + `\\` + "\n")
		}
	}
	b.WriteString(`\end{center}
`)
	if design.ShowRule {
		b.WriteString(`{\color{accent}\rule{\textwidth}{0.6pt}}

`)
	}

	if tpl.Layout == LayoutSidebar {
		renderSidebarLaTeX(&b, doc, esc, tpl)
	} else {
		renderColumnSections(&b, doc, esc, tpl)
	}
	b.WriteString(`\end{document}
`)
	return b.String()
}

// renderColumnSections writes the single-column section flow for a template.
func renderColumnSections(b *strings.Builder, doc ImprovedDoc, esc func(string) string, tpl Template) {
	sections := tpl.Sections
	if len(sections) == 0 {
		sections = []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
		}
	}
	for _, sec := range sections {
		switch sec.Key {
		case SectionSummary:
			if doc.Summary != "" {
				fmt.Fprintf(b, `\rsec{%s}
%s

`, sec.Label, esc(doc.Summary))
			}
		case SectionSkills:
			if len(doc.Skills) > 0 {
				fmt.Fprintf(b, `\rsec{%s}
%s

`, sec.Label, esc(strings.Join(doc.Skills, " · ")))
			}
		case SectionExperience:
			if len(doc.Experience) > 0 {
				fmt.Fprintf(b, `\rsec{%s}
`, sec.Label)
				renderRoles(b, doc.Experience, esc)
			}
		case SectionEducation:
			if len(doc.Education) > 0 {
				fmt.Fprintf(b, `\rsec{%s}
\begin{itemize}
`, sec.Label)
				for _, line := range doc.Education {
					fmt.Fprintf(b, "  \\item %s\n", esc(line))
				}
				b.WriteString(`\end{itemize}
`)
			}
		}
	}
}

// renderSidebarLaTeX renders the Sidebar/Split templates: summary full-width
// on top, then a two-column body. The rail (skills + education) sits on the
// side the template declares (left by default, right for Split).
func renderSidebarLaTeX(b *strings.Builder, doc ImprovedDoc, esc func(string) string, tpl Template) {
	if doc.Summary != "" {
		fmt.Fprintf(b, `\noindent{\color{accent}\textbf{\MakeUppercase{Summary}}}\\[0.25em]
%s

`, esc(doc.Summary))
	}
	rail := renderSidebarRailLaTeX(doc, esc)
	main := renderSidebarMainLaTeX(doc, esc)
	if tpl.RailSide == "right" {
		b.WriteString(main)
		b.WriteString("\\hfill\n")
		b.WriteString(rail)
		return
	}
	b.WriteString(rail)
	b.WriteString("\\hfill\n")
	b.WriteString(main)
}

// renderSidebarRailLaTeX builds the narrow rail (skills + education).
func renderSidebarRailLaTeX(doc ImprovedDoc, esc func(string) string) string {
	var b strings.Builder
	b.WriteString(`\begin{minipage}[t]{0.30\textwidth}
`)
	if len(doc.Skills) > 0 {
		b.WriteString(`\rsec{Skills}
\begin{itemize}
`)
		for _, s := range doc.Skills {
			fmt.Fprintf(&b, "  \\item %s\n", esc(s))
		}
		b.WriteString(`\end{itemize}
`)
	}
	if len(doc.Education) > 0 {
		b.WriteString(`\rsec{Education}
\begin{itemize}
`)
		for _, line := range doc.Education {
			fmt.Fprintf(&b, "  \\item %s\n", esc(line))
		}
		b.WriteString(`\end{itemize}
`)
	}
	b.WriteString(`\end{minipage}
`)
	return b.String()
}

// renderSidebarMainLaTeX builds the wide column (experience history).
func renderSidebarMainLaTeX(doc ImprovedDoc, esc func(string) string) string {
	var b strings.Builder
	b.WriteString(`\begin{minipage}[t]{0.66\textwidth}
`)
	if len(doc.Experience) > 0 {
		b.WriteString(`\rsec{Experience}
`)
		renderRoles(&b, doc.Experience, esc)
	}
	b.WriteString(`\end{minipage}
`)
	return b.String()
}

// renderRoles writes the experience entries (title + org, period right-hfill,
// then impact bullets).
func renderRoles(b *strings.Builder, roles []ImprovedRole, esc func(string) string) {
	for _, role := range roles {
		title := role.Title
		if role.Org != "" {
			if title != "" {
				title += " — " + role.Org
			} else {
				title = role.Org
			}
		}
		fmt.Fprintf(b, `\textbf{%s}`, esc(title))
		if role.Period != "" {
			fmt.Fprintf(b, ` \hfill %s`, esc(role.Period))
		}
		b.WriteString("\n")
		if len(role.Bullets) > 0 {
			b.WriteString("\\begin{itemize}\n")
			for _, bullet := range role.Bullets {
				fmt.Fprintf(b, "  \\item %s\n", esc(bullet))
			}
			b.WriteString("\\end{itemize}\n")
		}
		b.WriteString("\\vspace{0.4em}\n")
	}
}

// latexAccentHex normalizes a "#rrggbb" accent to an HTML hex token for xcolor.
func latexAccentHex(hex string) string {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) == 6 {
		return strings.ToUpper(hex)
	}
	return "059669" // classic green default
}

func latexEscape(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\textbackslash{}`,
		`&`, `\&`,
		`%`, `\%`,
		`$`, `\$`,
		`#`, `\#`,
		`_`, `\_`,
		`{`, `\{`,
		`}`, `\}`,
		`~`, `\textasciitilde{}`,
		`^`, `\textasciicircum{}`,
	)
	return replacer.Replace(s)
}

// FormatHint explains PDF prerequisites for the UI.
func FormatHint(f Format) string {
	switch f {
	case FormatMarkdown:
		return "Best for preview & copy-paste"
	case FormatLaTeX:
		return "Editable professional typesetting"
	case FormatPDF:
		return "Always saved — LaTeX/pandoc if available, else Nexus PDF"
	default:
		return ""
	}
}

// ReadyChecklist returns human blockers before generate.
func ReadyChecklist(aiOn bool, hasResume bool, projectCount int) []string {
	var miss []string
	if !aiOn {
		miss = append(miss, "Enable AI Assist in Config")
	}
	if !hasResume {
		miss = append(miss, "Set a resume path in Config")
	}
	if projectCount == 0 {
		miss = append(miss, "Add work projects under Resume → Work (aim for 4–6)")
	} else if projectCount < 4 {
		miss = append(miss, fmt.Sprintf("Only %d work project(s) — 4–6 repos gives a stronger rewrite", projectCount))
	}
	return miss
}
