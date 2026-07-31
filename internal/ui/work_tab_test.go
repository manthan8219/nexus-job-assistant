package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/workcontext"
)

// TestWorkTabViewList_CompactRows verifies each saved project renders as a
// single truncated row (name · meta · N bullets) so the list stays a few
// lines tall no matter how many projects exist.
func TestWorkTabViewList_CompactRows(t *testing.T) {
	mk := func(name string, bullets int) workcontext.Project {
		p := workcontext.Project{Name: name, Role: "Backend Engineer", Period: "2024 – Present", Repo: "github.com/org/repo"}
		for i := 0; i < bullets; i++ {
			p.Bullets = append(p.Bullets, "built thing")
		}
		return p
	}

	tests := []struct {
		name      string
		width     int
		projects  []workcontext.Project
		cursor    int
		wantLines int // non-empty lines in the rendered list body
	}{
		{
			name:      "three projects render one row each",
			width:     100,
			projects:  []workcontext.Project{mk("Payments API", 0), mk("Auth Service", 4), mk("Jobs Pipeline", 2)},
			cursor:    0,
			wantLines: 5, // "N projects" header + 3 rows + hint line
		},
		{
			name:      "single project",
			width:     80,
			projects:  []workcontext.Project{mk("Only Project", 0)},
			cursor:    0,
			wantLines: 3,
		},
		{
			name:      "long names and meta stay inside terminal width",
			width:     80,
			projects:  []workcontext.Project{mk(strings.Repeat("A", 80), 9), mk(strings.Repeat("B", 90), 9)},
			cursor:    1,
			wantLines: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := WorkTabModel{width: tt.width, mode: workList, projects: tt.projects, cursor: tt.cursor}
			got := m.viewList()

			var lines []string
			for _, ln := range strings.Split(got, "\n") {
				if strings.TrimSpace(ln) != "" {
					lines = append(lines, ln)
				}
			}
			if len(lines) != tt.wantLines {
				t.Errorf("viewList() produced %d non-empty lines; want %d\n%s", len(lines), tt.wantLines, got)
			}
			for i, ln := range lines {
				if w := lipgloss.Width(ln); w > tt.width {
					t.Errorf("line %d (%q) width %d > terminal width %d", i, ln, w, tt.width)
				}
			}
		})
	}
}

// TestWorkTabViewList_RowContent verifies the compact row composition: active
// ▶ marker, meta joined with " · ", and the bullet count.
func TestWorkTabViewList_RowContent(t *testing.T) {
	m := WorkTabModel{
		width:  100,
		mode:   workList,
		cursor: 0,
		projects: []workcontext.Project{
			{Name: "Payments API", Role: "Backend Engineer", Period: "2024 – Present", Bullets: []string{"a", "b"}},
			{Name: "Auth Service", Bullets: []string{"x"}},
		},
	}
	got := m.viewList()

	if !strings.Contains(got, "▶ Payments API") {
		t.Errorf("viewList() active row missing ▶ marker; got:\n%s", got)
	}
	if !strings.Contains(got, "2 bullets") {
		t.Errorf("viewList() missing bullet count; got:\n%s", got)
	}
	if !strings.Contains(got, "Backend Engineer · 2024 – Present") {
		t.Errorf("viewList() missing meta; got:\n%s", got)
	}
	// No project name may appear on a line that is otherwise blank (each
	// project must occupy exactly one row).
	for _, proj := range m.projects {
		for _, ln := range strings.Split(got, "\n") {
			if strings.TrimSpace(ln) == "" && strings.Contains(ln, proj.Name) {
				t.Errorf("project %q sits on an otherwise-empty line", proj.Name)
			}
		}
	}
}
