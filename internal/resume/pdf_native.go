package resume

import (
	"fmt"
	"strings"

	"github.com/phpdave11/gofpdf"
)

// RenderNativePDF writes a polished single-page-friendly PDF from structured content.
func RenderNativePDF(doc ImprovedDoc, path string) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(16, 14, 16)
	pdf.SetAutoPageBreak(true, 14)
	pdf.AddPage()

	left, right := 16.0, 194.0
	width := right - left

	name := strings.TrimSpace(doc.FullName)
	if name == "" {
		name = "Resume"
	}

	// Name
	pdf.SetTextColor(17, 24, 39) // near-black
	pdf.SetFont("Helvetica", "B", 22)
	pdf.CellFormat(width, 10, name, "", 1, "L", false, 0, "")

	// Headline
	if doc.Headline != "" {
		pdf.SetTextColor(55, 65, 81)
		pdf.SetFont("Helvetica", "", 11)
		pdf.CellFormat(width, 6, doc.Headline, "", 1, "L", false, 0, "")
	}

	// Accent rule under header
	pdf.Ln(2)
	pdf.SetDrawColor(5, 150, 105)
	pdf.SetLineWidth(0.8)
	y := pdf.GetY()
	pdf.Line(left, y, right, y)
	pdf.SetLineWidth(0.2)
	pdf.Ln(4)

	writeSection := func(title string) {
		pdf.Ln(1)
		pdf.SetTextColor(5, 150, 105)
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(width, 6, strings.ToUpper(title), "", 1, "L", false, 0, "")
		pdf.SetDrawColor(229, 231, 235)
		y := pdf.GetY()
		pdf.Line(left, y, right, y)
		pdf.Ln(2.5)
		pdf.SetTextColor(17, 24, 39)
	}

	if doc.Summary != "" {
		writeSection("Summary")
		pdf.SetFont("Helvetica", "", 10)
		pdf.MultiCell(width, 5, doc.Summary, "", "J", false)
		pdf.Ln(1)
	}

	if len(doc.Skills) > 0 {
		writeSection("Skills")
		pdf.SetFont("Helvetica", "", 10)
		// Wrap skills nicely with middots
		pdf.MultiCell(width, 5, strings.Join(doc.Skills, "  ·  "), "", "", false)
		pdf.Ln(1)
	}

	if len(doc.Experience) > 0 {
		writeSection("Experience")
		for _, role := range doc.Experience {
			title := strings.TrimSpace(role.Title)
			org := strings.TrimSpace(role.Org)
			head := title
			if org != "" {
				if head != "" {
					head += "  ·  " + org
				} else {
					head = org
				}
			}
			// Title left, period right on one row
			rowY := pdf.GetY()
			pdf.SetFont("Helvetica", "B", 10)
			pdf.SetTextColor(17, 24, 39)
			pdf.CellFormat(width*0.68, 5.5, head, "", 0, "L", false, 0, "")
			if role.Period != "" {
				pdf.SetFont("Helvetica", "", 9)
				pdf.SetTextColor(107, 114, 128)
				pdf.CellFormat(width*0.32, 5.5, role.Period, "", 1, "R", false, 0, "")
			} else {
				pdf.Ln(5.5)
			}
			_ = rowY
			pdf.SetTextColor(31, 41, 55)
			pdf.SetFont("Helvetica", "", 10)
			for _, b := range role.Bullets {
				b = strings.TrimSpace(b)
				if b == "" {
					continue
				}
				// hanging bullet
				x := pdf.GetX()
				y := pdf.GetY()
				pdf.SetX(left)
				pdf.CellFormat(4, 5, "•", "", 0, "L", false, 0, "")
				pdf.MultiCell(width-4, 5, b, "", "", false)
				pdf.SetXY(x, pdf.GetY())
				_ = y
			}
			pdf.Ln(2.5)
		}
	}

	if len(doc.Education) > 0 {
		writeSection("Education")
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetTextColor(31, 41, 55)
		for _, line := range doc.Education {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			pdf.CellFormat(4, 5, "•", "", 0, "L", false, 0, "")
			pdf.MultiCell(width-4, 5, line, "", "", false)
		}
	}

	if err := pdf.OutputFileAndClose(path); err != nil {
		return fmt.Errorf("write pdf: %w", err)
	}
	return nil
}
