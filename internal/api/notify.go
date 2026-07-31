package api

import (
	"net/http"

	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
)

// NotifierChannel describes a discoverable notification integration.
type NotifierChannel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// handleGetNotifyChannels returns available notification channels.
func (s *Server) handleGetNotifyChannels(w http.ResponseWriter, r *http.Request) {
	avail := notifier.Available()
	enabled := s.cfg.NotifyChannels
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
