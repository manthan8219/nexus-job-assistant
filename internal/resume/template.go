package resume

import (
	"fmt"
	"strings"
)

// Template ids for the curated registry. The classic template is the default
// and the fallback when a caller passes an empty id (old clients keep working).
const (
	TemplateClassic    = "classic"
	TemplateModern     = "modern"
	TemplateSidebar    = "sidebar"
	TemplateCompact    = "compact"
	TemplateExecutive  = "executive"
	TemplateMinimal    = "minimal"
	TemplateAcademic   = "academic"
	TemplateDeveloper  = "developer"
	TemplateSplit      = "split"
	TemplateBold       = "bold"
	TemplateMonochrome = "monochrome"
	TemplateNordic     = "nordic"
)

// TemplateLayout describes the visual geometry of a resume template.
type TemplateLayout string

const (
	LayoutSingle  TemplateLayout = "single"  // classic single-column flow
	LayoutSidebar TemplateLayout = "sidebar" // two-column with a rail
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

// SpaceBudget is how much content a template realistically holds on its target
// page count, derived from the design geometry (font size, margins, column
// width). The AI creator writes to this budget, the deterministic planner
// enforces it, and the renderer verifies the final page count.
type SpaceBudget struct {
	TargetPages       int `json:"targetPages"`       // 1 = must fit one page; 0 = flexible
	MaxSummaryLines   int `json:"maxSummaryLines"`   // summary lines the planner caps at
	MaxBulletsPerRole int `json:"maxBulletsPerRole"` // bullets per experience entry
	MaxRoles          int `json:"maxRoles"`          // experience entries
	MaxSkills         int `json:"maxSkills"`         // skills listed
	MaxEducation      int `json:"maxEducation"`      // education entries
	CharsPerLine      int `json:"charsPerLine"`      // ~chars that fit one body line
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
	RailSide    string            `json:"railSide,omitempty"` // "left" | "right" (sidebar layouts)
	BodyFont    string            `json:"bodyFont,omitempty"` // "sans" | "serif" | "mono"
	// Header alignment + rule tokens are promoted from Design before the
	// manifest is served so the web gallery can render faithful miniatures.
	HeaderAlign string `json:"headerAlign,omitempty"` // "left" | "center"
	// NOTE: no omitempty on ShowRule — `false` must round-trip so the UI
	// knows a template deliberately has no rule under the header.
	ShowRule bool           `json:"showRule"` // accent rule under the header
	Budget   SpaceBudget    `json:"budget,omitempty"`
	Design   TemplateDesign `json:"-"`
}

// TemplateIDs returns the ordered registry ids (used by the API docs/tests).
func TemplateIDs() []string {
	return []string{
		TemplateClassic, TemplateModern, TemplateSidebar, TemplateCompact,
		TemplateExecutive, TemplateMinimal, TemplateAcademic, TemplateDeveloper,
		TemplateSplit, TemplateBold, TemplateMonochrome, TemplateNordic,
	}
}

// Templates returns the curated template registry.
func Templates() []Template {
	templates := []Template{
		classicTemplate(),
		modernTemplate(),
		sidebarTemplate(),
		compactTemplate(),
		executiveTemplate(),
		minimalTemplate(),
		academicTemplate(),
		developerTemplate(),
		splitTemplate(),
		boldTemplate(),
		monochromeTemplate(),
		nordicTemplate(),
	}
	// Promote the design tokens the web preview needs (header alignment +
	// rule) onto the served manifest so UI miniatures stay faithful to the
	// real renderers, and attach each template's explicit space budget so the
	// UI + AI both know how much content it holds.
	for i := range templates {
		templates[i].HeaderAlign = templates[i].Design.HeaderAlign
		templates[i].ShowRule = templates[i].Design.ShowRule
		templates[i].Budget = budgetFor(templates[i])
	}
	return templates
}

// budgetFor returns the explicit space budget for a curated template. The
// numbers come from the design geometry: column width, body size, and the
// one-page target. Unknown ids get a conservative default so callers can
// always plan content.
func budgetFor(tpl Template) SpaceBudget {
	switch tpl.ID {
	case TemplateCompact:
		return SpaceBudget{TargetPages: 1, MaxSummaryLines: 2, MaxBulletsPerRole: 3, MaxRoles: 5, MaxSkills: 10, MaxEducation: 2, CharsPerLine: 100}
	case TemplateSidebar, TemplateSplit:
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 3, MaxBulletsPerRole: 4, MaxRoles: 4, MaxSkills: 10, MaxEducation: 2, CharsPerLine: 60}
	case TemplateDeveloper:
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 2, MaxBulletsPerRole: 3, MaxRoles: 4, MaxSkills: 14, MaxEducation: 2, CharsPerLine: 85}
	case TemplateMinimal:
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 2, MaxBulletsPerRole: 3, MaxRoles: 4, MaxSkills: 10, MaxEducation: 2, CharsPerLine: 90}
	case TemplateAcademic:
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 3, MaxBulletsPerRole: 4, MaxRoles: 5, MaxSkills: 12, MaxEducation: 3, CharsPerLine: 85}
	case TemplateExecutive:
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 3, MaxBulletsPerRole: 4, MaxRoles: 5, MaxSkills: 12, MaxEducation: 2, CharsPerLine: 90}
	case TemplateModern:
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 3, MaxBulletsPerRole: 4, MaxRoles: 5, MaxSkills: 12, MaxEducation: 2, CharsPerLine: 90}
	case TemplateBold:
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 3, MaxBulletsPerRole: 4, MaxRoles: 5, MaxSkills: 12, MaxEducation: 2, CharsPerLine: 85}
	case TemplateMonochrome:
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 3, MaxBulletsPerRole: 4, MaxRoles: 5, MaxSkills: 12, MaxEducation: 2, CharsPerLine: 90}
	case TemplateNordic:
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 3, MaxBulletsPerRole: 4, MaxRoles: 5, MaxSkills: 12, MaxEducation: 2, CharsPerLine: 100}
	default: // classic + unknown ids
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 3, MaxBulletsPerRole: 4, MaxRoles: 5, MaxSkills: 12, MaxEducation: 2, CharsPerLine: 95}
	}
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

// executiveTemplate is a formal, senior-leader look: serif type, muted steel
// accent, centered header with no rule.
func executiveTemplate() Template {
	return Template{
		ID:          TemplateExecutive,
		Name:        "Executive",
		Description: "Serif type with a muted steel accent — formal, senior-leader tone.",
		Layout:      LayoutSingle,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#475569",
		BodyFont:  "serif",
		ATSNote:   "ATS-safe — single column with standard section names.",
		Design: TemplateDesign{
			AccentRGB:   [3]int{71, 85, 105},
			HeaderAlign: "center",
			ShowRule:    false,
			LaTeXSize:   11,
			LaTeXMargin: "0.8in",
			NativeSize:  10,
			NativeName:  22,
			NativeLead:  5,
		},
	}
}

// minimalTemplate strips everything down: left header, no rule, soft slate
// accent and extra breathing room.
func minimalTemplate() Template {
	return Template{
		ID:          TemplateMinimal,
		Name:        "Minimal",
		Description: "Bare-bones single column with generous whitespace and a soft slate accent.",
		Layout:      LayoutSingle,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#94a3b8",
		ATSNote:   "ATS-safe — single column with standard section names.",
		Design: TemplateDesign{
			AccentRGB:   [3]int{148, 163, 184},
			HeaderAlign: "left",
			ShowRule:    false,
			LaTeXSize:   11,
			LaTeXMargin: "0.9in",
			NativeSize:  10,
			NativeName:  20,
			NativeLead:  5.5,
		},
	}
}

// academicTemplate leads with education and uses serif type with a deep navy
// accent — built for researchers, students and academia.
func academicTemplate() Template {
	return Template{
		ID:          TemplateAcademic,
		Name:        "Academic",
		Description: "Education-forward with serif type and a deep navy accent — built for academia.",
		Layout:      LayoutSingle,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionEducation, Label: "Education"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionSkills, Label: "Skills"},
		},
		AccentHex: "#1e3a8a",
		BodyFont:  "serif",
		ATSNote:   "ATS-safe — single column with standard section names.",
		Design: TemplateDesign{
			AccentRGB:   [3]int{30, 58, 138},
			HeaderAlign: "center",
			ShowRule:    true,
			LaTeXSize:   11,
			LaTeXMargin: "0.8in",
			NativeSize:  10,
			NativeName:  22,
			NativeLead:  5,
		},
	}
}

// developerTemplate is a monospace, terminal-flavoured design with a lime
// accent — right at home for the Nexus crowd.
func developerTemplate() Template {
	return Template{
		ID:          TemplateDeveloper,
		Name:        "Developer",
		Description: "Monospace type with a lime accent — a terminal-flavoured look.",
		Layout:      LayoutSingle,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#a3e635",
		BodyFont:  "mono",
		ATSNote:   "ATS-safe — single column with standard section names.",
		Design: TemplateDesign{
			AccentRGB:   [3]int{163, 230, 53},
			HeaderAlign: "left",
			ShowRule:    true,
			LaTeXSize:   10,
			LaTeXMargin: "0.7in",
			NativeSize:  9,
			NativeName:  20,
			NativeLead:  4.5,
		},
	}
}

// splitTemplate is a sidebar variant with the rail on the RIGHT — skills and
// education sit in a right rail while experience leads the main column.
func splitTemplate() Template {
	return Template{
		ID:          TemplateSplit,
		Name:        "Split",
		Description: "Two-column layout: skills and education in a right rail, experience leads on the left.",
		Layout:      LayoutSidebar,
		RailSide:    "right",
		Sections: []TemplateSection{
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#f59e0b",
		ATSNote:   "Design-forward — two columns can confuse some ATS systems; use for roles where design matters.",
		Design: TemplateDesign{
			AccentRGB:   [3]int{245, 158, 11},
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

// boldTemplate makes the name the hero: large centered header, magenta accent
// and a strong rule. Eye-catching but still single column.
func boldTemplate() Template {
	return Template{
		ID:          TemplateBold,
		Name:        "Bold",
		Description: "Big centered header with a magenta accent — makes your name the hero.",
		Layout:      LayoutSingle,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#ec4899",
		ATSNote:   "ATS-safe — single column with standard section names.",
		Design: TemplateDesign{
			AccentRGB:   [3]int{236, 72, 153},
			HeaderAlign: "center",
			ShowRule:    true,
			LaTeXSize:   12,
			LaTeXMargin: "0.7in",
			NativeSize:  10,
			NativeName:  28,
			NativeLead:  5,
		},
	}
}

// monochromeTemplate is all-ink: black accent, serif type, no rule — quiet,
// classic and universally acceptable.
func monochromeTemplate() Template {
	return Template{
		ID:          TemplateMonochrome,
		Name:        "Monochrome",
		Description: "All-ink serif with a black accent and no rule — quiet, classic, universal.",
		Layout:      LayoutSingle,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#111827",
		BodyFont:  "serif",
		ATSNote:   "ATS-safe — single column with standard section names.",
		Design: TemplateDesign{
			AccentRGB:   [3]int{17, 24, 39},
			HeaderAlign: "left",
			ShowRule:    false,
			LaTeXSize:   11,
			LaTeXMargin: "0.85in",
			NativeSize:  10,
			NativeName:  22,
			NativeLead:  5,
		},
	}
}

// nordicTemplate is a clean scandi look: teal accent, left header, thin rule
// and modest margins.
func nordicTemplate() Template {
	return Template{
		ID:          TemplateNordic,
		Name:        "Nordic",
		Description: "Clean scandi look — teal accent, left header and a thin rule.",
		Layout:      LayoutSingle,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#0d9488",
		ATSNote:   "ATS-safe — single column with standard section names.",
		Design: TemplateDesign{
			AccentRGB:   [3]int{13, 148, 136},
			HeaderAlign: "left",
			ShowRule:    true,
			LaTeXSize:   10,
			LaTeXMargin: "0.6in",
			NativeSize:  9,
			NativeName:  21,
			NativeLead:  4.5,
		},
	}
}
