// Package aiprovider is the single source of truth for OpenAI-compatible LLM
// API providers: their base URL, a sane default model, and the precedence order
// used to pick one from the keys a user has configured.
//
// Anthropic is intentionally NOT modelled here — it has its own (non
// OpenAI-compatible) message format and is handled natively by the resume
// completion path and the eino claude client. Callers check the Anthropic key
// first, then fall back to Select for any OpenAI-compatible provider.
package aiprovider

// Provider describes one OpenAI-compatible chat-completions endpoint.
type Provider struct {
	Name    string // human-facing identifier, e.g. "google"
	BaseURL string // versioned root, e.g. "https://api.groq.com/openai/v1"
	Model   string // default model id for chat completions
	APIKey  string // filled in by Select from the matching key
}

// Keys carries the API key for every OpenAI-compatible provider. Callers build
// it from whichever config/options struct they hold (config.Config or
// resume.AIOptions) so this package stays free of those dependencies.
type Keys struct {
	OpenAI     string
	Google     string
	DeepSeek   string
	Groq       string
	Mistral    string
	Together   string
	OpenRouter string
	XAI        string
}

// providers lists every OpenAI-compatible provider in precedence order: the
// first entry whose key is set wins. Base URLs are the official
// OpenAI-compatible chat-completions roots (verified against each provider's
// docs). Default models follow the same "cheap, fast, stable" budget the
// built-in OpenAI/Anthropic defaults use (gpt-4o-mini / claude-3-5-haiku).
var providers = []Provider{
	{Name: "openai", BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini"},
	{Name: "google", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai", Model: "gemini-2.5-flash"},
	{Name: "deepseek", BaseURL: "https://api.deepseek.com", Model: "deepseek-chat"},
	{Name: "groq", BaseURL: "https://api.groq.com/openai/v1", Model: "llama-3.3-70b-versatile"},
	{Name: "mistral", BaseURL: "https://api.mistral.ai/v1", Model: "mistral-small-latest"},
	{Name: "together", BaseURL: "https://api.together.xyz/v1", Model: "meta-llama/Llama-3.3-70B-Instruct-Turbo"},
	{Name: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Model: "meta-llama/llama-3.3-70b-instruct"},
	{Name: "xai", BaseURL: "https://api.x.ai/v1", Model: "grok-2-latest"},
}

// Select returns the first OpenAI-compatible provider (by precedence) whose
// API key is set in k, with that key filled into the returned Provider. When
// no key is set it returns ok=false.
func Select(k Keys) (Provider, bool) {
	keys := []string{k.OpenAI, k.Google, k.DeepSeek, k.Groq, k.Mistral, k.Together, k.OpenRouter, k.XAI}
	for i, p := range providers {
		if keys[i] != "" {
			p.APIKey = keys[i]
			return p, true
		}
	}
	return Provider{}, false
}
