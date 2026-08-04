// Command test-email sends a single outreach email through the user's
// configured Gmail (SMTP app password, or the Gmail API OAuth path when a
// token is configured) using the real outreach pipeline entry point. It is a
// thin CLI over internal/outreach.SendEmail — no business logic lives here.
//
// It respects the same consent gate and daily caps as the TUI, and records the
// item in the outreach store so reply-checking can match it later.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
)

func main() {
	to := flag.String("to", "", "recipient email address (required)")
	subject := flag.String("subject", "", "subject line (default: Hello)")
	body := flag.String("body", "", "message body")
	company := flag.String("company", "GrowthRadar", "company name recorded on the item")
	role := flag.String("role", "Outreach Test", "role recorded on the item")
	url := flag.String("url", "", "optional job URL recorded on the item")
	flag.Parse()

	if strings.TrimSpace(*to) == "" {
		fmt.Fprintln(os.Stderr, "usage: test-email -to someone@example.com [-subject ...] [-body ...]")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}

	item := outreach.Item{
		Channel:      outreach.ChannelEmail,
		Company:      *company,
		Role:         *role,
		JobURL:       *url,
		ContactEmail: *to,
		Subject:      *subject,
		Body:         *body,
	}

	if err := outreach.SendEmail(cfg, item); err != nil {
		fmt.Fprintln(os.Stderr, "send failed:", err)
		os.Exit(1)
	}
	fmt.Printf("✓ sent to %s from %s\n", *to, cfg.Email)
}
