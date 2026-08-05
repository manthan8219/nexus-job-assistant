package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/aiprovider"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// AIModelsResponse is the list of model IDs a provider exposes.
type AIModelsResponse struct {
	Provider string   `json:"provider"`
	Models   []string `json:"models"`
}

// handleGetAIModels lists the chat models available to an API provider's key.
// POST /api/ai/models with {"provider":"google","key":"AIza..."}; key is
// optional and falls back to the stored config key for that provider.
func (s *Server) handleGetAIModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
		Key      string `json:"key"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	provider := strings.ToLower(strings.TrimSpace(body.Provider))
	if provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}
	key, baseURL := aiProviderCreds(s.cfgFor(r), provider)
	if trimmed := strings.TrimSpace(body.Key); trimmed != "" {
		key = trimmed
	}
	if key == "" {
		writeError(w, http.StatusBadRequest, "no API key configured for provider "+provider)
		return
	}
	models, err := listAIModels(r.Context(), provider, key, baseURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("%s models request failed: %v", provider, err))
		return
	}
	writeJSON(w, http.StatusOK, AIModelsResponse{Provider: provider, Models: models})
}

// aiProviderCreds returns the configured API key and base URL for a provider.
// Anthropic uses its native /v1/models endpoint; the rest are OpenAI-compatible.
func aiProviderCreds(cfg *config.Config, provider string) (key, baseURL string) {
	if cfg == nil {
		return "", ""
	}
	switch provider {
	case "anthropic":
		return cfg.AnthropicKey, "https://api.anthropic.com/v1"
	case "openai":
		return cfg.OpenAIKey, aiprovider.BaseURL("openai")
	case "google":
		return cfg.GoogleKey, aiprovider.BaseURL("google")
	case "deepseek":
		return cfg.DeepSeekKey, aiprovider.BaseURL("deepseek")
	case "groq":
		return cfg.GroqKey, aiprovider.BaseURL("groq")
	case "mistral":
		return cfg.MistralKey, aiprovider.BaseURL("mistral")
	case "together":
		return cfg.TogetherKey, aiprovider.BaseURL("together")
	case "openrouter":
		return cfg.OpenRouterKey, aiprovider.BaseURL("openrouter")
	case "xai":
		return cfg.XAIKey, aiprovider.BaseURL("xai")
	}
	return "", ""
}

// listAIModels fetches {baseURL}/models and returns the deduplicated model IDs.
// A leading "models/" prefix (Google's OpenAI-compat endpoint returns it) is
// stripped so IDs match what the chat-completions endpoint accepts.
func listAIModels(ctx context.Context, provider, key, baseURL string) ([]string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if provider == "anthropic" {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateBytes(body))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var ids []string
	for _, d := range out.Data {
		id := strings.TrimPrefix(d.ID, "models/")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

// truncateBytes shortens an error body for display (never logs secrets).
func truncateBytes(b []byte) string {
	const max = 200
	s := strings.TrimSpace(string(b))
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
