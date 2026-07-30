package ui

// Package ui — form_locations.go
// Target Locations tag field: city autocomplete, tag add/remove, and the
// handleLocationsKey key handler. Returns ok=false for navigation keys so they
// fall through to the shared field navigation.

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/geo"
)

// handleLocationsKey handles the Target Locations tag field with city autocomplete.
// Returns ok=false for navigation keys so they fall through to field navigation.
func (m FormModel) handleLocationsKey(key string) (FormModel, tea.Cmd, bool) {
	// Autocomplete navigation for city picker
	if len(m.acSuggestions) > 0 {
		switch key {
		case "down", "ctrl+n":
			if m.acIdx < len(m.acSuggestions)-1 {
				m.acIdx++
			}
			return m, nil, true
		case "up", "ctrl+p":
			if m.acIdx > 0 {
				m.acIdx--
			}
			return m, nil, true
		case "esc":
			m.acSuggestions = nil
			m.acIdx = -1
			return m, nil, true
		case "tab", "enter":
			idx := m.acIdx
			if idx < 0 {
				idx = 0
			}
			sel := m.acSuggestions[idx]
			if m.addLocationTag(sel) {
				m.inputs[fLocations].SetValue("")
				m.acSuggestions = nil
				m.acIdx = -1
				return m, m.saveCmd(), true
			}
			return m, nil, true
		}
	}
	switch key {
	case "enter", "tab":
		val := strings.TrimSpace(m.inputs[fLocations].Value())
		if val == "" {
			return m, nil, false // fall through to field nav
		}
		if m.addLocationTag(val) {
			m.inputs[fLocations].SetValue("")
			m.acSuggestions = nil
			m.acIdx = -1
			return m, m.saveCmd(), true
		}
		// Not in index — keep typing; refresh suggestions
		m.updateLocationAC(val)
		return m, nil, true
	case "backspace":
		if m.inputs[fLocations].Value() == "" && len(m.locationTags) > 0 {
			m.locationTags = m.locationTags[:len(m.locationTags)-1]
			return m, m.saveCmd(), true
		}
	}
	return m, nil, false
}

// updateLocationAC refreshes city suggestions for Target Locations.
func (m *FormModel) updateLocationAC(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		m.acSuggestions = nil
		m.acIdx = -1
		return
	}
	hits := geo.Search(input, 8)
	suggestions := make([]string, 0, len(hits))
	for _, c := range hits {
		suggestions = append(suggestions, c.Display())
	}
	m.acSuggestions = suggestions
	m.acIdx = -1
}

// addLocationTag resolves input against the geo index and appends a unique tag.
// Returns false if the city is not in the index.
func (m *FormModel) addLocationTag(raw string) bool {
	c, ok := geo.Resolve(raw)
	if !ok {
		return false
	}
	tag := c.Display()
	for _, existing := range m.locationTags {
		if strings.EqualFold(existing, tag) {
			return true // already present — treat as success
		}
	}
	m.locationTags = append(m.locationTags, tag)
	return true
}

// parseTags splits a comma-separated string into trimmed, non-empty tags.
func parseTags(s string) []string {
	var tags []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			tags = append(tags, part)
		}
	}
	return tags
}
