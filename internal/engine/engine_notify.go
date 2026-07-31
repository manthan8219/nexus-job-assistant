package engine

// Package engine — engine_notify.go
// Notifier wiring and event emission: building the MultiNotifier from config,
// and the non-blocking senders used to stream logs/results/progress to the UI.

import (
	"fmt"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
)

// RebuildNotifier re-reads notification credentials from cfg and replaces the
// current MultiNotifier. Call this whenever the config is saved so new or
// removed channels take effect immediately without restarting.
func (e *Engine) RebuildNotifier(cfg *config.Config) {
	e.Notifier = notifierFromCfg(cfg)
}

// notifierFromCfg is the single wiring point between config and notifier.
// All channels are built here via notifier.FromConfig.
func notifierFromCfg(cfg *config.Config) notifier.MultiNotifier {
	discordURL, tgToken, tgChatID, channels := cfg.NotifyFields()
	return notifier.FromConfig(&notifier.NotifyConfig{
		DiscordWebhookURL: discordURL,
		TelegramBotToken:  tgToken,
		TelegramChatID:    tgChatID,
		EnabledChannels:   channels,
	})
}

// log appends a formatted line to the run log (→ Logs tab), dropping it when
// the UI is lagging rather than blocking the apply pipeline.
func (e *Engine) log(format string, args ...any) {
	defer func() { recover() }() // guard against send on closed channel after stop
	msg := fmt.Sprintf(format, args...)
	select {
	case e.LogCh <- msg:
	default:
	}
}

// sendProgress emits per-provider search progress (→ Dashboard), non-blocking.
func (e *Engine) sendProgress(p ProviderProgress) {
	defer func() { recover() }() // guard against send on closed channel after stop
	select {
	case e.ProgressCh <- p:
	default:
	}
}

// sendResult emits one per-job result, dropping it when the UI is lagging.
func (e *Engine) sendResult(r Result) {
	defer func() { recover() }() // guard against send on closed channel after stop
	select {
	case e.ResultCh <- r:
	default:
		// UI lagging — drop rather than stall the apply pipeline
	}
}
