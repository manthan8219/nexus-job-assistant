// Package osint finds HR/recruiter contacts for cold outreach and verifies
// their email addresses for deliverability.
//
// Contact discovery combines paid sources (Hunter.io, Apollo.io) with free
// ones (GitHub org members, a local OSINT scraper service) and pattern-based
// fallbacks (careers@/hr@/jobs@ …).
//
// Verification (Verifier) checks an address over DNS + SMTP without sending
// mail: it resolves the domain's mail servers, performs an RCPT TO probe, and
// detects catch-all domains. Every outcome — valid, invalid, catch-all, or
// inconclusive (blocked port 25, greylisting, timeouts) — is returned
// explicitly; verification never drops a contact and never panics.
package osint
