package resume

import (
	"fmt"
	"strings"
)

// Template ids for the curated registry. The classic template is the default
// and the fallback when a caller passes an empty id (old clients keep working).
const (
	TemplateClassic = "classic"
	TemplateModern  = "modern"
	TemplateSidebar = "sidebar"
	TemplateCompact = "compact"
)

// TemplateLayout describes the visual geometry of a resume template.
type TemplateLayout string

const (
	LayoutSingle  TemplateLayout = "single"  // classic single-column flow
	LayoutSidebar TemplateLayout = "sidebar" // two-column with a left rail
)

// SectionKey identifies a content slot a template knows how to render.
type SectionKey string

const (
	SectionSummary    SectionKey = "summary"
	SectionSkills     SectionKey = "skills"
	SectionExperience SectionKey = "experience"
	SectionEducation  SectionKey = "education"
)

// TemplateSection is one labelled slot in a template manifest.
type TemplateSection struct {
	Key   SectionKey `json:"key"`
	Label string     `json:"label"`
}

// TemplateDesign carries the design tokens each renderer (LaTeX / native PDF)
// needs to draw the template. It is deliberately not exposed over the API —
// the web UI only needs the manifest fields above.
type TemplateDesign struct {
	AccentRGB   [3]int  // rgb triple for rules, section titles, header accents
	HeaderAlign string  // "left" | "center"
	ShowRule    bool    // draw an accent rule under the header block
	LaTeXSize   int     // body font size (pt) for the LaTeX renderer
	LaTeXMargin string  // geometry margin for the LaTeX renderer (e.g. "0.7in")
	NativeSize  int     // native-PDF body font size
	NativeName  int     // native-PDF name font size
	NativeLead  float64 // native-PDF line leading
}

// Template is a machine-readable resume design. The manifest is what the
// system "understands" about a template: which sections it supports, in what
// order, on what geometry, and what constraints it imposes. The AI polish loop
// reads the same manifest so the content it writes already fits the design.
type Template struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Layout      TemplateLayout    `json:"layout"`
	Sections    []TemplateSection `json:"sections"`
	AccentHex   string            `json:"accentHex"`
	OnePage     bool              `json:"onePage"`
	ATSNote     string            `json:"atsNote"`
	Design      TemplateDesign    `json:"-"`
}

// TemplateIDs returns the ordered registry ids (used by the API docs/tests).
func TemplateIDs() []string {
	return []string{TemplateClassic, TemplateModern, TemplateSidebar, TemplateCompact}
}

// Templates returns the curated template registry.
func Templates() []Template {
	return []Template{classicTemplate(), modernTemplate(), sidebarTemplate(), compactTemplate()}
}

// GetTemplate resolves a template by id. An empty id resolves to Classic so
// existing callers (TUI, tailor) keep working unchanged.
func GetTemplate(id string) (Template, error) {
	if strings.TrimSpace(id) == "" {
		id = TemplateClassic
	}
	for _, t := range Templates() {
		if t.ID == id {
			return t, nil
		}
	}
	return Template{}, fmt.Errorf(
		"unknown resume template %q — choose one of: %s",
		id, strings.Join(TemplateIDs(), ", "),
	)
}

// classicTemplate is the single-column ATS-max layout that matches the
// original hardcoded renderer exactly.
func classicTemplate() Template {
	return Template{
		ID:          TemplateClassic,
		Name:        "Classic",
		Description: "Clean single-column flow with standard headings. The safest choice for ATS parsing.",
		Layout:      LayoutSingle,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#059669",
		OnePage:   false,
		ATSNote:   "Safest for ATS — single column, standard section names.",
		Design: TemplateDesign{
			AccentRGB:   [3]int{5, 150, 105},
			HeaderAlign: "left",
			ShowRule:    true,
			LaTeXSize:   11,
			LaTeXMargin: "0.75in",
			NativeSize:  10,
			NativeName:  22,
			NativeLead:  5,
		},
	}
}

// modernTemplate centers the header and adds a subtle accent to section
// titles — still single column, so it keeps ATS compatibility.
func modernTemplate() Template {
	return Template{
		ID:          TemplateModern,
		Name:        "Modern",
		Description: "Centered header with a violet accent. Single column, slightly more whitespace.",
		Layout:      LayoutSingle,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#8b5cf6",
		OnePage:   false,
		ATSNote:   "ATS-safe — single column with standard section names.",
		Design: TemplateDesign{
			AccentRGB:   [3]int{139, 92, 246},
			HeaderAlign: "center",
			ShowRule:    true,
			LaTeXSize:   11,
			LaTeXMargin: "0.8in",
			NativeSize:  10,
			NativeName:  24,
			NativeLead:  5,
		},
	}
}

// sidebarTemplate puts contact-style content (skills, education) in a left
// rail and the experience history in the main column.
func sidebarTemplate() Template {
	return Template{
		ID:          TemplateSidebar,
		Name:        "Sidebar",
		Description: "Two-column layout: skills and education in a left rail, experience as the main column.",
		Layout:      LayoutSidebar,
		Sections: []TemplateSection{
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#22d3ee",
		OnePage:   false,
		ATSNote:   "Design-forward — two columns can confuse some ATS systems; use for roles where design matters.",
		Design: TemplateDesign{
			AccentRGB:   [3]int{34, 211, 238},
			HeaderAlign: "center",
			ShowRule:    true,
			LaTeXSize:   10,
			LaTeXMargin: "0.6in",
			NativeSize:  10,
			NativeName:  22,
			NativeLead:  5,
		},
	}
}

// compactTemplate is a tighter single-column design aimed at one page —
// good for senior candidates with a long history.
func compactTemplate() Template {
	return Template{
		ID:          TemplateCompact,
		Name:        "Compact",
		Description: "Tighter spacing and smaller margins to fit more on one page.",
		Layout:      LayoutSingle,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#38bdf8",
		OnePage:   true,
		ATSNote:   "Optimized for one page — good for senior candidates.",
		Design: TemplateDesign{
			AccentRGB:   [3]int{56, 189, 248},
			HeaderAlign: "left",
			ShowRule:    true,
			LaTeXSize:   10,
			LaTeXMargin: "0.5in",
			NativeSize:  9,
			NativeName:  20,
			NativeLead:  4.5,
		},
	}
}

