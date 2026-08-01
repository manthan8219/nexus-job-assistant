package resume

import (
	"fmt"
	"strings"

	"github.com/phpdave11/gofpdf"
)

// RenderNativePDF writes a polished single-page-friendly PDF from structured
// content using the default Classic template (kept for TUI/tailor callers).
func RenderNativePDF(doc ImprovedDoc, path string) error {
	tpl, _ := GetTemplate(TemplateClassic)
	return RenderNativePDFFor(doc, tpl, path)
}

// RenderNativePDFFor writes a PDF whose layout follows the template manifest.
// This is the fallback when no LaTeX engine is installed; design fidelity
// mirrors the LaTeX renderer as closely as gofpdf allows.
func RenderNativePDFFor(doc ImprovedDoc, tpl Template, path string) error {
	_, err := RenderNativePDFForCounted(doc, tpl, path)
	return err
}

// RenderNativePDFForCounted renders like RenderNativePDFFor and also reports
// how many pages the produced document uses. The fit pipeline uses it to
// verify the content really fits the template's target page count.
func RenderNativePDFForCounted(doc ImprovedDoc, tpl Template, path string) (int, error) {
	if tpl.Layout == LayoutSidebar {
		return renderNativeSidebar(doc, tpl, path)
	}
	return renderNativeColumn(doc, tpl, path)
}

// renderNativeColumn renders the single-column templates with design tokens
// from the manifest (margin, fonts, section style, name style, contact line).
func renderNativeColumn(doc ImprovedDoc, tpl Template, path string) (int, error) {
	design := tpl.Design
	margin := design.NativeMargin
	if margin <= 0 {
		margin = 16.0
	}
	body := design.NativeSize
	if body <= 0 {
		body = 10
	}
	nameSize := design.NativeName
	if nameSize <= 0 {
		nameSize = 22
	}
	lead := design.NativeLead
	if lead <= 0 {
		lead = 5
	}
	accent := design.AccentRGB
	if accent == [3]int{} {
		accent = [3]int{5, 150, 105}
	}
	align := "L"
	if design.HeaderAlign == "center" {
		align = "C"
	}
	ff := fontFor(tpl.BodyFont)

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(margin, 14, margin)
	pdf.SetAutoPageBreak(true, 14)
	pdf.AddPage()

	left, right := margin, 210.0-margin
	width := right - left

	name := strings.TrimSpace(doc.FullName)
	if name == "" {
		name = "Resume"
	}

	// Name (accent-colored for Macchiato-style headers)
	nameColor := [3]int{17, 24, 39}
	if tpl.NameStyle == NameStyleColored {
		nameColor = accent
	}
	pdf.SetTextColor(nameColor[0], nameColor[1], nameColor[2])
	pdf.SetFont(ff, "B", float64(nameSize))
	pdf.SetX(left)
	pdf.CellFormat(width, 10, name, "", 1, align, false, 0, "")

	// Headline
	if doc.Headline != "" {
		pdf.SetTextColor(accent[0], accent[1], accent[2])
		pdf.SetFont(ff, "B", 11)
		pdf.SetX(left)
		pdf.CellFormat(width, 6, doc.Headline, "", 1, align, false, 0, "")
	}

	// Contact line (moderncv banking style): email · phone · location
	if tpl.ContactLine {
		var parts []string
		for _, p := range []string{doc.Email, doc.Phone, doc.Location} {
			if strings.TrimSpace(p) != "" {
				parts = append(parts, p)
			}
		}
		if len(parts) > 0 {
			pdf.SetTextColor(107, 114, 128)
			pdf.SetFont(ff, "", 9)
			pdf.SetX(left)
			pdf.CellFormat(width, 5, strings.Join(parts, "  ·  "), "", 1, align, false, 0, "")
		}
	}

	// Accent rule under header
	if design.ShowRule {
		pdf.Ln(2)
		pdf.SetDrawColor(accent[0], accent[1], accent[2])
		pdf.SetLineWidth(0.8)
		y := pdf.GetY()
		pdf.Line(left, y, right, y)
		pdf.SetLineWidth(0.2)
		pdf.Ln(4)
	}

	writeSection := func(title string) {
		pdf.Ln(1)
		switch tpl.SectionStyle {
		case SectionStyleCaps: // Jake: small-caps style heading, no rule
			pdf.SetTextColor(accent[0], accent[1], accent[2])
			pdf.SetFont(ff, "B", float64(body))
			pdf.SetX(left)
			pdf.CellFormat(width, 6, strings.ToUpper(title), "", 1, "L", false, 0, "")
			pdf.Ln(1.5)
			pdf.SetTextColor(17, 24, 39)
		case SectionStyleMarker: // Awesome-CV: filled accent square + heading + rule
			pdf.SetFillColor(accent[0], accent[1], accent[2])
			sq := 3.4
			y := pdf.GetY() + 1.2
			pdf.Rect(left, y, sq, sq, "F")
			pdf.SetTextColor(accent[0], accent[1], accent[2])
			pdf.SetFont(ff, "B", float64(body))
			pdf.SetX(left + sq + 2)
			pdf.CellFormat(width-sq-2, 6, strings.ToUpper(title), "", 1, "L", false, 0, "")
			pdf.SetDrawColor(203, 213, 225)
			y = pdf.GetY()
			pdf.Line(left, y, right, y)
			pdf.Ln(2.5)
			pdf.SetTextColor(17, 24, 39)
		case SectionStyleRuleAbove: // Banking: rule above the heading
			pdf.SetDrawColor(accent[0], accent[1], accent[2])
			y := pdf.GetY()
			pdf.Line(left, y, right, y)
			pdf.Ln(3)
			pdf.SetTextColor(accent[0], accent[1], accent[2])
			pdf.SetFont(ff, "B", float64(body))
			pdf.SetX(left)
			pdf.CellFormat(width, 6, strings.ToUpper(title), "", 1, "L", false, 0, "")
			pdf.Ln(1)
			pdf.SetTextColor(17, 24, 39)
		case SectionStyleSoft: // McDowell: sentence-case gray heading, no rule
			pdf.SetTextColor(100, 116, 139)
			pdf.SetFont(ff, "B", float64(body))
			pdf.SetX(left)
			pdf.CellFormat(width, 6, title, "", 1, "L", false, 0, "")
			pdf.Ln(1.5)
			pdf.SetTextColor(17, 24, 39)
		default: // plain: uppercase heading + light rule
			pdf.SetTextColor(accent[0], accent[1], accent[2])
			pdf.SetFont(ff, "B", float64(body))
			pdf.SetX(left)
			pdf.CellFormat(width, 6, strings.ToUpper(title), "", 1, "L", false, 0, "")
			pdf.SetDrawColor(229, 231, 235)
			y := pdf.GetY()
			pdf.Line(left, y, right, y)
			pdf.Ln(2.5)
			pdf.SetTextColor(17, 24, 39)
		}
	}

	for _, sec := range tpl.Sections {
		switch sec.Key {
		case SectionSummary:
			if doc.Summary != "" {
				writeSection(sec.Label)
				pdf.SetFont(ff, "", float64(body))
				pdf.MultiCell(width, lead, doc.Summary, "", "J", false)
				pdf.Ln(1)
			}
		case SectionSkills:
			if len(doc.Skills) > 0 {
				writeSection(sec.Label)
				pdf.SetFont(ff, "", float64(body))
				pdf.MultiCell(width, lead, strings.Join(doc.Skills, "  ·  "), "", "", false)
				pdf.Ln(1)
			}
		case SectionExperience:
			if len(doc.Experience) > 0 {
				writeSection(sec.Label)
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
					pdf.SetFont(ff, "B", float64(body))
					pdf.SetTextColor(17, 24, 39)
					pdf.SetX(left)
					pdf.CellFormat(width*0.68, 5.5, head, "", 0, "L", false, 0, "")
					if role.Period != "" {
						pdf.SetFont(ff, "", float64(body-1))
						pdf.SetTextColor(107, 114, 128)
						pdf.CellFormat(width*0.32, 5.5, role.Period, "", 1, "R", false, 0, "")
					} else {
						pdf.Ln(5.5)
					}
					pdf.SetTextColor(31, 41, 55)
					pdf.SetFont(ff, "", float64(body))
					for _, b := range role.Bullets {
						b = strings.TrimSpace(b)
						if b == "" {
							continue
						}
						x := pdf.GetX()
						pdf.SetX(left)
						pdf.CellFormat(4, lead, "•", "", 0, "L", false, 0, "")
						pdf.MultiCell(width-4, lead, b, "", "", false)
						pdf.SetXY(x, pdf.GetY())
					}
					pdf.Ln(2.5)
				}
			}
		case SectionEducation:
			if len(doc.Education) > 0 {
				writeSection(sec.Label)
				pdf.SetFont(ff, "", float64(body))
				pdf.SetTextColor(31, 41, 55)
				for _, line := range doc.Education {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					pdf.SetX(left)
					pdf.CellFormat(4, lead, "•", "", 0, "L", false, 0, "")
					pdf.MultiCell(width-4, lead, line, "", "", false)
				}
			}
		}
	}

	if err := pdf.OutputFileAndClose(path); err != nil {
		return 0, fmt.Errorf("write pdf: %w", err)
	}
	return pdf.PageNo(), nil
}

// fontFor maps a template body-font token to a core gofpdf font family.
func fontFor(bodyFont string) string {
	switch bodyFont {
	case "mono":
		return "Courier"
	case "serif":
		return "Times"
	default:
		return "Helvetica"
	}
}

// lightenRGB mixes an RGB color toward white by f (0-1).
func lightenRGB(c [3]int, f float64) [3]int {
	return [3]int{
		int(float64(c[0]) + float64(255-c[0])*f),
		int(float64(c[1]) + float64(255-c[1])*f),
		int(float64(c[2]) + float64(255-c[2])*f),
	}
}

// renderNativeSidebar renders the Sidebar/Split templates: full-width header
// and summary, then skills + education in a rail on the template-declared side
// and experience in the main column.
func renderNativeSidebar(doc ImprovedDoc, tpl Template, path string) (int, error) {
	design := tpl.Design
	accent := design.AccentRGB
	if accent == [3]int{} {
		accent = [3]int{34, 211, 238}
	}
	body := design.NativeSize
	if body <= 0 {
		body = 10
	}
	nameSize := design.NativeName
	if nameSize <= 0 {
		nameSize = 22
	}
	lead := design.NativeLead
	if lead <= 0 {
		lead = 5
	}
	ff := fontFor(tpl.BodyFont)

	const margin = 14.0
	const top = 14.0
	const bottom = 297.0 - 14.0
	left := margin
	right := 210.0 - margin
	width := right - left
	gap := 6.0
	// ColumnRatio widens the main column for Deedy-style asymmetric layouts.
	ratio := tpl.ColumnRatio
	if ratio <= 0 {
		ratio = 0.66
	}
	colW := width*ratio - gap/2
	railW := width - colW - gap
	railX := left
	mainX := left + railW + gap
	if tpl.RailSide == "right" {
		railX = left + colW + gap
		mainX = left
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(left, top, 16)
	pdf.SetAutoPageBreak(false, 14)
	pdf.AddPage()

	y := top
	// Header (centered)
	name := strings.TrimSpace(doc.FullName)
	if name == "" {
		name = "Resume"
	}
	nameColor := [3]int{17, 24, 39}
	if tpl.NameStyle == NameStyleColored {
		nameColor = accent
	}
	pdf.SetTextColor(nameColor[0], nameColor[1], nameColor[2])
	pdf.SetFont(ff, "B", float64(nameSize))
	pdf.SetX(left)
	pdf.CellFormat(width, 10, name, "", 1, "C", false, 0, "")
	if doc.Headline != "" {
		pdf.SetTextColor(accent[0], accent[1], accent[2])
		pdf.SetFont(ff, "B", 11)
		pdf.SetX(left)
		pdf.CellFormat(width, 6, doc.Headline, "", 1, "C", false, 0, "")
	}
	y = pdf.GetY() + 2
	pdf.SetDrawColor(accent[0], accent[1], accent[2])
	pdf.SetLineWidth(0.8)
	pdf.Line(left, y, right, y)
	pdf.SetLineWidth(0.2)
	y += 5

	// Summary (full width)
	if doc.Summary != "" {
		if y+8 > bottom {
			pdf.AddPage()
			y = top
		}
		pdf.SetTextColor(accent[0], accent[1], accent[2])
		pdf.SetFont(ff, "B", float64(body))
		pdf.SetXY(left, y)
		pdf.CellFormat(width, 6, "SUMMARY", "", 1, "L", false, 0, "")
		y = pdf.GetY()
		pdf.SetDrawColor(229, 231, 235)
		pdf.Line(left, y, right, y)
		pdf.SetTextColor(17, 24, 39)
		pdf.SetFont(ff, "", float64(body))
		pdf.SetXY(left, y+2.5)
		pdf.MultiCell(width, lead, doc.Summary, "", "J", false)
		y = pdf.GetY() + 3
	}

	colY := y
	// Rail background fill (Kendall dark, Macchiato accent, tint).
	var railFill [3]int
	drawRailBg := false
	railInk := [3]int{31, 41, 55}
	railHead := accent
	switch tpl.RailBackground {
	case "dark":
		drawRailBg = true
		railFill = [3]int{17, 24, 39}
		railInk = [3]int{255, 255, 255}
		railHead = [3]int{255, 255, 255}
	case "accent":
		drawRailBg = true
		railFill = accent
		railInk = [3]int{255, 255, 255}
		railHead = [3]int{255, 255, 255}
	case "tint":
		drawRailBg = true
		railFill = lightenRGB(accent, 0.9)
	}
	if drawRailBg {
		pdf.SetFillColor(railFill[0], railFill[1], railFill[2])
		pdf.Rect(railX, colY, railW, bottom-colY, "F")
	}
	railTitle := func(title string) {
		if colY+8 > bottom {
			pdf.AddPage()
			colY = top
			if drawRailBg {
				pdf.SetFillColor(railFill[0], railFill[1], railFill[2])
				pdf.Rect(railX, colY, railW, bottom-colY, "F")
			}
		}
		pdf.SetTextColor(railHead[0], railHead[1], railHead[2])
		pdf.SetFont(ff, "B", float64(body))
		pdf.SetXY(railX, colY)
		pdf.CellFormat(railW, 6, title, "", 1, "L", false, 0, "")
		colY = pdf.GetY() + 1.5
	}
	railBody := func(line string) {
		if colY+lead > bottom {
			pdf.AddPage()
			colY = top
			if drawRailBg {
				pdf.SetFillColor(railFill[0], railFill[1], railFill[2])
				pdf.Rect(railX, colY, railW, bottom-colY, "F")
			}
		}
		pdf.SetTextColor(railInk[0], railInk[1], railInk[2])
		pdf.SetFont(ff, "", float64(body))
		pdf.SetXY(railX, colY)
		pdf.MultiCell(railW, lead, "•  "+line, "", "", false)
		colY = pdf.GetY()
	}

	// Left rail: skills + education
	if len(doc.Skills) > 0 {
		railTitle("SKILLS")
		for _, s := range doc.Skills {
			railBody(s)
		}
		colY += 2
	}
	if len(doc.Education) > 0 {
		railTitle("EDUCATION")
		for _, line := range doc.Education {
			railBody(line)
		}
	}

	// Right column: experience
	mainY := y
	if len(doc.Experience) > 0 {
		if mainY+8 > bottom {
			pdf.AddPage()
			mainY = top
		}
		pdf.SetTextColor(accent[0], accent[1], accent[2])
		pdf.SetFont(ff, "B", float64(body))
		pdf.SetXY(mainX, mainY)
		pdf.CellFormat(colW, 6, "EXPERIENCE", "", 1, "L", false, 0, "")
		mainY = pdf.GetY() + 2
		pdf.SetDrawColor(229, 231, 235)
		pdf.Line(mainX, mainY, right, mainY)
		mainY += 2.5
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
			if mainY+8 > bottom {
				pdf.AddPage()
				mainY = top
			}
			pdf.SetFont(ff, "B", float64(body))
			pdf.SetTextColor(17, 24, 39)
			pdf.SetXY(mainX, mainY)
			pdf.CellFormat(colW*0.72, 5.5, head, "", 0, "L", false, 0, "")
			if role.Period != "" {
				pdf.SetFont(ff, "", float64(body-1))
				pdf.SetTextColor(107, 114, 128)
				pdf.CellFormat(colW*0.28, 5.5, role.Period, "", 1, "R", false, 0, "")
			} else {
				pdf.Ln(5.5)
			}
			mainY = pdf.GetY()
			pdf.SetTextColor(31, 41, 55)
			pdf.SetFont(ff, "", float64(body))
			for _, b := range role.Bullets {
				b = strings.TrimSpace(b)
				if b == "" {
					continue
				}
				if mainY+lead > bottom {
					pdf.AddPage()
					mainY = top
				}
				pdf.SetXY(mainX, mainY)
				pdf.CellFormat(4, lead, "•", "", 0, "L", false, 0, "")
				pdf.MultiCell(colW-4, lead, b, "", "", false)
				mainY = pdf.GetY()
			}
			mainY += 2.5
		}
	}

	if err := pdf.OutputFileAndClose(path); err != nil {
		return 0, fmt.Errorf("write pdf: %w", err)
	}
	return pdf.PageNo(), nil
}
