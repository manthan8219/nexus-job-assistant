package outreach

import (
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// Run modes for automated outreach.
const (
	ModeConfirm = "confirm" // prepare queue; ask y/n before each action
	ModeQueue   = "queue"   // prepare queue; each Enter fires the next item
	ModeAuto    = "auto"    // run the whole queue with delays
)

// NormalizeMode maps current + legacy values to confirm|queue|auto.
func NormalizeMode(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case ModeAuto, "autosend", "fully_auto", "full":
		return ModeAuto
	case ModeQueue, "onetap", "batch", "tap":
		return ModeQueue
	case ModeConfirm, "assisted", "manual", "":
		return ModeConfirm
	default:
		return ModeConfirm
	}
}

// EffectiveMode reads OutreachMode, falling back to LinkedInMode for older configs.
func EffectiveMode(cfg *config.Config) string {
	if cfg == nil {
		return ModeConfirm
	}
	if strings.TrimSpace(cfg.OutreachMode) != "" {
		return NormalizeMode(cfg.OutreachMode)
	}
	return NormalizeMode(cfg.LinkedInMode)
}

func ModeLabel(mode string) string {
	switch NormalizeMode(mode) {
	case ModeAuto:
		return "Auto — send/open the whole queue (with daily caps)"
	case ModeQueue:
		return "Queue — generate many, tap Enter to fire each one"
	default:
		return "Confirm — ask before every email / browser open"
	}
}

func CycleMode(mode string) string {
	switch NormalizeMode(mode) {
	case ModeConfirm:
		return ModeQueue
	case ModeQueue:
		return ModeAuto
	default:
		return ModeConfirm
	}
}
