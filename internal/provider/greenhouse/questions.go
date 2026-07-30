package greenhouse

import (
	"context"

	"github.com/manthan8219/nexus-job-assistant/internal/provider/lever"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

// AnswerQuestions asks the configured AI to answer a form's custom questions,
// reusing Lever's battle-tested prompting/validation logic: Greenhouse and
// Lever custom questions are structurally identical (free text, dropdown,
// multi-select), so the questions are converted to the shared shape, answered,
// and mapped back — keeping one well-tuned prompt implementation.
//
// Only custom (question_NNN) text/select questions are sent to the AI; basic
// fields (name/email/…) come from the profile and files are uploaded.
func AnswerQuestions(ctx context.Context, ai resume.AIOptions, questions []ghQuestion, actx lever.AnswerContext) ([]Answer, error) {
	var lqs []lever.Question
	var custom []ghQuestion
	for _, q := range questions {
		if !isCustomQuestion(q) {
			continue
		}
		field := q.Fields[0]
		lq := lever.Question{
			FieldName: field.Name,
			Type:      leverQuestionType(field.Type),
			Text:      q.Label,
			Required:  q.Required,
		}
		for _, v := range field.Values {
			lq.Options = append(lq.Options, v.Label)
		}
		lqs = append(lqs, lq)
		custom = append(custom, q)
	}

	las, err := lever.AnswerQuestions(ctx, ai, lqs, actx)
	if err != nil {
		return nil, err
	}

	out := make([]Answer, len(custom))
	for i, la := range las {
		out[i] = Answer{Question: custom[i], Value: la.Value, Err: la.Err}
	}
	return out, nil
}

// leverQuestionType maps Greenhouse field types to Lever question types.
func leverQuestionType(ghType string) string {
	switch ghType {
	case "textarea":
		return "textarea"
	case "multi_value_single_select":
		return "dropdown"
	case "multi_value_multi_select":
		return "multiple-select"
	default:
		return "text"
	}
}
