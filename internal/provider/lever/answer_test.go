package lever

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

func TestValidateAnswer(t *testing.T) {
	cases := []struct {
		name      string
		q         Question
		value     string
		grounding string
		want      string
	}{
		{"empty passthrough", Question{}, "", "resume", ""},
		{"free text no digits kept", Question{Text: "Why?"}, "Because I love Go", "resume text", "Because I love Go"},
		{"grounded number kept", Question{Text: "Exp?"}, "I have 5 years", "5 years experience", "I have 5 years"},
		{"ungrounded number cleared", Question{Text: "Exp?"}, "I have 99 years", "5 years experience", ""},
		{"word boundary cleared (30 in 3000000)", Question{Text: "Notice?"}, "30 days notice", "salary 3000000", ""},
		{"word boundary kept (30 standalone)", Question{Text: "Notice?"}, "30 days notice", "30 days notice period", "30 days notice"},
		{"dropdown valid kept", Question{Type: "dropdown", Options: []string{"Yes", "No"}}, "Yes", "", "Yes"},
		{"dropdown invalid cleared", Question{Type: "dropdown", Options: []string{"Yes", "No"}}, "Maybe", "", ""},
		{"dropdown case-insensitive kept", Question{Type: "dropdown", Options: []string{"Yes", "No"}}, "yes", "", "yes"},
		{"dropdown trims spaces", Question{Type: "dropdown", Options: []string{"Yes"}}, "  Yes  ", "", "Yes"},
		{"multiple-select mixed filtered", Question{Type: "multiple-select", Options: []string{"Mon", "Tue", "Wed"}}, "Mon; Tue; Bogus", "", "Mon; Tue"},
		{"multiple-select all invalid cleared", Question{Type: "multiple-select", Options: []string{"Mon", "Tue"}}, "Foo; Bar", "", ""},
		{"multiple-select all valid kept", Question{Type: "multiple-select", Options: []string{"Mon", "Tue"}}, "Mon; Tue", "", "Mon; Tue"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := validateAnswer(c.q, c.value, c.grounding)
			if got != c.want {
				t.Errorf("validateAnswer(%q) = %q; want %q", c.value, got, c.want)
			}
		})
	}
}

func TestParseAnswer(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"clean json", `{"value":"hello"}`, "hello"},
		{"json with surrounding whitespace", `  {"value":"hello"}  `, "hello"},
		{"json embedded in commentary", `Here is my answer: {"value":"hi"} thanks`, "hi"},
		{"raw fallback", "plain text", "plain text"},
		{"empty value", `{"value":""}`, ""},
		{"empty raw", "", ""},
		{"inner spaces preserved", `{"value":"  spaced  "}`, "  spaced  "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseAnswer(c.raw)
			if err != nil {
				t.Fatalf("parseAnswer(%q) unexpected error: %v", c.raw, err)
			}
			if got != c.want {
				t.Errorf("parseAnswer(%q) = %q; want %q", c.raw, got, c.want)
			}
		})
	}
}

func TestBuildAnswerPrompt(t *testing.T) {
	actx := AnswerContext{
		FirstName: "Ada", LastName: "Lovelace", Email: "ada@x.com", Phone: "555-1234",
		City: "London", YearsExp: "10", Currency: "USD", MinSalary: "100000",
		WorkAuth: "citizen", WorkType: "Remote", NoticePeriod: "30", OfficeDays: "3",
		CompanyName: "Acme", JobTitle: "Backend Engineer", ResumeText: "I built distributed systems.",
	}
	prompt := buildAnswerPrompt(Question{Text: "Why this role?", Type: "textarea"}, actx, nil)
	for _, want := range []string{
		"Ada Lovelace", "ada@x.com", "London", "10", "USD", "100000",
		"citizen", "Remote", "30", "3", "Acme", "Backend Engineer",
		"I built distributed systems.", "RESUME TEXT:", "Why this role?",
		"NEVER been employed by",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildAnswerPrompt_OptionsListed(t *testing.T) {
	q := Question{Text: "Which days?", Type: "multiple-select", Options: []string{"Mon", "Tue"}}
	prompt := buildAnswerPrompt(q, AnswerContext{FirstName: "A", LastName: "B", JobTitle: "Eng", CompanyName: "Co"}, nil)
	if !strings.Contains(prompt, "choice question") {
		t.Error("expected choice-question instruction for optioned question")
	}
	if !strings.Contains(prompt, "Mon") || !strings.Contains(prompt, "Tue") {
		t.Error("expected options listed in prompt")
	}
}

func TestBuildAnswerPrompt_PriorQuestionsIncluded(t *testing.T) {
	prior := []answeredPair{{Text: "Are you authorized?", Value: "No"}}
	prompt := buildAnswerPrompt(Question{Text: "If yes, details?", Type: "textarea"},
		AnswerContext{FirstName: "A", LastName: "B", JobTitle: "Eng", CompanyName: "Co"}, prior)
	if !strings.Contains(prompt, "EARLIER QUESTIONS") {
		t.Error("expected prior-questions block when prior is non-empty")
	}
	if !strings.Contains(prompt, "Are you authorized?") || !strings.Contains(prompt, `"No"`) {
		t.Error("expected prior Q/A in prompt")
	}
}

func TestAnswerQuestions_ModelSuccess(t *testing.T) {
	orig := completeFn
	completeFn = func(_ context.Context, _ resume.AIOptions, _ string) (string, error) {
		return `{"value":"Yes"}`, nil
	}
	defer func() { completeFn = orig }()

	q := Question{Text: "Are you authorized to work?", Type: "dropdown", Options: []string{"Yes", "No"}, FieldName: "cards[x][field0]"}
	answers, err := AnswerQuestions(context.Background(), resume.AIOptions{}, []Question{q},
		AnswerContext{FirstName: "A", LastName: "B", JobTitle: "Eng", CompanyName: "Co"})
	if err != nil {
		t.Fatalf("AnswerQuestions: %v", err)
	}
	if len(answers) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(answers))
	}
	if answers[0].Value != "Yes" {
		t.Errorf("value = %q; want \"Yes\"", answers[0].Value)
	}
	if answers[0].Err != nil {
		t.Errorf("unexpected error: %v", answers[0].Err)
	}
}

func TestAnswerQuestions_ModelError(t *testing.T) {
	orig := completeFn
	completeFn = func(_ context.Context, _ resume.AIOptions, _ string) (string, error) {
		return "", errors.New("model unavailable")
	}
	defer func() { completeFn = orig }()

	answers, err := AnswerQuestions(context.Background(), resume.AIOptions{},
		[]Question{{Text: "Q?", FieldName: "f"}},
		AnswerContext{FirstName: "A", LastName: "B", JobTitle: "E", CompanyName: "C"})
	if err != nil {
		t.Fatalf("AnswerQuestions: %v", err)
	}
	if answers[0].Err == nil {
		t.Fatal("expected per-question error when model fails")
	}
	if answers[0].Value != "" {
		t.Errorf("value = %q; want \"\" on model error", answers[0].Value)
	}
}

// AGENTS.md section 14: a hallucinated number not traceable to the resume or
// supplied facts must be cleared (left for human input) rather than submitted.
func TestAnswerQuestions_UngroundedNumberCleared(t *testing.T) {
	orig := completeFn
	completeFn = func(_ context.Context, _ resume.AIOptions, _ string) (string, error) {
		return `{"value":"I have 99 years of experience"}`, nil
	}
	defer func() { completeFn = orig }()

	// Grounding has "5" (YearsExp) but no "99" -> the invented 99 must be cleared.
	answers, err := AnswerQuestions(context.Background(), resume.AIOptions{},
		[]Question{{Text: "Experience?", Type: "textarea", FieldName: "f"}},
		AnswerContext{FirstName: "A", LastName: "B", JobTitle: "E", CompanyName: "C", YearsExp: "5"})
	if err != nil {
		t.Fatalf("AnswerQuestions: %v", err)
	}
	if answers[0].Err != nil {
		t.Errorf("unexpected error: %v", answers[0].Err)
	}
	if answers[0].Value != "" {
		t.Errorf("ungrounded number should be cleared, got %q", answers[0].Value)
	}
}

func TestAnswerQuestions_PriorQuestionCarryThrough(t *testing.T) {
	var prompts []string
	orig := completeFn
	completeFn = func(_ context.Context, _ resume.AIOptions, prompt string) (string, error) {
		prompts = append(prompts, prompt)
		return `{"value":"No"}`, nil
	}
	defer func() { completeFn = orig }()

	questions := []Question{
		{Text: "Are you authorized?", Type: "dropdown", Options: []string{"Yes", "No"}, FieldName: "f0"},
		{Text: "If yes, visa details?", Type: "textarea", FieldName: "f1"},
	}
	if _, err := AnswerQuestions(context.Background(), resume.AIOptions{}, questions,
		AnswerContext{FirstName: "A", LastName: "B", JobTitle: "E", CompanyName: "C"}); err != nil {
		t.Fatalf("AnswerQuestions: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("expected 2 completions, got %d", len(prompts))
	}
	if !strings.Contains(prompts[1], "Are you authorized?") {
		t.Error("second prompt should include the first question's text")
	}
	if !strings.Contains(prompts[1], `"No"`) {
		t.Error("second prompt should include the first answer \"No\"")
	}
}
