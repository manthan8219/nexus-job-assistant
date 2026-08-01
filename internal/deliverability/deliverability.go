// Package deliverability audits a domain's email deliverability posture —
// SPF, DMARC and DKIM TXT records — and returns actionable guidance for
// fixing gaps so outreach mail lands in the inbox instead of spam.
//
// All DNS work goes through the injected TxtResolver (nil → net.DefaultResolver);
// tests substitute a fake so no real DNS is ever touched.
package deliverability

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// TxtResolver looks up TXT records for a name. net.DefaultResolver satisfies it.
type TxtResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// Result is one audit axis: SPF or DMARC.
type Result struct {
	// Present is true when a valid record of the expected type was found.
	Present bool `json:"present"`
	// Record is the raw DNS record that was found ("" when missing).
	Record string `json:"record,omitempty"`
	// Verdict is a short human label: "pass", "softfail", "reject",
	// "quarantine", "none", "missing", "unknown", ...
	Verdict string `json:"verdict"`
	// Guidance tells the user exactly which DNS record to add or fix.
	Guidance string `json:"guidance"`
}

// DKIMResult reports which common selectors resolve for the domain.
type DKIMResult struct {
	// Found is true when at least one common selector has a key published.
	Found bool `json:"found"`
	// Selectors lists the selectors that resolved (empty when none).
	Selectors []string `json:"selectors,omitempty"`
	// Guidance tells the user what to add when nothing resolves.
	Guidance string `json:"guidance"`
}

// Report is the complete audit for one domain.
type Report struct {
	Domain  string     `json:"domain"`
	SPF     Result     `json:"spf"`
	DMARC   Result     `json:"dmarc"`
	DKIM    DKIMResult `json:"dkim"`
	Summary string     `json:"summary"`
}

// dkimSelectors is a small, bounded set of the most common selector names so a
// single audit probes a handful of TXT records, never the whole zone.
var dkimSelectors = []string{
	"google", "k1", "default", "selector1", "selector2", "s1", "s2", "mail", "protonmail",
}

// Audit checks SPF, DMARC and DKIM for domain. A nil resolver uses
// net.DefaultResolver. The domain is lowercased and validated before any
// lookup happens.
func Audit(ctx context.Context, domain string, resolver TxtResolver) (*Report, error) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if !ValidDomain(domain) {
		return nil, fmt.Errorf("deliverability: invalid domain %q", domain)
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	r := &Report{Domain: domain}
	r.SPF = auditSPF(ctx, domain, resolver)
	r.DMARC = auditDMARC(ctx, domain, resolver)
	r.DKIM = auditDKIM(ctx, domain, resolver)
	r.Summary = summarize(r)
	return r, nil
}

// ValidDomain reports whether domain is a plausible public hostname for a DNS
// audit: ASCII labels separated by dots, no scheme, port, spaces, empty or
// over-long labels, and at least one dot (single-label names like "localhost"
// are not useful to audit).
func ValidDomain(domain string) bool {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	if domain == "" || len(domain) > 253 || !strings.Contains(domain, ".") {
		return false
	}
	if strings.ContainsAny(domain, " /:\\@") || strings.Contains(domain, "..") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
				continue
			}
			return false
		}
	}
	return true
}

// --- SPF -----------------------------------------------------------------

func auditSPF(ctx context.Context, domain string, r TxtResolver) Result {
	records, err := r.LookupTXT(ctx, domain)
	if err != nil {
		return Result{Verdict: "unknown", Guidance: "Could not read SPF TXT records (DNS error). Verify DNS is reachable and retry."}
	}
	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if !strings.HasPrefix(strings.ToLower(rec), "v=spf1") {
			continue
		}
		return Result{Present: true, Record: rec, Verdict: spfVerdict(rec), Guidance: spfGuidance(rec)}
	}
	return Result{Verdict: "missing", Guidance: "No SPF record found. Add one, e.g. v=spf1 include:_spf.google.com ~all, adjusted to your mail providers, then tighten to -all once every sender is covered."}
}

// spfVerdict classifies the enforcement qualifier in an SPF record.
func spfVerdict(rec string) string {
	switch {
	case strings.Contains(strings.ToLower(rec), "-all"):
		return "pass (hard fail)"
	case strings.Contains(strings.ToLower(rec), "~all"):
		return "softfail"
	default:
		return "pass (no enforcement)"
	}
}

func spfGuidance(rec string) string {
	switch {
	case strings.Contains(strings.ToLower(rec), "-all"):
		return "SPF is strict (-all). Keep every sending service in the include list or its mail will bounce."
	case strings.Contains(strings.ToLower(rec), "~all"):
		return "SPF is set with soft fail (~all). Move to -all once all senders are covered by includes."
	default:
		return "SPF is missing an all qualifier, so failures are not enforced. Add ~all (or -all) to the record."
	}
}

// --- DMARC ---------------------------------------------------------------

func auditDMARC(ctx context.Context, domain string, r TxtResolver) Result {
	records, err := r.LookupTXT(ctx, "_dmarc."+domain)
	if err != nil {
		return Result{Verdict: "unknown", Guidance: "Could not read DMARC TXT records (DNS error). Verify DNS is reachable and retry."}
	}
	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if !strings.HasPrefix(strings.ToLower(rec), "v=dmarc1") {
			continue
		}
		policy := dmarcPolicy(rec)
		return Result{Present: true, Record: rec, Verdict: policy, Guidance: dmarcGuidance(policy)}
	}
	return Result{Verdict: "missing", Guidance: "No DMARC record found. Add one: v=DMARC1; p=none; rua=mailto:postmaster@" + domain + " — start with p=none, then tighten to quarantine and reject as SPF/DKIM alignment improves."}
}

// dmarcPolicy extracts the p= value from a DMARC record.
func dmarcPolicy(rec string) string {
	for _, part := range strings.Split(rec, ";") {
		part = strings.TrimSpace(part)
		lower := strings.ToLower(part)
		if !strings.HasPrefix(lower, "p=") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(lower, "p="))
		switch v {
		case "reject", "quarantine", "none":
			return v
		default:
			return "unknown"
		}
	}
	return "unknown"
}

func dmarcGuidance(policy string) string {
	switch policy {
	case "reject":
		return "DMARC policy is reject — the strongest protection. Keep SPF/DKIM aligned or genuine mail will be rejected."
	case "quarantine":
		return "DMARC policy is quarantine (mail can go to spam). Move to p=reject once SPF/DKIM alignment is stable."
	case "none":
		return "DMARC is published but monitoring only (p=none). Move to p=quarantine, then p=reject, as SPF/DKIM alignment improves."
	default:
		return "DMARC record is present but the policy is unclear. Use p=none | quarantine | reject."
	}
}

// --- DKIM ----------------------------------------------------------------

func auditDKIM(ctx context.Context, domain string, r TxtResolver) DKIMResult {
	var found []string
	for _, sel := range dkimSelectors {
		if err := ctx.Err(); err != nil {
			break
		}
		records, err := r.LookupTXT(ctx, sel+"._domainkey."+domain)
		if err != nil {
			continue
		}
		for _, rec := range records {
			if strings.Contains(strings.ToLower(rec), "v=dkim1") || strings.Contains(strings.ToLower(rec), "p=") {
				found = append(found, sel)
				break
			}
		}
	}
	if len(found) > 0 {
		return DKIMResult{
			Found:     true,
			Selectors: found,
			Guidance:  "DKIM keys found for: " + strings.Join(found, ", ") + ". Ensure every sending service publishes its own key under _domainkey." + domain + ".",
		}
	}
	return DKIMResult{
		Guidance: "No DKIM key found for common selectors. Publish a key at <selector>._domainkey." + domain + " (your mail provider generates the TXT record) so mail is signed.",
	}
}

// --- Summary -------------------------------------------------------------

func summarize(r *Report) string {
	var gaps []string
	if r.SPF.Verdict == "missing" {
		gaps = append(gaps, "SPF missing")
	}
	if r.DMARC.Verdict == "missing" {
		gaps = append(gaps, "DMARC missing")
	}
	if !r.DKIM.Found {
		gaps = append(gaps, "DKIM not found")
	}
	if len(gaps) == 0 {
		return "SPF, DKIM and DMARC are in place — deliverability posture looks good."
	}
	return "Gaps: " + strings.Join(gaps, ", ") + ". Fix them to keep outreach mail out of spam."
}
