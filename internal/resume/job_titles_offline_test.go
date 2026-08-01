package resume

import (
	"strings"
	"testing"
)

func TestSuggestTitlesOffline(t *testing.T) {
	cases := []struct {
		intent string
		want   string // substring that must appear in the joined suggestions
	}{
		{"I'm a cardiologist, remote", "Cardiolog"},
		{"Registered Nurse, hospital", "Registered Nurse"},
		{"Senior Go Engineer, backend", "Engineer"},
		{"Product Designer, Figma", "Designer"},
		{"Data Analyst, SQL", "Data Analyst"},
		{"Math teacher, high school", "Teacher"},
		{"Accountant", "Accountant"},
		{"Recruiter at a startup", "Recruiter"},
		{"Sales account executive", "Account Executive"},
	}
	for _, c := range cases {
		got := strings.Join(SuggestTitlesOffline(c.intent, "", nil), ", ")
		if !strings.Contains(got, c.want) {
			t.Errorf("intent %q: want suggestions containing %q, got %q", c.intent, c.want, got)
		}
	}
}

func TestSuggestTitlesOfflineFallback(t *testing.T) {
	// Unknown intent falls back to comma-split fragments.
	got := SuggestTitlesOffline("Life Coach, Wellness", "", nil)
	if len(got) == 0 {
		t.Fatal("expected a fallback suggestion")
	}
	if !strings.Contains(strings.Join(got, ","), "Life Coach") {
		t.Errorf("fallback should keep the intent fragment, got %v", got)
	}

	// Empty intent gets a curated generic default.
	empty := SuggestTitlesOffline("", "", nil)
	if len(empty) == 0 {
		t.Error("expected a generic default for an empty intent")
	}

	// Results are capped at 6.
	cap := SuggestTitlesOffline("engineer, doctor, designer, nurse, accountant, teacher, lawyer", "", nil)
	if len(cap) > 6 {
		t.Errorf("expected at most 6 suggestions, got %d", len(cap))
	}
}

func TestSuggestProfession(t *testing.T) {
	cases := []struct {
		name   string
		intent string
		want   string
	}{
		{"cardiologist", "I'm a cardiologist, remote", "Healthcare"},
		{"nurse", "Registered Nurse, hospital", "Healthcare"},
		{"veterinarian", "Veterinarian, clinic", "Healthcare"},
		{"data analyst", "Data Analyst, SQL", "Data/AI"},
		{"data engineer", "Senior Data Engineer, warehouse", "Data/AI"},
		{"ml engineer", "Machine Learning Engineer", "Data/AI"},
		{"go engineer", "Senior Go Engineer, backend", "Engineering"},
		{"devops", "DevOps platform engineer", "Engineering"},
		{"designer", "Product Designer, Figma", "Design"},
		{"ux designer", "UX designer, mobile", "Design"},
		{"research scientist", "Research scientist, genomics", "Research/Science"},
		{"chemist", "Chemist in a lab", "Research/Science"},
		{"marketing", "Growth marketing manager", "Marketing"},
		{"sales", "Sales account executive", "Sales"},
		{"accountant", "Accountant", "Finance"},
		{"teacher", "Math teacher, high school", "Education"},
		{"lawyer", "Corporate lawyer", "Legal"},
		{"recruiter", "Recruiter at a startup", "HR"},
		{"hr manager", "HR manager, people ops", "HR"},
		{"writer", "Technical writer", "Writing"},
		{"electrician", "Electrician, residential", "Trade/Construction"},
		{"support", "Customer support specialist", "Customer Support"},
		{"project manager", "Project manager, agile", "Project Management"},
		{"product manager", "Product manager, fintech", "Project Management"},
		{"unknown", "Life Coach, Wellness", ""},
		{"exploring", "exploring", ""},
		{"empty", "", ""},
		{"email does not imply ai", "Email me at a@example.com", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SuggestProfession(c.intent); got != c.want {
				t.Errorf("SuggestProfession(%q) = %q; want %q", c.intent, got, c.want)
			}
		})
	}
}
