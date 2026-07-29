package notifier

import "testing"

func TestAvailable_ReturnsRegisteredChannels(t *testing.T) {
	got := Available()
	if len(got) < 2 {
		t.Fatalf("expected at least discord+telegram, got %d", len(got))
	}
	ids := map[string]bool{}
	for _, ch := range got {
		ids[ch.ID] = true
		if ch.DisplayName == "" {
			t.Errorf("channel %q missing DisplayName", ch.ID)
		}
		if ch.Configured == nil || ch.Build == nil {
			t.Errorf("channel %q missing Configured/Build", ch.ID)
		}
	}
	for _, want := range []string{"discord", "telegram"} {
		if !ids[want] {
			t.Errorf("missing channel %q in Available()", want)
		}
	}
}

func TestFromConfig_UsesRegistry(t *testing.T) {
	cfg := &NotifyConfig{
		DiscordWebhookURL: "https://discord.com/api/webhooks/x",
		TelegramBotToken:  "token",
		TelegramChatID:    "123",
		EnabledChannels:   []string{"discord", "telegram"},
	}
	mn := FromConfig(cfg)
	if len(mn) != 2 {
		t.Fatalf("want 2 notifiers, got %d", len(mn))
	}
	names := map[string]bool{}
	for _, n := range mn {
		names[n.Name()] = true
	}
	if !names["discord"] || !names["telegram"] {
		t.Errorf("unexpected notifiers: %v", names)
	}
}

func TestFromConfig_RespectsEnabledChannels(t *testing.T) {
	cfg := &NotifyConfig{
		DiscordWebhookURL: "https://discord.com/api/webhooks/x",
		TelegramBotToken:  "token",
		TelegramChatID:    "123",
		EnabledChannels:   []string{"telegram"},
	}
	mn := FromConfig(cfg)
	if len(mn) != 1 || mn[0].Name() != "telegram" {
		t.Fatalf("want only telegram, got %#v", mn)
	}
}

func TestFromConfig_SkipsUnconfigured(t *testing.T) {
	cfg := &NotifyConfig{
		EnabledChannels: []string{"discord", "telegram"},
	}
	mn := FromConfig(cfg)
	if len(mn) != 0 {
		t.Fatalf("want empty MultiNotifier, got %d", len(mn))
	}
}
