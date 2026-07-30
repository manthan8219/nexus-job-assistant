package outreach

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// This file implements sending through the Gmail API with the user's OAuth
// token (refresh token → short-lived access token → users.messages.send).
// It uses only the standard library. A refresh token is obtained once with
// the cmd/gmailauth helper. When configured, it takes precedence over the
// SMTP app-password path — both send from the user's own Gmail address.

const (
	googleTokenURL = "https://oauth2.googleapis.com/token"
	gmailSendURL   = "https://gmail.googleapis.com/gmail/v1/users/me/messages/send"
	// GmailSendScope is the only scope Nexus requests.
	GmailSendScope = "https://www.googleapis.com/auth/gmail.send"
)

// HasGmailOAuth reports whether a full OAuth token configuration is present.
func HasGmailOAuth(cfg *config.Config) bool {
	return cfg != nil &&
		strings.TrimSpace(cfg.GmailOAuthRefreshToken) != "" &&
		strings.TrimSpace(cfg.GmailOAuthClientID) != "" &&
		strings.TrimSpace(cfg.GmailOAuthClientSecret) != ""
}

// refreshAccessToken exchanges the stored refresh token for an access token.
func refreshAccessToken(ctx context.Context, cfg *config.Config) (string, error) {
	form := url.Values{
		"client_id":     {strings.TrimSpace(cfg.GmailOAuthClientID)},
		"client_secret": {strings.TrimSpace(cfg.GmailOAuthClientSecret)},
		"refresh_token": {strings.TrimSpace(cfg.GmailOAuthRefreshToken)},
		"grant_type":    {"refresh_token"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("google token refresh: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google token refresh HTTP %d: %s — re-run nexus-gmailauth to get a fresh token", resp.StatusCode, truncateStr(string(body), 200))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("google token refresh decode: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("google token refresh: empty access token (%s)", tok.Error)
	}
	return tok.AccessToken, nil
}

// sendViaGmailAPI sends a raw RFC 822 message through the Gmail API.
func sendViaGmailAPI(ctx context.Context, cfg *config.Config, rawMessage string) error {
	access, err := refreshAccessToken(ctx, cfg)
	if err != nil {
		return err
	}
	raw := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString([]byte(rawMessage))
	payload, _ := json.Marshal(map[string]string{"raw": raw})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gmailSendURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("gmail send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		return fmt.Errorf("gmail send HTTP %d: %s", resp.StatusCode, truncateStr(string(body), 200))
	}
	return nil
}

// buildRFC822 renders a plain-text email with proper header encoding.
func buildRFC822(from, to, subject, body string) string {
	var msg strings.Builder
	msg.WriteString("From: " + from + "\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: " + encodeHeader(subject) + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)
	return msg.String()
}

// encodeHeader makes non-ASCII subjects safe for RFC 822 transport.
func encodeHeader(s string) string {
	ascii := true
	for _, r := range s {
		if r > 127 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	return mime.QEncoding.Encode("utf-8", s)
}

func truncateStr(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
