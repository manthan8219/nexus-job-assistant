package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func init() {
	Register(Channel{
		ID:          "telegram",
		DisplayName: "Telegram",
		WarnMsg:     "add bot token + chat ID above",
		Configured: func(c *NotifyConfig) bool {
			return c.TelegramBotToken != "" && c.TelegramChatID != ""
		},
		Build: func(c *NotifyConfig) Notifier {
			return NewTelegramNotifier(c.TelegramBotToken, c.TelegramChatID)
		},
	})
}

// TelegramNotifier sends notifications via the Telegram Bot API.
// Requires a bot token (from @BotFather) and a chat ID.
type TelegramNotifier struct {
	botToken string
	chatID   string
	client   *http.Client
}

// NewTelegramNotifier creates a Telegram notifier.
// Empty token or chatID creates a no-op notifier.
func NewTelegramNotifier(botToken, chatID string) *TelegramNotifier {
	if botToken == "" || chatID == "" {
		return &TelegramNotifier{}
	}
	return &TelegramNotifier{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

// Name returns "telegram".
func (t *TelegramNotifier) Name() string { return "telegram" }

// Send delivers an event as a Telegram message (HTML parse mode).
func (t *TelegramNotifier) Send(ctx context.Context, ev Event) error {
	if t.botToken == "" || t.chatID == "" {
		return nil
	}

	text := t.buildMessage(ev)
	if text == "" {
		return nil
	}

	payload := map[string]string{
		"chat_id":    t.chatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram marshal: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram send: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("telegram read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	// Telegram can return HTTP 200 with ok:false
	var apiResp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(bodyBytes, &apiResp); err == nil && !apiResp.OK {
		return fmt.Errorf("telegram API: %s", apiResp.Description)
	}
	return nil
}

func (t *TelegramNotifier) buildMessage(ev Event) string {
	switch ev.Kind {
	case EventJobApplied:
		msg := fmt.Sprintf("✅ <b>Application Submitted</b>\n<b>%s</b> @ <b>%s</b>", ev.JobTitle, ev.Company)
		if ev.Location != "" {
			msg += "\n📍 " + ev.Location
		}
		if ev.Provider != "" {
			msg += "\n🔧 " + ev.Provider
		}
		return msg

	case EventJobFailed:
		msg := fmt.Sprintf("❌ <b>Application Failed</b>\n<b>%s</b> @ <b>%s</b>", ev.JobTitle, ev.Company)
		if ev.Reason != "" {
			msg += "\n<b>Reason:</b> " + ev.Reason
		}
		return msg

	case EventCAPTCHA:
		msg := "🤖 <b>CAPTCHA Detected</b>\nA CAPTCHA was encountered during automation."
		if ev.CAPTCHAURL != "" {
			msg += "\n<b>URL:</b> " + ev.CAPTCHAURL
		}
		return msg

	case EventError:
		msg := ev.Message
		if msg == "" {
			msg = "An unexpected error occurred."
		}
		return "🚨 <b>Error</b>\n" + msg

	case EventRunStarted:
		msg := "⚡ <b>Run Started</b>\nJob application run has begun."
		if ev.Message != "" {
			msg = "⚡ <b>Run Started</b>\n" + ev.Message
		}
		return msg

	case EventRunComplete:
		msg := fmt.Sprintf(
			"✅ <b>Run Complete</b>\n<b>Applied:</b> %d\n<b>Failed:</b> %d\n<b>Skipped:</b> %d",
			ev.TotalApplied, ev.TotalFailed, ev.TotalSkipped,
		)
		if ev.RunDuration > 0 {
			msg += "\n<b>Duration:</b> " + FormatDuration(ev.RunDuration)
		}
		return msg

	case EventDailySummary:
		return fmt.Sprintf(
			"📊 <b>Daily Summary</b>\n<b>Applied:</b> %d\n<b>Failed:</b> %d\n<b>Skipped:</b> %d",
			ev.TotalApplied, ev.TotalFailed, ev.TotalSkipped,
		)

	case EventWeeklySummary:
		return fmt.Sprintf(
			"📈 <b>Weekly Summary</b>\n<b>Applied:</b> %d\n<b>Failed:</b> %d\n<b>Skipped:</b> %d",
			ev.TotalApplied, ev.TotalFailed, ev.TotalSkipped,
		)

	case EventCustom:
		title := ev.Title
		if title == "" {
			title = "ℹ️ Notification"
		}
		msg := fmt.Sprintf("<b>%s</b>", htmlEscape(title))
		if ev.Message != "" {
			msg += "\n" + htmlEscape(ev.Message)
		}
		for k, v := range ev.Fields {
			msg += fmt.Sprintf("\n<b>%s:</b> %s", htmlEscape(k), htmlEscape(v))
		}
		return msg

	default:
		if ev.Message != "" {
			return ev.Message
		}
		return ""
	}
}

// htmlEscape escapes characters that break Telegram HTML parse mode.
func htmlEscape(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(s)
}
