package lever

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/manthanmanthan/nexus/internal/resume"
)

// AnswerContext is everything the AI needs to answer a job's custom
// application questions truthfully.
type AnswerContext struct {
	ResumeText   string
	FirstName    string
	LastName     string
	Email        string
	Phone        string
	City         string
	YearsExp     string
	Currency     string
	MinSalary    string
	WorkAuth     string // e.g. "citizen", "visa required", "unspecified"
	WorkType     string // e.g. "Remote, Onsite, Hybrid" — preferences the applicant accepts
	NoticePeriod string // e.g. "30", "Immediate"
	OfficeDays   string // e.g. "3" — days/week willing to work onsite
	CompanyName  string
	JobTitle     string
}

// Answer is one question paired with the AI's proposed response.
type Answer struct {
	Question Question
	Value    string // for multiple-select, options joined with "; "
	Err      error  // set if this specific question's completion failed
}

type answeredPair struct {
	Text  string
	Value string
}

// AnswerQuestions asks the configured AI to answer each extracted
// question, one completion per question — batching all questions into a
// single JSON-constrained completion proved unreliable on local models
// (they tend to only answer the first one under Ollama's format=json
// grammar). Each question's prompt includes prior questions + answers so
// the model can correctly resolve Lever's common "if yes, ..." follow-up
// pattern (e.g. skip a follow-up when the question it depends on was
// answered "No").
//
// Choice-type answers are validated against the question's own option
// list, and free-text answers are checked for numbers that don't trace
// back to the resume or supplied facts — both are cleared to "" (left for
// human input) rather than submitted, since a wrong-but-plausible-looking
// answer is worse than an obvious gap when a human reviews everything
// before submission anyway.
func AnswerQuestions(ctx context.Context, ai resume.AIOptions, questions []Question, actx AnswerContext) ([]Answer, error) {
	out := make([]Answer, len(questions))
	debug := os.Getenv("NEXUS_DEBUG_AI") != ""
	var prior []answeredPair
	groundingText := actx.ResumeText + " " + actx.YearsExp + " " + actx.MinSalary + " " +
		actx.Phone + " " + actx.NoticePeriod + " " + actx.OfficeDays

	for i, q := range questions {
		prompt := buildAnswerPrompt(q, actx, prior)
		raw, err := resume.Complete(ctx, ai, prompt)
		if err != nil {
			out[i] = Answer{Question: q, Err: fmt.Errorf("ai completion: %w", err)}
			prior = append(prior, answeredPair{Text: q.Text, Value: "(error)"})
			continue
		}
		if debug {
			fmt.Fprintf(os.Stderr, "=== raw AI response (q%d: %s) ===\n%s\n=== end ===\n", i+1, q.FieldName, raw)
		}
		value, err := parseAnswer(raw)
		if err != nil {
			out[i] = Answer{Question: q, Err: fmt.Errorf("parse AI answer: %w", err)}
			prior = append(prior, answeredPair{Text: q.Text, Value: "(error)"})
			continue
		}
		final := validateAnswer(q, value, groundingText)
		out[i] = Answer{Question: q, Value: final}
		shown := final
		if shown == "" {
			shown = "(left blank)"
		}
		prior = append(prior, answeredPair{Text: q.Text, Value: shown})
	}
	return out, nil
}

var digitsRE = regexp.MustCompile(`\d+`)

// validateAnswer clears an answer that doesn't match the question's own
// option list for choice-type questions, and clears free-text answers
// containing a number that doesn't appear anywhere in the resume or
// supplied facts — a cheap guard against the model inventing specific
// figures (notice period, tenure, etc.) it wasn't actually given.
func validateAnswer(q Question, value, groundingText string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if len(q.Options) == 0 {
		for _, digits := range digitsRE.FindAllString(value, -1) {
			// Word-boundary match — a naive substring check would let "30"
			// through just because it appears inside "3000000" (salary).
			re := regexp.MustCompile(`\b` + digits + `\b`)
			if !re.MatchString(groundingText) {
				return "" // number not traceable to any given fact — likely invented
			}
		}
		return value
	}
	valid := func(v string) bool {
		for _, o := range q.Options {
			if strings.EqualFold(strings.TrimSpace(o), v) {
				return true
			}
		}
		return false
	}
	if q.Type == "multiple-select" {
		parts := strings.Split(value, ";")
		var kept []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if valid(p) {
				kept = append(kept, p)
			}
		}
		return strings.Join(kept, "; ")
	}
	if valid(value) {
		return value
	}
	return "" // model returned something not in the option list — leave for human review
}

// parseAnswer extracts {"value": "..."} from a model completion. Falls
// back to the raw trimmed text if the model didn't wrap it in JSON —
// still usable as a free-text answer, and validateAnswer will discard it
// anyway if the question required an exact option match.
func parseAnswer(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	var obj struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		return obj.Value, nil
	}
	// Try to locate a {"value": ...} object anywhere in the text (model
	// added commentary around it).
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			if err := json.Unmarshal([]byte(raw[i:j+1]), &obj); err == nil {
				return obj.Value, nil
			}
		}
	}
	return raw, nil // best-effort: treat the whole response as the answer
}

func buildAnswerPrompt(q Question, actx AnswerContext, prior []answeredPair) string {
	var qb strings.Builder
	qb.WriteString(fmt.Sprintf("Question: %q\n", q.Text))
	if len(q.Options) > 0 {
		qb.WriteString("This is a choice question — your \"value\" MUST be copied verbatim from these exact options:\n")
		for _, o := range q.Options {
			qb.WriteString(fmt.Sprintf("  - %q\n", o))
		}
		if q.Type == "multiple-select" {
			qb.WriteString("If more than one option applies, join the chosen ones with \"; \".\n")
		}
	} else {
		qb.WriteString("This is a free-text question — answer concisely, first person, honest and specific.\n")
	}

	var priorBlock string
	if len(prior) > 0 {
		var pb strings.Builder
		pb.WriteString("EARLIER QUESTIONS ON THIS SAME APPLICATION (in order) — use these to resolve\nfollow-up questions like \"if yes, ...\" or \"if applicable, ...\": if the\nquestion this one depends on was answered \"No\" or left blank, this\nfollow-up does not apply — answer with \"\" rather than a placeholder:\n")
		for _, p := range prior {
			pb.WriteString(fmt.Sprintf("  - Q: %q → A: %q\n", p.Text, p.Value))
		}
		priorBlock = pb.String()
	}

	return fmt.Sprintf(`You are helping %s %s answer ONE custom screening question on a job
application for the role %q at %q, truthfully and concisely, based ONLY
on the facts below. Do not invent experience, employers, or numbers that
aren't supported by the resume or profile facts.

APPLICANT FACTS:
- Email: %s
- Phone: %s
- City: %s
- Years of experience: %s
- Desired salary: %s %s
- Work authorization: %s
- Work arrangements this applicant accepts: %s
- Notice period: %s
- Days per week willing to work onsite/in-office: %s
- IMPORTANT: this applicant has NEVER been employed by %s, and is NOT
  currently employed by any partner of %s, unless the resume text below
  explicitly states otherwise. When a question asks about prior/current
  employment at this company or its partners, answer confidently based
  on this fact (e.g. "No") — this is not a guess, it is given.

RESUME TEXT:
"""
%s
"""

%s
%s

Respond with ONLY a JSON object, no markdown fences, no commentary:
{"value": "<your answer>"}

Only use {"value": ""} when the answer is truly not knowable from the
facts above or the resume, or when a follow-up question doesn't apply
given an earlier answer — not for facts already given to you above.`,
		actx.FirstName, actx.LastName, actx.JobTitle, actx.CompanyName,
		actx.Email, actx.Phone, actx.City, actx.YearsExp,
		actx.Currency, actx.MinSalary, actx.WorkAuth, actx.WorkType,
		actx.NoticePeriod, actx.OfficeDays,
		actx.CompanyName, actx.CompanyName,
		actx.ResumeText, priorBlock, qb.String())
}
