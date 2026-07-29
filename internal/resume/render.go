package resume

import (
	"fmt"
	"strings"
)

// RenderMarkdown turns an ImprovedDoc into a clean Markdown resume.
func RenderMarkdown(doc ImprovedDoc) string {
	var b strings.Builder
	if doc.FullName != "" {
		b.WriteString("# " + doc.FullName + "\n\n")
	} else {
		b.WriteString("# Resume\n\n")
	}
	if doc.Headline != "" {
		b.WriteString("**" + doc.Headline + "**\n\n")
	}
	if doc.Summary != "" {
		b.WriteString("## Summary\n\n" + doc.Summary + "\n\n")
	}
	if len(doc.Skills) > 0 {
		b.WriteString("## Skills\n\n" + strings.Join(doc.Skills, " · ") + "\n\n")
	}
	if len(doc.Experience) > 0 {
		b.WriteString("## Experience\n\n")
		for _, role := range doc.Experience {
			head := role.Title
			if role.Org != "" {
				if head != "" {
					head += " — " + role.Org
				} else {
					head = role.Org
				}
			}
			b.WriteString("### " + head + "\n")
			if role.Period != "" {
				b.WriteString("*" + role.Period + "*\n\n")
			} else {
				b.WriteString("\n")
			}
			for _, bullet := range role.Bullets {
				b.WriteString("- " + bullet + "\n")
			}
			b.WriteString("\n")
		}
	}
	if len(doc.Education) > 0 {
		b.WriteString("## Education\n\n")
		for _, line := range doc.Education {
			b.WriteString("- " + line + "\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()) + "\n"
}

// RenderLaTeX turns an ImprovedDoc into a simple article-class resume.
func RenderLaTeX(doc ImprovedDoc) string {
	esc := latexEscape
	var b strings.Builder
	b.WriteString(`\documentclass[11pt,a4paper]{article}
\usepackage[margin=0.75in]{geometry}
\usepackage[T1]{fontenc}
\usepackage{enumitem}
\usepackage{hyperref}
\pagestyle{empty}
\setlist[itemize]{leftmargin=*,itemsep=0.15em,topsep=0.2em}
\begin{document}
`)
	name := doc.FullName
	if name == "" {
		name = "Resume"
	}
	b.WriteString(`\begin{center}
{\LARGE\textbf{` + esc(name) + `}}\\[0.35em]
`)
	if doc.Headline != "" {
		b.WriteString(esc(doc.Headline) + "\\\\\n")
	}
	b.WriteString(`\end{center}
\vspace{0.6em}
`)
	if doc.Summary != "" {
		b.WriteString(`\section*{Summary}
` + esc(doc.Summary) + "\n\n")
	}
	if len(doc.Skills) > 0 {
		b.WriteString(`\section*{Skills}
` + esc(strings.Join(doc.Skills, " · ")) + "\n\n")
	}
	if len(doc.Experience) > 0 {
		b.WriteString(`\section*{Experience}
`)
		for _, role := range doc.Experience {
			title := role.Title
			if role.Org != "" {
				if title != "" {
					title += " — " + role.Org
				} else {
					title = role.Org
				}
			}
			b.WriteString(`\textbf{` + esc(title) + `}`)
			if role.Period != "" {
				b.WriteString(` \hfill ` + esc(role.Period))
			}
			b.WriteString("\n")
			if len(role.Bullets) > 0 {
				b.WriteString("\\begin{itemize}\n")
				for _, bullet := range role.Bullets {
					b.WriteString("  \\item " + esc(bullet) + "\n")
				}
				b.WriteString("\\end{itemize}\n")
			}
			b.WriteString("\\vspace{0.4em}\n")
		}
	}
	if len(doc.Education) > 0 {
		b.WriteString(`\section*{Education}
\begin{itemize}
`)
		for _, line := range doc.Education {
			b.WriteString("  \\item " + esc(line) + "\n")
		}
		b.WriteString("\\end{itemize}\n")
	}
	b.WriteString("\\end{document}\n")
	return b.String()
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
