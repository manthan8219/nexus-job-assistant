package workcontext

import (
	"strings"
	"time"
)

// Project is one repo / body of work the user wants Nexus to remember.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Repo      string    `json:"repo,omitempty"`
	Period    string    `json:"period,omitempty"` // e.g. "2024 – 2025" or "Jan 2025 – Present"
	Role      string    `json:"role,omitempty"`   // e.g. Backend Engineer
	Summary   string    `json:"summary"`          // Claude / freeform context paste
	Bullets   []string  `json:"bullets,omitempty"`
	Stack     []string  `json:"stack,omitempty"`
	Skills    []string  `json:"skills,omitempty"`
	Source    string    `json:"source,omitempty"` // "claude" | "manual"
	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
}

// StoreFile is the on-disk document.
type StoreFile struct {
	Projects []Project `json:"projects"`
}

// ExtractBullets pulls markdown-ish bullets from a Claude paste.
func ExtractBullets(summary string) []string {
	var out []string
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "- "):
			line = strings.TrimSpace(line[2:])
		case strings.HasPrefix(line, "* "):
			line = strings.TrimSpace(line[2:])
		case strings.HasPrefix(line, "• "):
			line = strings.TrimSpace(line[len("• "):])
		default:
			continue
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// ShortSummary returns a one-line preview.
func (p Project) ShortSummary(max int) string {
	s := strings.Join(strings.Fields(p.Summary), " ")
	if s == "" && len(p.Bullets) > 0 {
		s = p.Bullets[0]
	}
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
