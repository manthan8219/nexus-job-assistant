package aiprovider

import "testing"

func TestSelect(t *testing.T) {
	tests := []struct {
		name string
		keys Keys
		want Provider
		ok   bool
	}{
		{
			"no keys set returns false",
			Keys{},
			Provider{},
			false,
		},
		{
			"openai wins by precedence over all others",
			Keys{OpenAI: "sk-openai", Google: "gem", Groq: "gq"},
			Provider{Name: "openai", BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini", APIKey: "sk-openai"},
			true,
		},
		{
			"google selected when openai empty",
			Keys{Google: "gem-key", Groq: "gq"},
			Provider{Name: "google", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai", Model: "gemini-2.0-flash", APIKey: "gem-key"},
			true,
		},
		{
			"deepseek selected when openai and google empty",
			Keys{DeepSeek: "ds-key"},
			Provider{Name: "deepseek", BaseURL: "https://api.deepseek.com", Model: "deepseek-chat", APIKey: "ds-key"},
			true,
		},
		{
			"groq precedence after deepseek",
			Keys{Groq: "gq-key", Mistral: "mi"},
			Provider{Name: "groq", BaseURL: "https://api.groq.com/openai/v1", Model: "llama-3.3-70b-versatile", APIKey: "gq-key"},
			true,
		},
		{
			"mistral selected alone",
			Keys{Mistral: "mi-key"},
			Provider{Name: "mistral", BaseURL: "https://api.mistral.ai/v1", Model: "mistral-small-latest", APIKey: "mi-key"},
			true,
		},
		{
			"together selected alone",
			Keys{Together: "tg-key"},
			Provider{Name: "together", BaseURL: "https://api.together.xyz/v1", Model: "meta-llama/Llama-3.3-70B-Instruct-Turbo", APIKey: "tg-key"},
			true,
		},
		{
			"openrouter selected alone",
			Keys{OpenRouter: "or-key"},
			Provider{Name: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Model: "meta-llama/llama-3.3-70b-instruct", APIKey: "or-key"},
			true,
		},
		{
			"xai is last in precedence",
			Keys{XAI: "xai-key"},
			Provider{Name: "xai", BaseURL: "https://api.x.ai/v1", Model: "grok-2-latest", APIKey: "xai-key"},
			true,
		},
		{
			"first non-empty key in precedence wins",
			Keys{OpenAI: "sk-openai", Google: "gem"},
			Provider{Name: "openai", BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini", APIKey: "sk-openai"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Select(tt.keys)
			if ok != tt.ok {
				t.Fatalf("Select ok = %v; want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("Select = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestProvidersPrecedenceOrder(t *testing.T) {
	// Guard the precedence order against accidental reorder: every provider
	// must be checked before the next one in the slice.
	wantOrder := []string{"openai", "google", "deepseek", "groq", "mistral", "together", "openrouter", "xai"}
	if len(providers) != len(wantOrder) {
		t.Fatalf("providers has %d entries; want %d", len(providers), len(wantOrder))
	}
	for i, p := range providers {
		if p.Name != wantOrder[i] {
			t.Errorf("providers[%d].Name = %q; want %q", i, p.Name, wantOrder[i])
		}
		if p.BaseURL == "" {
			t.Errorf("providers[%d] (%q) has empty BaseURL", i, p.Name)
		}
		if p.Model == "" {
			t.Errorf("providers[%d] (%q) has empty Model", i, p.Name)
		}
	}
}
