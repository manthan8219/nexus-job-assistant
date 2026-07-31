package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func subNavKey(key string) tea.KeyMsg {
	switch key {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

// pressAppKey routes a key through the app-level dispatcher (handleAppKeyMsg),
// which is where tab/shift+tab routing between main tabs and sub-menus lives.
func pressAppKey(t *testing.T, m AppModel, key string) AppModel {
	t.Helper()
	updated, _ := m.handleAppKeyMsg(subNavKey(key))
	app, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("handleAppKeyMsg(%q) returned %T, want AppModel", key, updated)
	}
	return app
}

func TestOutreachTabKeyCyclesSubSections(t *testing.T) {
	m := AppModel{activeTab: TabOutreach, outreach: NewOutreachHubModel(), profileComplete: true}
	if m.outreach.sub != outreachSubSetup {
		t.Fatalf("start: sub = %d, want Setup(%d)", m.outreach.sub, outreachSubSetup)
	}

	m = pressAppKey(t, m, "tab")
	if m.outreach.sub != outreachSubEmail {
		t.Errorf("tab: sub = %d, want Email(%d)", m.outreach.sub, outreachSubEmail)
	}
	if m.activeTab != TabOutreach {
		t.Errorf("tab must not switch the main tab, activeTab = %d", m.activeTab)
	}

	m = pressAppKey(t, m, "tab")
	if m.outreach.sub != outreachSubLinkedIn {
		t.Errorf("tab: sub = %d, want LinkedIn(%d)", m.outreach.sub, outreachSubLinkedIn)
	}

	m = pressAppKey(t, m, "shift+tab")
	if m.outreach.sub != outreachSubEmail {
		t.Errorf("shift+tab: sub = %d, want Email(%d)", m.outreach.sub, outreachSubEmail)
	}
}

func TestOutreachTabKeyWrapsAround(t *testing.T) {
	m := AppModel{activeTab: TabOutreach, outreach: NewOutreachHubModel(), profileComplete: true}
	for i := 0; i < outreachSubCount; i++ {
		m = pressAppKey(t, m, "tab")
	}
	if m.outreach.sub != outreachSubSetup {
		t.Errorf("tab x%d: sub = %d, want back at Setup(%d)", outreachSubCount, m.outreach.sub, outreachSubSetup)
	}
}

func TestOutreachDigitKeysJumpToSubSection(t *testing.T) {
	m := AppModel{activeTab: TabOutreach, outreach: NewOutreachHubModel(), profileComplete: true}

	m = pressAppKey(t, m, "4")
	if m.outreach.sub != outreachSubSent {
		t.Errorf("key 4: sub = %d, want Sent(%d)", m.outreach.sub, outreachSubSent)
	}
	if !m.outreach.logLoading {
		t.Error("key 4 → Sent should trigger audit-log loading")
	}

	m = pressAppKey(t, m, "1")
	if m.outreach.sub != outreachSubSetup {
		t.Errorf("key 1: sub = %d, want Setup(%d)", m.outreach.sub, outreachSubSetup)
	}

	m = pressAppKey(t, m, "2")
	if m.outreach.sub != outreachSubEmail {
		t.Errorf("key 2: sub = %d, want Email(%d)", m.outreach.sub, outreachSubEmail)
	}
	if m.activeTab != TabOutreach {
		t.Errorf("digit keys must not switch the main tab, activeTab = %d", m.activeTab)
	}
}

func TestOutreachEscStillEntersTabMode(t *testing.T) {
	m := AppModel{activeTab: TabOutreach, outreach: NewOutreachHubModel(), profileComplete: true}
	m = pressAppKey(t, m, "esc")
	if !m.chromeNav {
		t.Error("esc from outreach browse should enter chrome nav (tab mode)")
	}
}

func TestOutreachGotoSubSameIsNoop(t *testing.T) {
	hub := NewOutreachHubModel()
	nm, cmd := hub.gotoSub(outreachSubSetup)
	if nm.sub != outreachSubSetup || cmd != nil || nm.logLoading {
		t.Errorf("gotoSub(current) = sub %d, cmd %v, logLoading %v; want no-op", nm.sub, cmd, nm.logLoading)
	}
}

func TestResumeTabKeyCyclesSteps(t *testing.T) {
	m := AppModel{activeTab: TabResume, resumeHub: NewResumeHubModel(), profileComplete: true}
	start := m.resumeHub.sub

	m = pressAppKey(t, m, "tab")
	if m.resumeHub.sub != start+1 {
		t.Errorf("tab: sub = %d, want %d", m.resumeHub.sub, start+1)
	}
	if m.activeTab != TabResume {
		t.Errorf("tab must not switch the main tab, activeTab = %d", m.activeTab)
	}

	m = pressAppKey(t, m, "shift+tab")
	if m.resumeHub.sub != start {
		t.Errorf("shift+tab: sub = %d, want %d", m.resumeHub.sub, start)
	}
}

func TestContactsTabKeyMovesBetweenInputs(t *testing.T) {
	// Contacts captures tab for its company → domain input navigation; the
	// main tab must not change.
	m := AppModel{activeTab: TabContacts, contacts: NewContactsTabModel(), profileComplete: true}
	m = pressAppKey(t, m, "tab")
	if m.activeTab != TabContacts {
		t.Errorf("tab in Contacts input mode: activeTab = %d, want Contacts(%d)", m.activeTab, TabContacts)
	}
	if m.contacts.focusField != 1 {
		t.Errorf("tab in Contacts input mode: focusField = %d, want 1 (domain)", m.contacts.focusField)
	}
}

func TestContactsTabKeyStillSwitchesMainTab(t *testing.T) {
	// Contacts has no tab-cycling sub-menu (its sub-tabs use 1/2) — when it is
	// idle (not capturing keys), tab must keep switching main tabs.
	c := NewContactsTabModel()
	c.mode = contactsModeIdle
	m := AppModel{activeTab: TabContacts, contacts: c, profileComplete: true}
	m = pressAppKey(t, m, "tab")
	if m.activeTab != TabLogs {
		t.Errorf("tab from idle Contacts: activeTab = %d, want Logs(%d)", m.activeTab, TabLogs)
	}
}
