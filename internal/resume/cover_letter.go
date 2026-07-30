package resume

import (
	"strings"
)

// CoverLetter is a generated application cover letter: structured content the
// model returns; Nexus renders Markdown / LaTeX / PDF locally.
type CoverLetter struct {
	Subject    string   `json:"subject,omitempty"`
	Greeting   string   `json:"greeting"`
	Paragraphs []string `json:"paragraphs"`
	Closing    string   `json:"closing"`
	Signature  string   `json:"signature,omitempty"`
}

// RenderCoverLetterMarkdown turns a CoverLetter into clean plain Markdown.
func RenderCoverLetterMarkdown(cl CoverLetter) string {
	var b strings.Builder
	if cl.Subject != "" {
		b.WriteString("**" + cl.Subject + "**\n\n")
	}
	greeting := cl.Greeting
	if greeting == "" {
		greeting = "Dear Hiring Team,"
	}
	b.WriteString(greeting + "\n\n")
	for _, p := range cl.Paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		b.WriteString(p + "\n\n")
	}
	closing := cl.Closing
	if closing == "" {
		closing = "Sincerely,"
	}
	b.WriteString(closing + "\n")
	if cl.Signature != "" {
		b.WriteString(cl.Signature + "\n")
	}
	return strings.TrimSpace(b.String()) + "\n"
}

// RenderCoverLetterLaTeX turns a CoverLetter into a simple article-class
// letter matching the resume template's typography.
func RenderCoverLetterLaTeX(cl CoverLetter) string {
	esc := latexEscape
	var b strings.Builder
	b.WriteString(`\documentclass[11pt,a4paper]{article}
\usepackage[margin=1in]{geometry}
\usepackage[T1]{fontenc}
\usepackage{hyperref}
\pagestyle{empty}
\setlength{\parindent}{0pt}
\setlength{\parskip}{0.8em}
\begin{document}
`)
	if cl.Subject != "" {
		b.WriteString("\\textbf{" + esc(cl.Subject) + "}\n\n")
	}
	greeting := cl.Greeting
	if greeting == "" {
		greeting = "Dear Hiring Team,"
	}
	b.WriteString(esc(greeting) + "\n\n")
	for _, p := range cl.Paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		b.WriteString(esc(p) + "\n\n")
	}
	closing := cl.Closing
	if closing == "" {
		closing = "Sincerely,"
	}
	b.WriteString(esc(closing) + "\\\\\n")
	if cl.Signature != "" {
		b.WriteString(esc(cl.Signature) + "\n")
	}
	b.WriteString("\\end{document}\n")
	return b.String()
}
