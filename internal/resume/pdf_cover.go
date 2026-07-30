package resume

import (
	"fmt"
	"strings"

	"github.com/phpdave11/gofpdf"
)

// RenderNativeCoverPDF writes a clean cover letter PDF without any external
// LaTeX engine. It is the always-available fallback for EnsureCoverPDF.
func RenderNativeCoverPDF(cl CoverLetter, path string) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(22, 20, 22)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()
	width := 166.0 // 210 - 22 - 22

	if cl.Subject != "" {
		pdf.SetTextColor(17, 24, 39)
		pdf.SetFont("Helvetica", "B", 12)
		pdf.MultiCell(width, 6.5, cl.Subject, "", "L", false)
		pdf.Ln(4)
	}

	pdf.SetTextColor(31, 41, 55)
	pdf.SetFont("Helvetica", "", 11)

	greeting := cl.Greeting
	if greeting == "" {
		greeting = "Dear Hiring Team,"
	}
	pdf.MultiCell(width, 6, greeting, "", "L", false)
	pdf.Ln(2)

	for _, p := range cl.Paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pdf.MultiCell(width, 6, p, "", "J", false)
		pdf.Ln(2)
	}

	closing := cl.Closing
	if closing == "" {
		closing = "Sincerely,"
	}
	pdf.Ln(2)
	pdf.MultiCell(width, 6, closing, "", "L", false)
	if cl.Signature != "" {
		pdf.SetFont("Helvetica", "B", 11)
		pdf.MultiCell(width, 6, cl.Signature, "", "L", false)
	}

	if err := pdf.OutputFileAndClose(path); err != nil {
		return fmt.Errorf("write cover letter pdf: %w", err)
	}
	return nil
}
