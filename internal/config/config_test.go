package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestDir_Path(t *testing.T) {
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(dir), "/.nexus") {
		t.Errorf("Dir() = %q; want suffix /.nexus", dir)
	}
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(p), "/.nexus/config.json") {
		t.Errorf("Path() = %q; want suffix /.nexus/config.json", p)
	}
}

func TestLoadFrom_MissingFileReturnsEmpty(t *testing.T) {
	// A path that does not exist must yield a usable zero Config, not an error —
	// so first-run users start fresh instead of seeing a setup failure.
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("LoadFrom missing file: %v; want nil", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil Config")
	}
	if cfg.FirstName != "" || cfg.ApplyConsent {
		t.Errorf("expected zero Config; got firstName=%q consent=%v", cfg.FirstName, cfg.ApplyConsent)
	}
}

func TestLoadFrom_MalformedJSONErrors(t *testing.T) {
	_, err := LoadFrom(writeFile(t, "{not json"))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLoadFrom_Table(t *testing.T) {
	tests := []struct {
		name string
		body string
		want func(*Config) bool
	}{
		{
			"old minimal config loads",
			`{"first_name":"Ada","email":"a@b.com"}`,
			func(c *Config) bool { return c.FirstName == "Ada" && c.Email == "a@b.com" },
		},
		{
			"unknown future fields ignored (forward compat)",
			`{"first_name":"Ada","future_field":123,"nested":{"x":1}}`,
			func(c *Config) bool { return c.FirstName == "Ada" },
		},
		{
			"consent + rate limits preserved",
			`{"apply_consent":true,"max_apps_per_run":5,"max_apps_per_day":25,"apply_delay_sec":3}`,
			func(c *Config) bool {
				return c.ApplyConsent && c.MaxAppsPerRun == 5 && c.MaxAppsPerDay == 25 && c.ApplyDelaySec == 3
			},
		},
		{
			"missing optional fields stay zero",
			`{"first_name":"Ada"}`,
			func(c *Config) bool { return c.DiscordWebhookURL == "" && !c.AIAssist && c.MaxAppsPerRun == 0 },
		},
		{
			"secrets round-trip",
			`{"discord_webhook_url":"https://discord.com/api/webhooks/x","anthropic_key":"sk-x"}`,
			func(c *Config) bool {
				return c.DiscordWebhookURL == "https://discord.com/api/webhooks/x" && c.AnthropicKey == "sk-x"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadFrom(writeFile(t, tt.body))
			if err != nil {
				t.Fatalf("LoadFrom: %v", err)
			}
			if !tt.want(cfg) {
				t.Errorf("assertion failed for %q; config=%+v", tt.name, cfg)
			}
		})
	}
}

// TestConfig_JSONRoundTrip confirms the JSON tags round-trip a full Config
// (additive schema: a serialized config reloads to the same values). Uses
// json.Marshal/Unmarshal directly so no ~/.nexus write is needed.
func TestConfig_JSONRoundTrip(t *testing.T) {
	orig := &Config{
		FirstName: "Grace", LastName: "Hopper", Email: "grace@navy.mil",
		WorkType: "Remote", TargetLocations: "Remote",
		AIAssist: true, AIProvider: "api", AnthropicKey: "sk-test", GoogleKey: "gem-test", GroqKey: "gq-test",
		ApplyConsent: true, MaxAppsPerRun: 7, MaxAppsPerDay: 20, ApplyDelaySec: 4,
		DiscordWebhookURL: "https://discord.com/api/webhooks/z",
	}
	data, err := json.MarshalIndent(orig, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Config
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Spot-check across personal, AI, safety, and notifier fields.
	if got.FirstName != orig.FirstName || got.Email != orig.Email {
		t.Errorf("personal fields lost: %+v", got)
	}
	if got.AIAssist != orig.AIAssist || got.AIProvider != orig.AIProvider || got.AnthropicKey != orig.AnthropicKey {
		t.Errorf("AI fields lost: %+v", got)
	}
	if got.GoogleKey != orig.GoogleKey || got.GroqKey != orig.GroqKey {
		t.Errorf("new AI provider keys lost: %+v", got)
	}
	if got.ApplyConsent != orig.ApplyConsent || got.MaxAppsPerRun != orig.MaxAppsPerRun ||
		got.MaxAppsPerDay != orig.MaxAppsPerDay || got.ApplyDelaySec != orig.ApplyDelaySec {
		t.Errorf("safety fields lost: %+v", got)
	}
	if got.DiscordWebhookURL != orig.DiscordWebhookURL {
		t.Errorf("notifier fields lost: %+v", got)
	}
}

func TestNotifyFields(t *testing.T) {
	c := &Config{
		DiscordWebhookURL: "dwh", TelegramBotToken: "tgt", TelegramChatID: "123",
		NotifyChannels: []string{"discord"},
	}
	dwh, tgt, chat, ch := c.NotifyFields()
	if dwh != "dwh" || tgt != "tgt" || chat != "123" || len(ch) != 1 || ch[0] != "discord" {
		t.Errorf("NotifyFields mismatch: %q %q %q %v", dwh, tgt, chat, ch)
	}
}
