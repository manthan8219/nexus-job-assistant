package resume

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/aiprovider"
	"github.com/manthan8219/nexus-job-assistant/internal/localllm"
)

func completeOpenAICompatible(ctx context.Context, apiKey, baseURL, model, prompt string) (string, error) {
	return completeOpenAICompatibleSplit(ctx, apiKey, baseURL, model,
		"Return only a single valid JSON object. No markdown.", prompt)
}

func completeAnthropic(ctx context.Context, apiKey, prompt string) (string, error) {
	return completeAnthropicTokens(ctx, apiKey, prompt, 1500)
}

func completeAnthropicTokens(ctx context.Context, apiKey, prompt string, maxTokens int) (string, error) {
	if maxTokens < 1 {
		maxTokens = 1500
	}
	payload, _ := json.Marshal(map[string]any{
		"model":      "claude-3-5-haiku-latest",
		"max_tokens": maxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	if sb.Len() == 0 {
		return "", fmt.Errorf("anthropic: empty response")
	}
	return strings.TrimSpace(sb.String()), nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// completeFull sends a separate system + user message to API providers.
// For local LLMs the system prompt is prepended to the user message.
func completeFull(ctx context.Context, ai AIOptions, system, user string, maxTokens int) (string, error) {
	switch strings.ToLower(ai.Provider) {
	case "api":
		if ai.AnthropicKey != "" {
			return completeSplitAnthropic(ctx, ai.AnthropicKey, system, user, maxTokens)
		}
		if p, ok := aiprovider.Select(aiAPIKeys(ai)); ok {
			return completeOpenAICompatibleSplit(ctx, p.APIKey, p.BaseURL, p.Model, system, user)
		}
		return "", fmt.Errorf("AI backend is API Keys but no provider key is set")
	default:
		client := localllm.NewClient(ai.LocalURL)
		if err := client.Ping(ctx); err != nil {
			return "", err
		}
		return client.GenerateJSON(ctx, ai.LocalModel, system+"\n\n"+user)
	}
}

func completeSplitAnthropic(ctx context.Context, apiKey, system, user string, maxTokens int) (string, error) {
	if maxTokens < 1 {
		maxTokens = 4096
	}
	payload, _ := json.Marshal(map[string]any{
		"model":      "claude-3-5-haiku-latest",
		"max_tokens": maxTokens,
		"system":     system,
		"messages": []map[string]string{
			{"role": "user", "content": user},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	if sb.Len() == 0 {
		return "", fmt.Errorf("anthropic: empty response")
	}
	return strings.TrimSpace(sb.String()), nil
}

// aiAPIKeys maps the AIOptions credential fields onto the aiprovider.Keys
// shape so the shared provider registry can select the active OpenAI-compatible
// endpoint. Anthropic is handled separately (native message format).
func aiAPIKeys(ai AIOptions) aiprovider.Keys {
	return aiprovider.Keys{
		OpenAI:     ai.OpenAIKey,
		Google:     ai.GoogleKey,
		DeepSeek:   ai.DeepSeekKey,
		Groq:       ai.GroqKey,
		Mistral:    ai.MistralKey,
		Together:   ai.TogetherKey,
		OpenRouter: ai.OpenRouterKey,
		XAI:        ai.XAIKey,
	}
}

// completeOpenAICompatibleSplit posts a system+user chat-completions request to
// any OpenAI-compatible endpoint (OpenAI, Google Gemini, DeepSeek, Groq, Mistral,
// Together, OpenRouter, xAI). baseURL is the versioned root, e.g.
// "https://api.groq.com/openai/v1"; the /chat/completions path is appended.
func completeOpenAICompatibleSplit(ctx context.Context, apiKey, baseURL, model, system, user string) (string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"
	payload, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature":     0.1,
		"response_format": map[string]string{"type": "json_object"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai-compatible HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("openai-compatible: empty response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
