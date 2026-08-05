package api

import (
	"net/http"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
)

// notifiersFromConfig builds the configured notification channel set for a
// config. Used by New (legacy wiring) and by the notify endpoints per request,
// so each tenant's own channels/credentials are used in multi-tenant mode.
func notifiersFromConfig(cfg *config.Config) notifier.MultiNotifier {
	if cfg == nil {
		return nil
	}
	discordURL, tgToken, tgChatID, channels := cfg.NotifyFields()
	return notifier.FromConfig(&notifier.NotifyConfig{
		DiscordWebhookURL:  discordURL,
		TelegramBotToken:   tgToken,
		TelegramChatID:     tgChatID,
		EnabledChannels:    channels,
		Email:              cfg.Email,
		GmailAppPassword:   cfg.GmailAppPassword,
		EmailNotifications: cfg.EmailNotifications,
		EmailPerJob:        cfg.EmailPerJob,
	})
}

// NotifierChannel describes a discoverable notification integration.
type NotifierChannel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// handleGetNotifyChannels returns available notification channels.
func (s *Server) handleGetNotifyChannels(w http.ResponseWriter, r *http.Request) {
	avail := notifier.Available()
	enabled := s.cfgFor(r).NotifyChannels
	enabledSet := make(map[string]bool, len(enabled))
	for _, ch := range enabled {
		enabledSet[ch] = true
	}

	channels := make([]NotifierChannel, 0, len(avail))
	for _, ch := range avail {
		channels = append(channels, NotifierChannel{
			ID:      ch.ID,
			Name:    ch.DisplayName,
			Enabled: enabledSet[ch.ID],
		})
	}
	writeJSON(w, http.StatusOK, channels)
}
