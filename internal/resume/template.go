package resume

import (
	"fmt"
	"strings"
)

// Template ids for the curated registry. The classic template is the default
// and the fallback when a caller passes an empty id (old clients keep working).
const (
	TemplateClassic   = "classic" // legacy alias → jake
	TemplateJake      = "jake"
	TemplateAwesomeCV = "awesome-cv"
	TemplateDeedy     = "deedy"
	TemplateMcDowell  = "mcdowell"
	TemplateBillRyan  = "billryan"
	TemplateKendall   = "kendall"
	TemplateMacchiato = "macchiato"
	TemplateBanking   = "banking"
	TemplateModern    = "modern"
	TemplateAcademic  = "academic"
)

// SectionStyle tokens: how a template draws its section headings.
const (
	SectionStylePlain     = "plain"
	SectionStyleCaps      = "caps"
	SectionStyleMarker    = "marker"
	SectionStyleRuleAbove = "ruleAbove"
	SectionStyleSoft      = "soft"
	SectionStyleRuleBelow = "ruleBelow"
	SectionStyleNumbered  = "numbered"
)

// NameStyle tokens: how the header block presents the candidate's name.
const (
	NameStylePlain    = "plain"
	NameStyleCentered = "centered"
	NameStyleColored  = "colored"
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
	AccentRGB    [3]int  // rgb triple for rules, section titles, header accents
	TitleRGB     [3]int  // rgb triple for section titles + name; zero = accent
	HeaderAlign  string  // "left" | "center"
	ShowRule     bool    // draw an accent rule under the header block
	LaTeXSize    int     // body font size (pt) for the LaTeX renderer
	LaTeXMargin  string  // geometry margin for the LaTeX renderer (e.g. "0.7in")
	NativeSize   int     // native-PDF body font size
	NativeName   int     // native-PDF name font size
	NativeLead   float64 // native-PDF line leading
	NativeMargin float64 // native-PDF side margin (mm)
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
	ShowRule bool        `json:"showRule"` // accent rule under the header
	Budget   SpaceBudget `json:"budget,omitempty"`
	// SectionStyle / NameStyle / ContactLine / ColumnRatio / RailBackground are
	// the "real design" tokens the renderers (LaTeX + native PDF) honor so each
	// curated template looks like the famous design it is adapted from.
	SectionStyle   string         `json:"sectionStyle,omitempty"`   // plain | caps | marker | ruleAbove | soft
	NameStyle      string         `json:"nameStyle,omitempty"`      // plain | centered | colored
	ContactLine    bool           `json:"contactLine"`              // email · phone · location under the header
	ColumnRatio    float64        `json:"columnRatio,omitempty"`    // main-column fraction for sidebar layouts
	RailBackground string         `json:"railBackground,omitempty"` // dark | accent | tint sidebar rail
	Source         string         `json:"source,omitempty"`         // the open-source design this adapts
	Design         TemplateDesign `json:"-"`
}

// TemplateIDs returns the ordered registry ids (used by the API docs/tests).
func TemplateIDs() []string {
	return []string{
		TemplateJake, TemplateAwesomeCV, TemplateDeedy, TemplateMcDowell,
		TemplateBillRyan, TemplateKendall, TemplateMacchiato, TemplateBanking,
		TemplateModern, TemplateAcademic,
	}
}

// Templates returns the curated registry of real, widely-used resume designs.
func Templates() []Template {
	templates := []Template{
		jakeTemplate(),
		awesomeCVTemplate(),
		deedyTemplate(),
		mcdowellTemplate(),
		billRyanTemplate(),
		kendallTemplate(),
		macchiatoTemplate(),
		bankingTemplate(),
		modernTemplate(),
		academicTemplate(),
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

// GetTemplate resolves a template by id. An empty id resolves to Jake (the
// default) so existing callers (TUI, tailor) keep working unchanged. The old
// "classic" id is kept as a legacy alias for stored versions/links.
func GetTemplate(id string) (Template, error) {
	if strings.TrimSpace(id) == "" {
		id = TemplateJake
	}
	if id == TemplateClassic {
		id = TemplateJake
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

// budgetFor returns the explicit space budget for a curated template. The
// numbers come from the design geometry: column width, body size, and the
// one-page target. Unknown ids get a conservative default so callers can
// always plan content.
func budgetFor(tpl Template) SpaceBudget {
	switch tpl.ID {
	case TemplateDeedy: // one-page asymmetric two-column
		return SpaceBudget{TargetPages: 1, MaxSummaryLines: 2, MaxBulletsPerRole: 3, MaxRoles: 4, MaxSkills: 10, MaxEducation: 2, CharsPerLine: 75}
	case TemplateKendall, TemplateMacchiato: // sidebar rail
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 3, MaxBulletsPerRole: 4, MaxRoles: 4, MaxSkills: 10, MaxEducation: 2, CharsPerLine: 60}
	case TemplateAwesomeCV: // marker style, generous skills
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 3, MaxBulletsPerRole: 4, MaxRoles: 5, MaxSkills: 14, MaxEducation: 2, CharsPerLine: 95}
	case TemplateBanking: // ruled headings, serif
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 3, MaxBulletsPerRole: 4, MaxRoles: 5, MaxSkills: 12, MaxEducation: 2, CharsPerLine: 90}
	case TemplateMcDowell: // generous whitespace
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 3, MaxBulletsPerRole: 4, MaxRoles: 5, MaxSkills: 12, MaxEducation: 2, CharsPerLine: 95}
	default: // jake, billryan + unknown ids
		return SpaceBudget{TargetPages: 0, MaxSummaryLines: 3, MaxBulletsPerRole: 4, MaxRoles: 5, MaxSkills: 14, MaxEducation: 2, CharsPerLine: 100}
	}
}

// classicTemplate comment removed in the real-templates registry.

// jakeTemplate is the default: the recruiter-favorite clean single column from
// jakegut/resume — small-caps section heads, tight spacing, zero gimmicks.
func jakeTemplate() Template {
	return Template{
		ID:           TemplateJake,
		Name:         "Jake",
		Description:  "The recruiter-favorite clean single column — small-caps section heads, tight spacing, zero gimmicks. Adapted from jakegut/resume.",
		Layout:       LayoutSingle,
		SectionStyle: SectionStyleCaps,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#334155",
		ATSNote:   "ATS-perfect — the most widely recommended clean LaTeX template.",
		Source:    "github.com/jakegut/resume (MIT)",
		Design: TemplateDesign{
			AccentRGB:    [3]int{51, 65, 85},
			HeaderAlign:  "left",
			ShowRule:     false,
			LaTeXSize:    10,
			LaTeXMargin:  "0.6in",
			NativeSize:   10,
			NativeName:   22,
			NativeLead:   4.5,
			NativeMargin: 15,
		},
	}
}

// awesomeCVTemplate adds a filled accent marker before every section heading —
// the signature of posquit0/Awesome-CV, the most-starred LaTeX CV on GitHub.
func awesomeCVTemplate() Template {
	return Template{
		ID:           TemplateAwesomeCV,
		Name:         "Awesome-CV",
		Description:  "Professional sections with a filled accent marker and a full-width rule. Adapted from posquit0/Awesome-CV.",
		Layout:       LayoutSingle,
		SectionStyle: SectionStyleMarker,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
			{Key: SectionSkills, Label: "Skills"},
		},
		AccentHex: "#00539b",
		ATSNote:   "ATS-safe — single column with standard section names.",
		Source:    "github.com/posquit0/Awesome-CV (LPPL)",
		Design: TemplateDesign{
			AccentRGB:    [3]int{0, 83, 155},
			HeaderAlign:  "left",
			ShowRule:     false,
			LaTeXSize:    10,
			LaTeXMargin:  "0.65in",
			NativeSize:   10,
			NativeName:   22,
			NativeLead:   4.5,
			NativeMargin: 15,
		},
	}
}

// deedyTemplate is the famous one-page asymmetric two-column resume — a narrow
// left rail (dates/skills) and a wide experience column, thin rule under the
// name. Adapted from deedy/Deedy-Resume.
func deedyTemplate() Template {
	return Template{
		ID:           TemplateDeedy,
		Name:         "Deedy",
		Description:  "One-page asymmetric two-column — dates and skills in a narrow rail, experience wide. Adapted from deedy/Deedy-Resume.",
		Layout:       LayoutSidebar,
		RailSide:     "left",
		ColumnRatio:  0.76,
		OnePage:      true,
		SectionStyle: SectionStylePlain,
		Sections: []TemplateSection{
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
			{Key: SectionSkills, Label: "Skills"},
		},
		AccentHex: "#111827",
		ATSNote:   "One-page asymmetric two-column — great for new grads; two columns can trip some ATS systems.",
		Source:    "github.com/deedy/Deedy-Resume (Apache-2.0)",
		Design: TemplateDesign{
			AccentRGB:    [3]int{17, 24, 39},
			HeaderAlign:  "left",
			ShowRule:     true,
			LaTeXSize:    10,
			LaTeXMargin:  "0.5in",
			NativeSize:   9,
			NativeName:   21,
			NativeLead:   4.5,
			NativeMargin: 13,
		},
	}
}

// mcdowellTemplate is a clean single column with generous whitespace and soft
// gray section heads — the polished look of dnl-blkv/mcdowell-cv.
func mcdowellTemplate() Template {
	return Template{
		ID:           TemplateMcDowell,
		Name:         "McDowell",
		Description:  "Clean single column with generous whitespace and soft gray section heads. Adapted from dnl-blkv/mcdowell-cv.",
		Layout:       LayoutSingle,
		SectionStyle: SectionStyleSoft,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#6b7280",
		ATSNote:   "ATS-safe — single column with standard section names.",
		Source:    "github.com/dnl-blkv/mcdowell-cv (MIT)",
		Design: TemplateDesign{
			AccentRGB:    [3]int{107, 114, 128},
			HeaderAlign:  "left",
			ShowRule:     false,
			LaTeXSize:    10,
			LaTeXMargin:  "0.85in",
			NativeSize:   10,
			NativeName:   22,
			NativeLead:   5.5,
			NativeMargin: 18,
		},
	}
}

// billRyanTemplate is the elegant minimal single column with a serif body —
// billryan/resume's look, clean and quietly professional.
func billRyanTemplate() Template {
	return Template{
		ID:           TemplateBillRyan,
		Name:         "BillRyan",
		Description:  "Elegant minimal single column with a serif body. Adapted from billryan/resume.",
		Layout:       LayoutSingle,
		BodyFont:     "serif",
		SectionStyle: SectionStylePlain,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#0f172a",
		ATSNote:   "ATS-safe — single column with standard section names.",
		Source:    "github.com/billryan/resume (MIT)",
		Design: TemplateDesign{
			AccentRGB:    [3]int{15, 23, 42},
			HeaderAlign:  "left",
			ShowRule:     true,
			LaTeXSize:    11,
			LaTeXMargin:  "0.75in",
			NativeSize:   10,
			NativeName:   22,
			NativeLead:   5,
			NativeMargin: 16,
		},
	}
}

// kendallTemplate puts skills + education in a dark sidebar rail, main column
// light — the signature of the JSON Resume Kendall theme.
func kendallTemplate() Template {
	return Template{
		ID:             TemplateKendall,
		Name:           "Kendall",
		Description:    "Two-column with a dark sidebar rail for skills and education. Adapted from the JSON Resume Kendall theme.",
		Layout:         LayoutSidebar,
		RailSide:       "left",
		RailBackground: "dark",
		SectionStyle:   SectionStylePlain,
		Sections: []TemplateSection{
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#111827",
		ATSNote:   "Two columns can confuse some ATS systems; use for design-forward roles.",
		Source:    "jsonresume.org — Kendall (MIT)",
		Design: TemplateDesign{
			AccentRGB:    [3]int{17, 24, 39},
			HeaderAlign:  "left",
			ShowRule:     false,
			LaTeXSize:    10,
			LaTeXMargin:  "0.7in",
			NativeSize:   10,
			NativeName:   22,
			NativeLead:   4.5,
			NativeMargin: 15,
		},
	}
}

// macchiatoTemplate is the two-column with an accent-colored sidebar and an
// accent-colored name — the JSON Resume Macchiato theme.
func macchiatoTemplate() Template {
	return Template{
		ID:             TemplateMacchiato,
		Name:           "Macchiato",
		Description:    "Two-column with an accent-colored sidebar and accent name. Adapted from the JSON Resume Macchiato theme.",
		Layout:         LayoutSidebar,
		RailSide:       "left",
		RailBackground: "accent",
		NameStyle:      NameStyleColored,
		SectionStyle:   SectionStylePlain,
		Sections: []TemplateSection{
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#0f766e",
		ATSNote:   "Two columns can confuse some ATS systems; use for design-forward roles.",
		Source:    "jsonresume.org — Macchiato (MIT)",
		Design: TemplateDesign{
			AccentRGB:    [3]int{15, 118, 110},
			HeaderAlign:  "left",
			ShowRule:     false,
			LaTeXSize:    10,
			LaTeXMargin:  "0.7in",
			NativeSize:   10,
			NativeName:   22,
			NativeLead:   4.5,
			NativeMargin: 15,
		},
	}
}

// bankingTemplate is the classic moderncv banking style: centered name, a
// contact line underneath, and a horizontal rule above every section heading.
func bankingTemplate() Template {
	return Template{
		ID:           TemplateBanking,
		Name:         "Banking",
		Description:  "Centered name with a contact line and ruled section heads — the classic moderncv banking style.",
		Layout:       LayoutSingle,
		BodyFont:     "serif",
		NameStyle:    NameStyleCentered,
		ContactLine:  true,
		SectionStyle: SectionStyleRuleAbove,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Summary"},
			{Key: SectionExperience, Label: "Experience"},
			{Key: SectionSkills, Label: "Skills"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#004d99",
		ATSNote:   "ATS-safe — single column with standard section names.",
		Source:    "moderncv — banking style (LPPL)",
		Design: TemplateDesign{
			AccentRGB:    [3]int{0, 77, 153},
			HeaderAlign:  "center",
			ShowRule:     false,
			LaTeXSize:    11,
			LaTeXMargin:  "0.75in",
			NativeSize:   10,
			NativeName:   24,
			NativeLead:   5,
			NativeMargin: 16,
		},
	}
}

// modernTemplate is the classic modern one-column design: centered header,
// serif body, and accent-ruled section heads (rule drawn below the title).
// Adapted from the widely-shared "Modern Software Engineer Resume" template.
func modernTemplate() Template {
	return Template{
		ID:           TemplateModern,
		Name:         "Modern",
		Description:  "Centered name with a serif body and ruled section heads — the classic modern software-engineer look.",
		Layout:       LayoutSingle,
		BodyFont:     "serif",
		NameStyle:    NameStyleColored,
		ContactLine:  true,
		SectionStyle: SectionStyleRuleBelow,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Professional Summary"},
			{Key: SectionExperience, Label: "Professional Experience"},
			{Key: SectionSkills, Label: "Technical Skills"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#3498DB",
		ATSNote:   "ATS-safe — single column with standard section names.",
		Source:    "Community modern LaTeX resume template",
		Design: TemplateDesign{
			AccentRGB:    [3]int{52, 152, 219}, // accent blue: rules + contact accents
			TitleRGB:     [3]int{45, 62, 80},   // primary slate: name + section titles
			HeaderAlign:  "center",
			ShowRule:     false,
			LaTeXSize:    11,
			LaTeXMargin:  "0.6in",
			NativeSize:   11,
			NativeName:   26,
			NativeLead:   5,
			NativeMargin: 15,
		},
	}
}

// academicTemplate is the classic academic CV: centered header, numbered
// small-caps section heads, and a formal serif body. Long-form academic
// sections (publications, grants, teaching, service) exceed the built-in
// content model, so only the representable sections are exposed.
func academicTemplate() Template {
	return Template{
		ID:           TemplateAcademic,
		Name:         "Academic",
		Description:  "Centered header with numbered small-caps section heads and a formal serif body — the classic academic research CV.",
		Layout:       LayoutSingle,
		BodyFont:     "serif",
		NameStyle:    NameStyleCentered,
		SectionStyle: SectionStyleNumbered,
		Sections: []TemplateSection{
			{Key: SectionSummary, Label: "Research Interests"},
			{Key: SectionExperience, Label: "Academic Appointments"},
			{Key: SectionEducation, Label: "Education"},
		},
		AccentHex: "#111827",
		ATSNote:   "Single column with standard section names — ATS-safe.",
		Source:    "Community academic CV LaTeX template",
		Design: TemplateDesign{
			AccentRGB:    [3]int{17, 24, 39},
			HeaderAlign:  "center",
			ShowRule:     false,
			LaTeXSize:    11,
			LaTeXMargin:  "1in",
			NativeSize:   11,
			NativeName:   24,
			NativeLead:   5,
			NativeMargin: 25,
		},
	}
}
