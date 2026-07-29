package outreach

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// LinkedInPeopleSearchURL opens LinkedIn people search for recruiters at a company.
func LinkedInPeopleSearchURL(company string) string {
	q := strings.TrimSpace(company) + ` recruiter OR "hiring manager" OR talent`
	return "https://www.linkedin.com/search/results/people/?keywords=" + url.QueryEscape(q)
}

// LinkedInMessagingURL opens the LinkedIn messaging inbox (user picks the thread).
func LinkedInMessagingURL() string {
	return "https://www.linkedin.com/messaging/"
}

// OpenBrowser opens url in the default browser (non-blocking).
func OpenBrowser(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("empty url")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}

// OpenLinkedInOutreach launches people search for the company (automation entry point).
func OpenLinkedInOutreach(company string) (string, error) {
	u := LinkedInPeopleSearchURL(company)
	if err := OpenBrowser(u); err != nil {
		return u, err
	}
	return u, nil
}
