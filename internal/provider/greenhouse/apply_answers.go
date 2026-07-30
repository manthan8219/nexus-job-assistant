package greenhouse

// Package greenhouse — apply_answers.go
// Answer-guessing heuristics for the Greenhouse apply form: mapping question
// labels to Profile values and picking safe defaults for common select/text
// questions. This is the engine's no-AI fast path (AutoAnswers). The HTTP
// submission + payload-building lives in apply.go.

import (
	"strconv"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// questionID extracts the numeric question id from a field name like
// "question_67517573" or "question_67517575[]".
func questionID(fieldName string) (int64, bool) {
	s := strings.TrimSuffix(strings.TrimPrefix(fieldName, "question_"), "[]")
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil
}

// isBoolOption reports whether an option value is one of Greenhouse's
// boolean encodings (1 = yes, 0 = no) — the front-end submits those as
// boolean_value instead of an option id.
func isBoolOption(v ghValue) bool {
	s := strings.TrimSpace(v.ValueStr())
	return s == "1" || s == "0"
}

// resolveOptions maps a "label; label" answer string back to the question's
// option values (case-insensitive, exact match first, then substring).
func resolveOptions(answer string, values []ghValue) []ghValue {
	if answer == "" {
		return nil
	}
	var out []ghValue
	for _, want := range strings.Split(answer, ";") {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		if v, ok := matchOption(want, values); ok {
			out = append(out, v)
		}
	}
	return out
}

func matchOption(want string, values []ghValue) (ghValue, bool) {
	lw := strings.ToLower(want)
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v.Label), want) {
			return v, true
		}
	}
	for _, v := range values {
		if strings.Contains(strings.ToLower(v.Label), lw) {
			return v, true
		}
	}
	return ghValue{}, false
}

// profileValue maps a basic Greenhouse field name to a Profile value.
func profileValue(name string, p provider.Profile) string {
	switch name {
	case "first_name":
		return p.FirstName
	case "last_name":
		return p.LastName
	case "email":
		return p.Email
	case "phone":
		return p.Phone
	}
	return ""
}

// isCustomQuestion reports whether a question is a custom (question_NNN)
// one rather than a basic applicant field.
func isCustomQuestion(q ghQuestion) bool {
	if len(q.Fields) == 0 {
		return false
	}
	return strings.HasPrefix(q.Fields[0].Name, "question_")
}

// isConsentLabel matches privacy/consent acknowledgement questions where
// ticking the single offered box is the expected answer.
func isConsentLabel(label string) bool {
	l := strings.ToLower(label)
	return strings.Contains(l, "consent") || strings.Contains(l, "privacy") ||
		strings.Contains(l, "acknowledge") || strings.Contains(l, "review the points") ||
		strings.Contains(l, "gdpr") || strings.Contains(l, "terms")
}

// fieldResolver returns a Profile value for a question label substring.
type fieldResolver func(p provider.Profile) string

var labelResolvers = map[string]fieldResolver{
	"first name":   func(p provider.Profile) string { return p.FirstName },
	"last name":    func(p provider.Profile) string { return p.LastName },
	"email":        func(p provider.Profile) string { return p.Email },
	"phone":        func(p provider.Profile) string { return p.Phone },
	"city":         func(p provider.Profile) string { return p.City },
	"location":     func(p provider.Profile) string { return p.City },
	"linkedin":     func(p provider.Profile) string { return "https://linkedin.com/in/" + p.LinkedInID },
	"github":       func(p provider.Profile) string { return p.Website },
	"website":      func(p provider.Profile) string { return p.Website },
	"portfolio":    func(p provider.Profile) string { return p.Website },
	"salary":       func(p provider.Profile) string { return p.MinSalary },
	"compensation": func(p provider.Profile) string { return p.MinSalary },
	"experience":   func(p provider.Profile) string { return p.YearsExp },
	"years":        func(p provider.Profile) string { return p.YearsExp },
}

// resolveValue tries to find a value for a question label from the profile.
func resolveValue(label string, p provider.Profile) (string, bool) {
	lower := strings.ToLower(label)
	for substring, resolver := range labelResolvers {
		if strings.Contains(lower, substring) {
			val := resolver(p)
			return val, val != ""
		}
	}
	return "", false
}

// AutoAnswers fills every custom question it can from the profile and
// well-known defaults — the engine's no-AI fast path.
func AutoAnswers(questions []ghQuestion, profile provider.Profile) []Answer {
	var out []Answer
	for _, q := range questions {
		if !isCustomQuestion(q) {
			continue
		}
		field := q.Fields[0]
		a := Answer{Question: q}
		switch field.Type {
		case "input_text", "textarea":
			if val, ok := resolveValue(q.Label, profile); ok {
				a.Value = val
			} else if def := autoAnswerText(q.Label); def != "" {
				a.Value = def
			}
		case "multi_value_single_select":
			if v, ok := autoAnswerOption(q.Label, field.Values); ok {
				a.Value = v.Label
			}
		case "multi_value_multi_select":
			// Only safe default: single-option consent/acknowledgement checkboxes.
			if len(field.Values) == 1 && isConsentLabel(q.Label) {
				a.Value = field.Values[0].Label
			}
		}
		out = append(out, a)
	}
	return out
}

// autoAnswerOption tries to pick an option for simple yes/no / authorization /
// demographic questions. It returns the chosen option, or false.
func autoAnswerOption(label string, values []ghValue) (ghValue, bool) {
	lower := strings.ToLower(label)

	findLabel := func(candidates ...string) (ghValue, bool) {
		for _, v := range values {
			vl := strings.ToLower(v.Label)
			for _, c := range candidates {
				if strings.Contains(vl, c) {
					return v, true
				}
			}
		}
		return ghValue{}, false
	}

	// Work authorization → yes
	if strings.Contains(lower, "authorized") || strings.Contains(lower, "authorised") ||
		strings.Contains(lower, "legally") || strings.Contains(lower, "eligible") {
		if v, ok := findLabel("yes"); ok {
			return v, true
		}
	}

	// Sponsorship required → no
	if strings.Contains(lower, "sponsor") {
		if v, ok := findLabel("no"); ok {
			return v, true
		}
	}

	// Previously employed → no
	if strings.Contains(lower, "employed") || strings.Contains(lower, "worked") {
		if v, ok := findLabel("no"); ok {
			return v, true
		}
	}

	// Relatives at company → no
	if strings.Contains(lower, "relative") || strings.Contains(lower, "blood") || strings.Contains(lower, "family") {
		if v, ok := findLabel("no"); ok {
			return v, true
		}
	}

	// Non-compete / non-solicitation → no
	if strings.Contains(lower, "non-compete") || strings.Contains(lower, "non-solicitation") || strings.Contains(lower, "noncompete") {
		if v, ok := findLabel("no"); ok {
			return v, true
		}
	}

	// How did you hear about → LinkedIn / Job Board
	if strings.Contains(lower, "hear about") || strings.Contains(lower, "how did you") || strings.Contains(lower, "source") {
		if v, ok := findLabel("linkedin"); ok {
			return v, true
		}
		if v, ok := findLabel("job board", "indeed", "glassdoor", "internet", "online"); ok {
			return v, true
		}
		if len(values) > 0 {
			return values[0], true
		}
	}

	// Privacy notice / consent → agree / yes
	if isConsentLabel(lower) {
		if v, ok := findLabel("agree", "yes", "accept", "acknowledge"); ok {
			return v, true
		}
		if len(values) > 0 {
			return values[0], true
		}
	}

	// Gender → prefer not to say
	if strings.Contains(lower, "gender") {
		if v, ok := findLabel("decline", "prefer not", "not to say", "rather not"); ok {
			return v, true
		}
	}

	// Race / ethnicity → prefer not to say
	if strings.Contains(lower, "race") || strings.Contains(lower, "ethnic") {
		if v, ok := findLabel("decline", "prefer not", "not to say", "rather not"); ok {
			return v, true
		}
	}

	// Veteran status → not a veteran
	if strings.Contains(lower, "veteran") || strings.Contains(lower, "military") {
		if v, ok := findLabel("not a veteran", "no", "i am not"); ok {
			return v, true
		}
	}

	// Disability → no disability
	if strings.Contains(lower, "disabilit") {
		if v, ok := findLabel("no", "i don", "do not"); ok {
			return v, true
		}
	}

	return ghValue{}, false
}

// autoAnswerText returns a default answer for common free-text questions.
func autoAnswerText(label string) string {
	lower := strings.ToLower(label)
	// Numeric scale questions (1-5, 1-10)
	if strings.Contains(lower, "scale of 1") || strings.Contains(lower, "rate yourself") ||
		strings.Contains(lower, "out of 5") || strings.Contains(lower, "out of 10") {
		if strings.Contains(lower, "out of 10") || strings.Contains(lower, "scale of 1-10") {
			return "8"
		}
		return "4"
	}
	// Salary / CTC questions
	if strings.Contains(lower, "salary") || strings.Contains(lower, "ctc") ||
		strings.Contains(lower, "compensation") || strings.Contains(lower, "expect") {
		return "open to discussion"
	}
	// Notice period
	if strings.Contains(lower, "notice period") || strings.Contains(lower, "notice") {
		return "30 days"
	}
	// Years of experience
	if strings.Contains(lower, "years of experience") || strings.Contains(lower, "how many years") {
		return "4"
	}
	return ""
}
