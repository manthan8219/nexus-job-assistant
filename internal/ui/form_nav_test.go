package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

func pressKey(m FormModel, key string) FormModel {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		msg = tea.KeyMsg{Type: tea.KeyShiftTab}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case " ":
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	}
	updated, _ := m.Update(msg)
	fm, _ := updated.(FormModel)
	return fm
}

func newTestForm() FormModel {
	cfg := &config.Config{
		FirstName:  "John",
		LastName:   "Doe",
		Email:      "john@example.com",
		Phone:      "1234567890",
		LinkedInID: "johndoe",
	}
	m := NewFormModel(cfg, true) // skipResumeCheck=true so we can nav freely
	m.focused = fFirstName
	return m
}

func TestDownArrow_MovesForward(t *testing.T) {
	m := newTestForm()
	if m.focused != fFirstName {
		t.Fatalf("expected fFirstName, got %d", m.focused)
	}
	m = pressKey(m, "down")
	if m.focused != fLastName {
		t.Errorf("down from fFirstName: want fLastName(%d), got %d", fLastName, m.focused)
	}
}

func TestUpArrow_MovesBackward(t *testing.T) {
	m := newTestForm()
	m.focused = fLastName
	m = pressKey(m, "up")
	if m.focused != fFirstName {
		t.Errorf("up from fLastName: want fFirstName(%d), got %d", fFirstName, m.focused)
	}
}

func TestDownArrow_TraversesAllFields(t *testing.T) {
	m := newTestForm()
	m.focused = fFirstName
	for i := 0; i < fieldCount-1; i++ {
		prev := m.focused
		m = pressKey(m, "down")
		if m.focused == prev {
			t.Errorf("stuck at field %d after pressing down", prev)
		}
	}
}

func TestUpArrow_TraversesAllFieldsBackward(t *testing.T) {
	m := newTestForm()
	m.focused = fIndeedKey // start at last
	for i := 0; i < fieldCount-1; i++ {
		prev := m.focused
		m = pressKey(m, "up")
		if m.focused == prev {
			t.Errorf("stuck at field %d after pressing up", prev)
		}
	}
}

func TestDown_LockedFieldBounces_WhenResumeInvalid(t *testing.T) {
	m := newTestForm()
	m.skipResumeCheck = false
	m.resumeAnalysisDone = true
	m.resumeAnalysisResult.Valid = false
	// User somehow got to fCity (a locked field); pressing down should bounce to fResumePath.
	m.focused = fCity

	m = pressKey(m, "down")
	// fYearsExp is also locked → should bounce to fResumePath
	if m.focused != fResumePath {
		t.Errorf("expected bounce to fResumePath, got %d", m.focused)
	}
}

func TestTabAndDownBothAdvance(t *testing.T) {
	m1 := newTestForm()
	m2 := newTestForm()
	m1 = pressKey(m1, "tab")
	m2 = pressKey(m2, "down")
	if m1.focused != m2.focused {
		t.Errorf("tab and down diverged: tab→%d down→%d", m1.focused, m2.focused)
	}
}

func TestShiftTabAndUpBothGoBack(t *testing.T) {
	m1 := newTestForm()
	m2 := newTestForm()
	m1.focused = fEmail
	m2.focused = fEmail
	m1 = pressKey(m1, "shift+tab")
	m2 = pressKey(m2, "up")
	if m1.focused != m2.focused {
		t.Errorf("shift+tab and up diverged: shift+tab→%d up→%d", m1.focused, m2.focused)
	}
}

func TestCustomFieldActive_IncludesNotifyChannels(t *testing.T) {
	m := newTestForm()
	m.focused = fNotifyChannels
	if !m.CustomFieldActive() {
		t.Fatal("CustomFieldActive should be true on fNotifyChannels so ←→ are not stolen by app tabs")
	}
}

func TestNotifyChannels_LeftRightMovesCursor(t *testing.T) {
	m := newTestForm()
	m.focused = fNotifyChannels
	m.ncCursor = 0
	if len(m.ncSelected) < 2 {
		t.Fatalf("expected at least 2 channels from registry, got %d", len(m.ncSelected))
	}
	m = pressKey(m, "right")
	if m.ncCursor != 1 {
		t.Errorf("right from Discord: want cursor 1 (Telegram), got %d", m.ncCursor)
	}
	m = pressKey(m, "left")
	if m.ncCursor != 0 {
		t.Errorf("left from Telegram: want cursor 0 (Discord), got %d", m.ncCursor)
	}
}

func TestNotifyChannels_TabAdvancesPastWidget(t *testing.T) {
	m := newTestForm()
	m.focused = fTelegramChatID
	m = pressKey(m, "tab")
	if m.focused != fNotifyChannels {
		t.Fatalf("tab from chat ID: want fNotifyChannels(%d), got %d", fNotifyChannels, m.focused)
	}
	m = pressKey(m, "tab")
	if m.focused != fApplyConsent {
		t.Errorf("tab from notify channels: want fApplyConsent(%d), got %d", fApplyConsent, m.focused)
	}
	// Cover letter text is hidden (mode=off), so next visible field is fScraperTargets.
	m.focused = fCoverLetterMode
	m = pressKey(m, "tab")
	if m.focused != fScraperTargets {
		t.Errorf("tab from fCoverLetterMode: want fScraperTargets(%d), got %d", fScraperTargets, m.focused)
	}
	// Tab from the last field (fScraperTargets) wraps back to first.
	m = pressKey(m, "tab")
	if m.focused != fFirstName {
		t.Errorf("tab from fScraperTargets: want wrap to fFirstName(%d), got %d", fFirstName, m.focused)
	}
}

func TestNotifyChannels_SpaceTogglesSelection(t *testing.T) {
	m := newTestForm()
	m.focused = fNotifyChannels
	m.ncCursor = 1 // telegram
	before := m.ncSelected[1]
	m = pressKey(m, " ")
	if m.ncSelected[1] == before {
		t.Fatal("space should toggle Telegram selection")
	}
}

func TestLocalLLMPicker_UpAtTopLeaves(t *testing.T) {
	m := newTestForm()
	m.aiAssist = true
	m.aiProvider = "local"
	m.llmOffline = false
	m.llmOptions = nil // empty list — up should leave
	m.focused = fLocalLLMModel
	m = pressKey(m, "up")
	if m.focused == fLocalLLMModel {
		t.Fatal("up on empty/top picker should leave Local Model field")
	}
}

func TestLocalLLMPicker_TabLeaves(t *testing.T) {
	m := newTestForm()
	m.aiAssist = true
	m.aiProvider = "local"
	m.focused = fLocalLLMModel
	m = pressKey(m, "tab")
	if m.focused == fLocalLLMModel {
		t.Fatal("tab should leave Local Model field")
	}
}

func TestLocalLLMSetup_UpAtTopLeaves(t *testing.T) {
	m := newTestForm()
	m.aiAssist = true
	m.aiProvider = "local"
	m.llmOffline = true
	m.llmSetupCursor = 0
	m.focused = fLocalLLMModel
	m = pressKey(m, "up")
	if m.focused == fLocalLLMModel {
		t.Fatal("up on first setup item should leave field")
	}
}

func TestCustomFieldActive_JobTitlesKeepsArrows(t *testing.T) {
	m := NewFormModel(&config.Config{}, true)
	m.focused = fJobTitles
	if !m.CustomFieldActive() {
		t.Fatal("Job Titles must keep ←→ for cursor (not stolen by app tabs)")
	}
	m.focused = fLocations
	if !m.CustomFieldActive() {
		t.Fatal("Locations must keep ←→ for cursor")
	}
}
