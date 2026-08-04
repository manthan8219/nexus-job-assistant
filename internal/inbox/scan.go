package inbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// DefaultScanDays is the lookback window used when inbox_scan_days is unset.
const DefaultScanDays = 45

// DefaultScanMax is the message cap used when inbox_scan_max is unset.
const DefaultScanMax = 200

// MessageSource provides inbox messages (headers + bodies) for scanning.
type MessageSource interface {
	FetchMessagesWithBodies(ctx context.Context, since time.Time, max int) ([]outreach.Reply, error)
}

// AppLister returns the applications used to link highlights to jobs.
type AppLister interface {
	List() ([]store.Application, error)
}

// genericDomains are senders whose domain carries no company signal.
var genericDomains = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "yahoo.com": true,
	"outlook.com": true, "hotmail.com": true, "live.com": true,
	"icloud.com": true, "me.com": true, "aol.com": true,
	"proton.me": true, "protonmail.com": true, "pm.me": true,
}

// rootDomain returns the lowercase sender domain without a leading "www.", or
// "" for malformed addresses.
func rootDomain(from string) string {
	i := strings.LastIndex(from, "@")
	if i < 0 || i+1 >= len(from) {
		return ""
	}
	d := strings.ToLower(strings.TrimSpace(from[i+1:]))
	return strings.TrimPrefix(d, "www.")
}

// companyInText reports whether a company name appears in free text.
func companyInText(company, text string) bool {
	name := strings.ToLower(strings.TrimSpace(company))
	if name == "" || len(name) < 3 {
		return false
	}
	return strings.Contains(strings.ToLower(text), name)
}

// link finds the application this highlight belongs to, if any.
func link(apps []store.Application, subject, body, domain string) (string, int64) {
	text := strings.ToLower(subject + " " + body)
	for _, a := range apps {
		if companyInText(a.Company, text) {
			return a.Company, a.ID
		}
	}
	// Fall back to the sender domain as a company label when it is meaningful.
	if domain != "" && !genericDomains[domain] {
		return domain, 0
	}
	return "", 0
}

// Scan fetches the inbox window, classifies hiring-related emails, links them
// to applications, and returns the new highlights. It does not persist so
// callers own saving via the store helpers.
func Scan(ctx context.Context, days, max int, src MessageSource, lister AppLister) ([]Highlight, error) {
	if days <= 0 {
		days = DefaultScanDays
	}
	if max <= 0 {
		max = DefaultScanMax
	}
	if src == nil {
		return nil, fmt.Errorf("inbox: no message source")
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	msgs, err := src.FetchMessagesWithBodies(ctx, since, max)
	if err != nil {
		return nil, fmt.Errorf("inbox: fetch: %w", err)
	}

	var apps []store.Application
	if lister != nil {
		if as, err := lister.List(); err == nil {
			apps = as
		}
	}

	var out []Highlight
	for _, m := range msgs {
		sig, conf := Classify(m.Subject, m.Body)
		if sig == SignalNone {
			continue
		}
		domain := rootDomain(m.From)
		company, appID := link(apps, m.Subject, m.Body, domain)
		preview := m.Body
		if len(preview) > 300 {
			preview = preview[:300]
		}
		out = append(out, Highlight{
			ID:          uuid.NewString(),
			MessageID:   m.MessageID,
			From:        m.From,
			FromName:    m.FromName,
			Subject:     m.Subject,
			BodyPreview: preview,
			Date:        m.Date,
			Signal:      sig,
			Confidence:  conf,
			Domain:      domain,
			Company:     company,
			AppID:       appID,
		})
	}
	return out, nil
}
