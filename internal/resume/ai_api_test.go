package resume

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// reqSnapshot captures the observable parts of a request while the handler
// owns the body, so assertions stay race-free.
type reqSnapshot struct {
	Path string
	Auth string
	Body string
}

// openAICompatServer returns an httptest server that captures the request and
// replies with the given status/body, modeling an OpenAI-compatible endpoint.
// The returned getter reads the snapshot captured inside the handler.
func openAICompatServer(t *testing.T, status int, body string) (*httptest.Server, func() reqSnapshot) {
	t.Helper()
	var snap reqSnapshot
	var once bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		snap = reqSnapshot{Path: r.URL.Path, Auth: r.Header.Get("Authorization"), Body: string(buf)}
		once = true
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, func() reqSnapshot {
		if !once {
			return reqSnapshot{}
		}
		return snap
	}
}

func TestCompleteOpenAICompatibleSplit_HappyPath(t *testing.T) {
	srv, gotReq := openAICompatServer(t, http.StatusOK,
		`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`)
	out, err := completeOpenAICompatibleSplit(context.Background(), "sk-test", srv.URL+"/v1", "gpt-4o-mini", "sys", "user")
	if err != nil {
		t.Fatalf("completeOpenAICompatibleSplit: %v", err)
	}
	if out != `{"ok":true}` {
		t.Errorf("out = %q; want %q", out, `{"ok":true}`)
	}
	// The request must hit the /v1/chat/completions endpoint with the model and
	// the bearer key, and keep the system+user messages.
	got := gotReq()
	if got.Path == "" {
		t.Fatal("server did not receive a request")
	}
	if !strings.HasSuffix(got.Path, "/v1/chat/completions") {
		t.Errorf("path = %q; want suffix /v1/chat/completions", got.Path)
	}
	if got.Auth != "Bearer sk-test" {
		t.Errorf("Authorization = %q; want Bearer sk-test", got.Auth)
	}
	var payload struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(got.Body), &payload); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if payload.Model != "gpt-4o-mini" {
		t.Errorf("model = %q; want gpt-4o-mini", payload.Model)
	}
	if len(payload.Messages) != 2 || payload.Messages[0].Role != "system" || payload.Messages[1].Content != "user" {
		t.Errorf("messages = %+v; want [system sys, user user]", payload.Messages)
	}
}

func TestCompleteOpenAICompatibleSplit_Non200(t *testing.T) {
	srv, _ := openAICompatServer(t, http.StatusBadGateway, `{"error":"boom"}`)
	_, err := completeOpenAICompatibleSplit(context.Background(), "k", srv.URL, "m", "s", "u")
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("err = %v; want HTTP 502 error", err)
	}
}

func TestCompleteOpenAICompatibleSplit_MalformedJSON(t *testing.T) {
	srv, _ := openAICompatServer(t, http.StatusOK, `{not json`)
	if _, err := completeOpenAICompatibleSplit(context.Background(), "k", srv.URL, "m", "s", "u"); err == nil {
		t.Fatal("expected error for malformed JSON body")
	}
}

func TestCompleteOpenAICompatibleSplit_EmptyChoices(t *testing.T) {
	srv, _ := openAICompatServer(t, http.StatusOK, `{"choices":[]}`)
	_, err := completeOpenAICompatibleSplit(context.Background(), "k", srv.URL, "m", "s", "u")
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("err = %v; want empty response error", err)
	}
}

func TestCompleteOpenAICompatibleSplit_CancelledContext(t *testing.T) {
	srv, _ := openAICompatServer(t, http.StatusOK, `{"choices":[{"message":{"content":"x"}}]}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := completeOpenAICompatibleSplit(ctx, "k", srv.URL, "m", "s", "u"); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestCompleteOpenAICompatible_SinglePrompt(t *testing.T) {
	srv, gotReq := openAICompatServer(t, http.StatusOK, `{"choices":[{"message":{"content":"hi"}}]}`)
	out, err := completeOpenAICompatible(context.Background(), "k", srv.URL, "m", "prompt")
	if err != nil {
		t.Fatalf("completeOpenAICompatible: %v", err)
	}
	if out != "hi" {
		t.Errorf("out = %q; want hi", out)
	}
	if gotReq().Path == "" {
		t.Fatal("server did not receive a request")
	}
}

func TestAIAPIKeys_MapsAllFields(t *testing.T) {
	ai := AIOptions{
		OpenAIKey:       "o",
		GoogleKey:       "g",
		DeepSeekKey:     "d",
		GroqKey:         "gr",
		MistralKey:      "m",
		TogetherKey:     "t",
		OpenRouterKey:   "or",
		XAIKey:          "x",
		OpenAIModel:     "gpt-4o",
		GoogleModel:     "gemini-2.5-pro",
		DeepSeekModel:   "deepseek-r1",
		GroqModel:       "llama-3.3",
		MistralModel:    "mistral-large",
		TogetherModel:   "together-model",
		OpenRouterModel: "or-model",
		XAIModel:        "grok-2",
	}
	got := aiAPIKeys(ai)
	if got.OpenAI != "o" || got.Google != "g" || got.XAI != "x" {
		t.Errorf("aiAPIKeys keys = %+v; want o/g/.../x", got)
	}
	wantModels := map[string]string{
		"openai": "gpt-4o", "google": "gemini-2.5-pro", "deepseek": "deepseek-r1",
		"groq": "llama-3.3", "mistral": "mistral-large", "together": "together-model",
		"openrouter": "or-model", "xai": "grok-2",
	}
	for k, want := range wantModels {
		if got.Models[k] != want {
			t.Errorf("Models[%q] = %q; want %q", k, got.Models[k], want)
		}
	}
}

func TestAIAPIKeys_EmptyWithZeroOptions(t *testing.T) {
	got := aiAPIKeys(AIOptions{})
	if got.OpenAI != "" || got.Google != "" || got.XAI != "" {
		t.Errorf("aiAPIKeys(zero) keys = %+v; want empty", got)
	}
	if len(got.Models) != 0 {
		t.Errorf("aiAPIKeys(zero).Models = %v; want empty", got.Models)
	}
}
