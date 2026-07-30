package ui

import (
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

// makeForm returns a FormModel with a pre-set analysis result.
func makeForm(analysisDone bool, valid bool) FormModel {
	cfg := &config.Config{
		FirstName:  "John",
		LastName:   "Doe",
		Email:      "john@example.com",
		Phone:      "1234567890",
		LinkedInID: "johndoe",
		ResumePath: "/tmp/resume.pdf",
	}
	m := NewFormModel(cfg, false)
	m.resumeAnalysisDone = analysisDone
	if analysisDone {
		m.resumeAnalysisResult = resume.Result{
			Valid:    valid,
			FileType: "PDF",
			Message:  "PDF · 5 resume keywords found",
			Err:      "not a valid PDF",
		}
		if valid {
			m.resumeAnalysisResult.Err = ""
		} else {
			m.resumeAnalysisResult.Message = ""
		}
	}
	m.resumeAnalyzing = false
	return m
}

func TestResumeInvalid_FalseWhenNotYetAnalyzed(t *testing.T) {
	m := makeForm(false, false)
	if m.resumeInvalid() {
		t.Fatal("resumeInvalid should be false when analysis not done yet")
	}
}

func TestResumeInvalid_FalseWhenValid(t *testing.T) {
	m := makeForm(true, true)
	if m.resumeInvalid() {
		t.Fatal("resumeInvalid should be false when analysis passed")
	}
}

func TestResumeInvalid_TrueWhenFailed(t *testing.T) {
	m := makeForm(true, false)
	if !m.resumeInvalid() {
		t.Fatal("resumeInvalid should be true when analysis failed")
	}
}

func TestIsLockedByResume_PersonalFieldsNotLocked(t *testing.T) {
	personalFields := []int{fFirstName, fLastName, fEmail, fPhone, fLinkedInID, fResumePath}
	for _, f := range personalFields {
		if isLockedByResume(f) {
			t.Errorf("field %d (personal) should NOT be locked by resume", f)
		}
	}
}

func TestIsLockedByResume_JobPrefFieldsAreLocked(t *testing.T) {
	jobPrefFields := []int{fCity, fYearsExp, fJobTitles, fWorkType, fLocations, fCurrency, fMinSalary}
	for _, f := range jobPrefFields {
		if !isLockedByResume(f) {
			t.Errorf("field %d (job pref) should be locked by resume", f)
		}
	}
}

func TestNavigation_BlockedPastResumePath_WhenInvalid(t *testing.T) {
	m := makeForm(true, false)
	// Start focused on resume path
	m.focused = fResumePath

	// Try navigating forward — should bounce back to fResumePath
	import_tea_key := func(k string) interface{} {
		return nil // placeholder, we test the logic directly
	}
	_ = import_tea_key

	// Test the logic directly: next field after fResumePath is fCity
	next := (fResumePath + 1) % fieldCount
	if m.resumeInvalid() && isLockedByResume(next) {
		next = fResumePath
	}
	if next != fResumePath {
		t.Errorf("expected navigation to stay at fResumePath, got field %d", next)
	}
}

func TestNavigation_AllowedPastResumePath_WhenValid(t *testing.T) {
	m := makeForm(true, true)
	m.focused = fResumePath

	next := (fResumePath + 1) % fieldCount
	if m.resumeInvalid() && isLockedByResume(next) {
		next = fResumePath
	}
	// Should advance to fCity (fResumePath + 1)
	if next != fResumePath+1 {
		t.Errorf("expected navigation to advance past fResumePath, got field %d", next)
	}
}

func TestNavigation_AllowedPastResumePath_WhenNotYetAnalyzed(t *testing.T) {
	m := makeForm(false, false) // not analyzed yet — should not lock
	m.focused = fResumePath

	next := (fResumePath + 1) % fieldCount
	if m.resumeInvalid() && isLockedByResume(next) {
		next = fResumePath
	}
	if next != fResumePath+1 {
		t.Errorf("expected navigation to advance when not yet analyzed, got field %d", next)
	}
}
