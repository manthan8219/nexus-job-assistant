package agentx

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// fakeModel is a scripted model.BaseChatModel for hermetic agent tests.
type fakeModel struct {
	content string
	err     error
	gotVars [][]*schema.Message
}

func (f *fakeModel) Generate(_ context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.gotVars = append(f.gotVars, msgs)
	if f.err != nil {
		return nil, f.err
	}
	return &schema.Message{Role: schema.Assistant, Content: f.content}, nil
}

func (f *fakeModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream not supported by fake")
}

type greetIn struct {
	Name string
}
type greetOut struct {
	Greeting string `json:"greeting"`
}

func newGreeter(m model.BaseChatModel) (*Agent[greetIn, greetOut], error) {
	return New[greetIn, greetOut]("greeter", m,
		"You greet people.",
		"Greet {name} warmly.",
		func(in greetIn) map[string]any { return map[string]any{"name": in.Name} },
		ParseJSON[greetOut])
}

func TestAgentRun(t *testing.T) {
	fm := &fakeModel{content: `{"greeting":"hello ada"}`}
	ag, err := newGreeter(fm)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if ag.Name() != "greeter" {
		t.Errorf("Name() = %q; want greeter", ag.Name())
	}
	got, err := ag.Run(context.Background(), greetIn{Name: "ada"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Greeting != "hello ada" {
		t.Errorf("Run = %+v; want greeting 'hello ada'", got)
	}
	// Template variables must reach the model: user message contains the name.
	if len(fm.gotVars) != 1 {
		t.Fatalf("model called %d times; want 1", len(fm.gotVars))
	}
	msgs := fm.gotVars[0]
	if len(msgs) != 2 {
		t.Fatalf("model got %d messages; want system+user", len(msgs))
	}
	if !strings.Contains(msgs[1].Content, "ada") {
		t.Errorf("user message %q lacks substituted variable", msgs[1].Content)
	}
}

func TestAgentRunErrors(t *testing.T) {
	t.Run("model error propagates", func(t *testing.T) {
		ag, err := newGreeter(&fakeModel{err: errors.New("boom")})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, err := ag.Run(context.Background(), greetIn{Name: "ada"}); err == nil {
			t.Fatal("Run expected error, got nil")
		}
	})

	t.Run("empty response names the agent", func(t *testing.T) {
		ag, err := newGreeter(&fakeModel{content: "   "})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		_, err = ag.Run(context.Background(), greetIn{Name: "ada"})
		if err == nil || !strings.Contains(err.Error(), "greeter") {
			t.Fatalf("Run error = %v; want mention of agent name", err)
		}
	})

	t.Run("bad json names the agent", func(t *testing.T) {
		ag, err := newGreeter(&fakeModel{content: "no json at all"})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		_, err = ag.Run(context.Background(), greetIn{Name: "ada"})
		if err == nil || !strings.Contains(err.Error(), "greeter") {
			t.Fatalf("Run error = %v; want mention of agent name", err)
		}
	})
}

func TestNewValidation(t *testing.T) {
	fm := &fakeModel{}
	if _, err := New[greetIn, greetOut]("x", nil, "s", "u", nil, nil); err == nil {
		t.Fatal("New(nil model) expected error")
	}
	if _, err := New[greetIn, greetOut]("x", fm, "s", "u", nil, nil); err == nil {
		t.Fatal("New(nil vars/parse) expected error")
	}
}
