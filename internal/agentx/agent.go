package agentx

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// Agent is a single LLM-backed step: typed input in, typed output out.
// Internally it is a compiled Eino chain of chat template → chat model →
// tolerant JSON parse, so agents compose, trace, and mock uniformly.
type Agent[In, Out any] struct {
	name   string
	vars   func(In) map[string]any
	runner compose.Runnable[map[string]any, Out]
}

// New builds an Agent from a system persona, a user-message template using
// {variable} placeholders (FString syntax), a mapper from In to template
// variables, and a parser from raw model output to Out. Templates must not
// contain literal braces — pass brace-heavy content (JSON examples, prior
// feedback) in as variable values.
func New[In, Out any](
	name string,
	m model.BaseChatModel,
	system, userTemplate string,
	vars func(In) map[string]any,
	parse func(string) (Out, error),
) (*Agent[In, Out], error) {
	if m == nil {
		return nil, fmt.Errorf("agentx: %s: nil chat model", name)
	}
	if vars == nil || parse == nil {
		return nil, fmt.Errorf("agentx: %s: vars and parse are required", name)
	}
	tmpl := prompt.FromMessages(schema.FString,
		schema.SystemMessage(system),
		schema.UserMessage(userTemplate),
	)
	chain := compose.NewChain[map[string]any, Out]()
	chain.AppendChatTemplate(tmpl)
	chain.AppendChatModel(m)
	chain.AppendLambda(compose.InvokableLambda(func(_ context.Context, msg *schema.Message) (Out, error) {
		var zero Out
		if msg == nil || strings.TrimSpace(msg.Content) == "" {
			return zero, fmt.Errorf("agent %s: empty model response", name)
		}
		out, err := parse(msg.Content)
		if err != nil {
			return zero, fmt.Errorf("agent %s: %w", name, err)
		}
		return out, nil
	}))
	r, err := chain.Compile(context.Background())
	if err != nil {
		return nil, fmt.Errorf("agentx: %s: compile: %w", name, err)
	}
	return &Agent[In, Out]{name: name, vars: vars, runner: r}, nil
}

// Name returns the agent identifier.
func (a *Agent[In, Out]) Name() string { return a.name }

// Run executes the template → model → parse chain for one input.
func (a *Agent[In, Out]) Run(ctx context.Context, in In) (Out, error) {
	return a.runner.Invoke(ctx, a.vars(in))
}
