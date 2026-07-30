package agentx

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// Default model names and budgets, mirroring the direct-HTTP fallbacks in
// internal/resume so behavior stays consistent across both AI paths.
const (
	defaultClaudeModel = "claude-3-5-haiku-latest"
	defaultOpenAIModel = "gpt-4o-mini"
	defaultOllamaURL   = "http://localhost:11434"
	defaultMaxTokens   = 4096
	requestTimeout     = 5 * time.Minute
)

// NewChatModel builds an Eino chat model from the Nexus AI configuration.
// Provider "local" (the default) uses Ollama at cfg.LocalLLMURL with JSON
// mode forced; "api" uses Claude when an Anthropic key is set, else OpenAI —
// the same precedence as the internal/resume completion routing.
func NewChatModel(ctx context.Context, cfg *config.Config) (model.BaseChatModel, error) {
	if cfg == nil || !cfg.AIAssist {
		return nil, fmt.Errorf("agentx: enable AI Assist in Config first")
	}
	switch strings.ToLower(cfg.AIProvider) {
	case "api":
		if cfg.AnthropicKey != "" {
			return claude.NewChatModel(ctx, &claude.Config{
				APIKey:    cfg.AnthropicKey,
				Model:     defaultClaudeModel,
				MaxTokens: defaultMaxTokens,
			})
		}
		if cfg.OpenAIKey != "" {
			maxTokens := defaultMaxTokens
			return openai.NewChatModel(ctx, &openai.ChatModelConfig{
				APIKey:    cfg.OpenAIKey,
				Model:     defaultOpenAIModel,
				MaxTokens: &maxTokens,
				ResponseFormat: &openai.ChatCompletionResponseFormat{
					Type: openai.ChatCompletionResponseFormatTypeJSONObject,
				},
			})
		}
		return nil, fmt.Errorf("agentx: AI backend is API keys but no Anthropic/OpenAI key is set")
	default:
		if strings.TrimSpace(cfg.LocalLLMModel) == "" {
			return nil, fmt.Errorf("agentx: no local model selected — pick one under AI Configuration")
		}
		baseURL := strings.TrimSpace(cfg.LocalLLMURL)
		if baseURL == "" {
			baseURL = defaultOllamaURL
		}
		return ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
			BaseURL: baseURL,
			Model:   cfg.LocalLLMModel,
			Timeout: requestTimeout,
			Format:  json.RawMessage(`"json"`), // force JSON mode like localllm.GenerateJSON
		})
	}
}
