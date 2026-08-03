package agentx

import (
	"context"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// apiCfg returns a config with AI Assist on and AIProvider set to "api".
func apiCfg(keys map[string]string) *config.Config {
	cfg := &config.Config{AIAssist: true, AIProvider: "api"}
	cfg.AnthropicKey = keys["anthropic"]
	cfg.OpenAIKey = keys["openai"]
	cfg.GoogleKey = keys["google"]
	cfg.DeepSeekKey = keys["deepseek"]
	cfg.GroqKey = keys["groq"]
	cfg.MistralKey = keys["mistral"]
	cfg.TogetherKey = keys["together"]
	cfg.OpenRouterKey = keys["openrouter"]
	cfg.XAIKey = keys["xai"]
	return cfg
}

func TestNewChatModel_RequiresAIAssist(t *testing.T) {
	if _, err := NewChatModel(context.Background(), nil); err == nil {
		t.Fatal("nil config should error")
	}
	if _, err := NewChatModel(context.Background(), &config.Config{}); err == nil {
		t.Fatal("AIAssist off should error")
	}
}

func TestNewChatModel_APIMode_AnthropicWins(t *testing.T) {
	// Anthropic takes precedence over any OpenAI-compatible provider.
	cfg := apiCfg(map[string]string{"anthropic": "sk-ant", "openai": "sk-o", "google": "gem"})
	m, err := NewChatModel(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewChatModel: %v", err)
	}
	if m == nil {
		t.Fatal("expected a non-nil chat model")
	}
}

func TestNewChatModel_APIMode_OpenAICompatible(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"openai", "openai"},
		{"google", "google"},
		{"deepseek", "deepseek"},
		{"groq", "groq"},
		{"mistral", "mistral"},
		{"together", "together"},
		{"openrouter", "openrouter"},
		{"xai", "xai"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := apiCfg(map[string]string{tt.key: "sk-test"})
			m, err := NewChatModel(context.Background(), cfg)
			if err != nil {
				t.Fatalf("NewChatModel(%s): %v", tt.name, err)
			}
			if m == nil {
				t.Fatalf("expected a non-nil model for %s", tt.name)
			}
		})
	}
}

func TestNewChatModel_APIMode_NoKeyErrors(t *testing.T) {
	cfg := apiCfg(nil)
	_, err := NewChatModel(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "no provider key") {
		t.Fatalf("err = %v; want 'no provider key' error", err)
	}
}

func TestNewChatModel_LocalMode_RequiresModel(t *testing.T) {
	cfg := &config.Config{AIAssist: true, AIProvider: "local"}
	if _, err := NewChatModel(context.Background(), cfg); err == nil {
		t.Fatal("local mode without a model should error")
	}
}

func TestNewChatModel_LocalMode_WithModel(t *testing.T) {
	cfg := &config.Config{AIAssist: true, AIProvider: "local", LocalLLMModel: "llama3"}
	m, err := NewChatModel(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewChatModel: %v", err)
	}
	if m == nil {
		t.Fatal("expected a non-nil local chat model")
	}
}
