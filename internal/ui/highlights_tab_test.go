package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/inbox"
)

func TestHighlightsTabLoadAndNavigate(t *testing.T) {
	m := NewHighlightsTabModel()
	hs := []inbox.Highlight{
		{ID: "1", From: "a@acme.com", Subject: "Interview", Date: time.Now(), Signal: inbox.SignalInterview},
		{ID: "2", From: "b@acme.com", Subject: "Offer", Date: time.Now(), Signal: inbox.SignalOffer},
		{ID: "3", From: "c@acme.com", Subject: "Rejection", Date: time.Now(), Signal: inbox.SignalRejection},
	}

	// Load.
	model, cmd := m.Update(highlightsLoadedMsg{hs: hs})
	m = model.(HighlightsTabModel)
	if len(m.highlights) != 3 {
		t.Fatalf("highlights = %d; want 3", len(m.highlights))
	}
	if m.statusLine == "" {
		t.Error("expected a status line after load")
	}
	if cmd != nil {
		t.Error("load msg should not produce a command")
	}

	// Navigate down.
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = model.(HighlightsTabModel)
	if m.cursor != 1 {
		t.Errorf("cursor = %d; want 1 after down", m.cursor)
	}

	// Refresh returns a command.
	model, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = model.(HighlightsTabModel)
	if cmd == nil {
		t.Error("refresh key should return a load command")
	}
	if m.cursor != 1 {
		t.Errorf("cursor = %d; want unchanged after refresh", m.cursor)
	}
}

func TestHighlightsTabLoadError(t *testing.T) {
	m := NewHighlightsTabModel()
	model, _ := m.Update(highlightsLoadedMsg{err: tea.ErrProgramKilled})
	m = model.(HighlightsTabModel)
	if m.errLine == "" {
		t.Error("expected an error line on load failure")
	}
}
