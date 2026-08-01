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
	rsec := sectionMacro(tpl.SectionStyle)
	fmt.Fprintf(&b, `\documentclass[%dpt,a4paper]{article}
\usepackage[margin=%s]{geometry}
\usepackage[T1]{fontenc}
\usepackage{enumitem}
\usepackage{hyperref}
\usepackage{xcolor}
\definecolor{accent}{HTML}{%s}
\pagestyle{empty}
\setlist[itemize]{leftmargin=*,itemsep=%s,topsep=%s}
\newcommand{\rsec}[1]{%s}
`, size, margin, latexAccentHex(tpl.AccentHex), itemSep, topSep, rsec)
	if tpl.SectionStyle == SectionStyleSoft {
		b.WriteString(`\definecolor{softsec}{HTML}{6b7280}
`)
	}
	if tpl.RailBackground != "" {
		fmt.Fprintf(&b, `\definecolor{railbg}{HTML}{%s}
`, railBackgroundHex(tpl))
	}

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
	nameColor := ""
	if tpl.NameStyle == NameStyleColored {
		nameColor = `\color{accent}`
	}
	headline := ""
	if doc.Headline != "" {
		if tpl.SectionStyle == SectionStyleSoft {
			headline = `\noindent{\color{softsec}\textbf{` + esc(doc.Headline) + `}}`
		} else {
			headline = `\noindent{\color{accent}\textbf{` + esc(doc.Headline) + `}}`
		}
	}
	if tpl.HeaderAlign == "left" && tpl.NameStyle != NameStyleCentered {
		b.WriteString(`\noindent{\LARGE` + nameColor + `\textbf{` + esc(name) + `}}\\[0.15em]
`)
		if headline != "" {
			b.WriteString(headline + `\\[0.2em]
`)
		}
	} else {
		b.WriteString(`\begin{center}
{\LARGE` + nameColor + `\textbf{` + esc(name) + `}}\\[0.25em]
`)
		if headline != "" {
			b.WriteString(headline + `\\[0.2em]
`)
		}
		b.WriteString(`\end{center}
`)
	}
	if tpl.ContactLine {
		if parts := contactParts(doc); len(parts) > 0 {
			fmt.Fprintf(&b, `\begin{center}\small\textcolor{gray}{%s}\end{center}

`, esc(strings.Join(parts, "  ·  ")))
		}
	}
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
	railW, mainW := sidebarColumns(tpl)
	rail := renderSidebarRailLaTeX(doc, esc)
	// Restring the hardcoded rail/main widths to the template's column ratio
	// (Deedy is asymmetric: 0.76 main / 0.22 rail) and wrap a colored rail.
	rail = strings.Replace(rail, "{0.30", fmt.Sprintf("{%g", railW), 1)
	main := renderSidebarMainLaTeX(doc, esc)
	main = strings.Replace(main, "{0.66", fmt.Sprintf("{%g", mainW), 1)
	if tpl.RailBackground != "" {
		rail = colorizeRailLaTeX(rail, railW)
	}
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

// sidebarColumns derives the rail/main width fractions, honoring the template's
// ColumnRatio for asymmetric designs (Deedy = 0.76 main / 0.22 rail).
func sidebarColumns(tpl Template) (railW, mainW float64) {
	ratio := tpl.ColumnRatio
	if ratio <= 0 {
		ratio = 0.66
	}
	mainW = ratio
	railW = 1.0 - ratio - 0.02
	if railW < 0.18 {
		railW = 0.18
	}
	return railW, mainW
}

// colorizeRailLaTeX wraps a plain rail minipage in a colored box with white
// text (Kendall dark rail, Macchiato accent rail). The minipage content is
// untouched — we only wrap it and flip the text color.
func colorizeRailLaTeX(rail string, railW float64) string {
	rail = strings.Replace(rail, `\begin{minipage}[t]{`, `{\colorbox{railbg}{\begin{minipage}[t]{`, 1)
	rail = strings.Replace(rail, `\textwidth}`, `\textwidth}\color{white}`, 1)
	rail = strings.Replace(rail, `\end{minipage}`, `\end{minipage}}}`, 1)
	return rail
}

// sectionMacro returns the LaTeX \rsec body for a section style token.
func sectionMacro(style string) string {
	switch style {
	case SectionStyleCaps:
		return `\par\vspace{0.55em}\noindent{\color{accent}\textbf{\textsc{#1}}}\vspace{0.2em}\par`
	case SectionStyleMarker:
		return `\par\vspace{0.6em}\noindent{\color{accent}\raisebox{1.5pt}{\rule{2.6mm}{2.6mm}}}\hspace{2mm}{\color{accent}\textbf{\MakeUppercase{#1}}}\par\vspace{0.15em}{\color{accent}\rule{\textwidth}{0.3pt}}\par`
	case SectionStyleRuleAbove:
		return `\par\vspace{0.5em}\noindent{\color{accent}\rule{\textwidth}{0.5pt}}\\[0.2em]{\color{accent}\textbf{\MakeUppercase{#1}}}\par`
	case SectionStyleSoft:
		return `\par\vspace{0.6em}\noindent{\color{softsec}\textbf{#1}}\vspace{0.2em}\par`
	default: // plain
		return `\par\vspace{0.6em}\noindent{\color{accent}\textbf{\MakeUppercase{#1}}}\vspace{0.25em}\par`
	}
}

// contactParts joins the document's contact details for a contact line.
func contactParts(doc ImprovedDoc) []string {
	var parts []string
	for _, p := range []string{doc.Email, doc.Phone, doc.Location} {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// railBackgroundHex returns the LaTeX fill color for a template's rail.
func railBackgroundHex(tpl Template) string {
	switch tpl.RailBackground {
	case "dark":
		return "111827"
	case "accent":
		return latexAccentHex(tpl.AccentHex)
	case "tint":
		return lightenHex(tpl.AccentHex, 0.9)
	default:
		return "ffffff"
	}
}

// lightenHex mixes a "#rrggbb" color toward white by f (0-1).
func lightenHex(hex string, f float64) string {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return "ffffff"
	}
	comp := func(s string) int {
		v := 0
		fmt.Sscanf(s, "%02x", &v)
		return v
	}
	r := int(float64(comp(hex[0:2])) + float64(255-comp(hex[0:2]))*f)
	g := int(float64(comp(hex[2:4])) + float64(255-comp(hex[2:4]))*f)
	bl := int(float64(comp(hex[4:6])) + float64(255-comp(hex[4:6]))*f)
	return fmt.Sprintf("%02x%02x%02x", r, g, bl)
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
