package osint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"time"
)

// VerificationStatus classifies how certain the verifier is about an address.
type VerificationStatus int

const (
	// StatusInvalid is a definitive negative: the address is malformed, the
	// domain accepts no mail (no MX / null MX), or the server permanently
	// (5xx) rejected the exact address.
	StatusInvalid VerificationStatus = iota
	// StatusValid means the server accepted the exact address (250) and the
	// domain was shown not to be catch-all.
	StatusValid
	// StatusCatchAll means the domain accepts any local part, so SMTP cannot
	// distinguish a real mailbox from a fake one.
	StatusCatchAll
	// StatusInconclusive means no definitive answer was reachable (port 25
	// blocked, timeouts, greylisting, sender rejected) — the address may or
	// may not exist.
	StatusInconclusive
)

// String implements fmt.Stringer.
func (s VerificationStatus) String() string {
	switch s {
	case StatusValid:
		return "valid"
	case StatusInvalid:
		return "invalid"
	case StatusCatchAll:
		return "catchall"
	case StatusInconclusive:
		return "inconclusive"
	default:
		return "unknown"
	}
}

// Stable machine-readable reasons for Verification.Reason. Downstream code
// may branch on them; treat them as API.
const (
	reasonMalformed            = "malformed"
	reasonDisposable           = "disposable"
	reasonNoMailHost           = "no_mail_host"
	reasonNullMX               = "null_mx"
	reasonRejected             = "rejected"
	reasonConfirmed            = "confirmed"
	reasonConfirmedCatchAllUkn = "confirmed_catchall_unknown"
	reasonCatchAll             = "catchall"
	reasonGreylisted           = "greylisted"
	reasonUnreachable          = "unreachable"
	reasonConnectFailed        = "connection_lost"
	reasonSenderRejected       = "sender_rejected"
	reasonCancelled            = "cancelled"
)

// Confidence floors produced by verification. They are applied on top of the
// source's own confidence, never below it, except for definitive rejections
// (see applyVerification).
const (
	// confidenceConfirmed is the floor for an address the server accepted.
	confidenceConfirmed = 85
	// confidenceConfirmedCatchAllUnknown applies when the address was
	// accepted but the catch-all question could not be answered.
	confidenceConfirmedCatchAllUnknown = 75
	// confidenceCatchAll applies to addresses on catch-all domains (pattern
	// guesses start at 25; 40 is the historical "domain is catch-all" value).
	confidenceCatchAll = 40
	// confidenceDomainReachable marks an address we could not confirm at a
	// domain whose mail server answered — the address may exist.
	confidenceDomainReachable = 35
)

// Resolver is the DNS backend used to find a domain's mail servers. The zero
// value in NewVerifier is net.DefaultResolver; tests substitute a fake.
type Resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// Connector opens TCP connections to mail servers ("host:port" addresses).
// The zero value in NewVerifier is a net.Dialer; tests substitute a fake to
// point probes at a local SMTP server.
type Connector interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Default verification tuning. Overrides live on the Verifier and take
// precedence over these values.
const (
	defaultConnectTimeout  = 10 * time.Second
	defaultReadTimeout     = 15 * time.Second
	defaultMaxAttempts     = 3
	defaultRetryBaseDelay  = 500 * time.Millisecond
	defaultInterProbeDelay = 250 * time.Millisecond
	defaultSMTPPort        = 25
)

// Verifier checks email addresses for deliverability using DNS and SMTP RCPT
// TO probes. No mail is ever sent, and no probe is ever retried beyond the
// configured budget. Create it with NewVerifier, then call Verify for a
// single address or VerifyContacts for a batch. A zero-value Verifier gets
// the same defaults lazily.
type Verifier struct {
	// Resolver finds MX / A records (defaults to net.DefaultResolver).
	Resolver Resolver
	// Connector dials mail servers (defaults to a net.Dialer).
	Connector Connector

	// ConnectTimeout bounds each TCP connection attempt (default 10s).
	ConnectTimeout time.Duration
	// ReadTimeout bounds each SMTP command exchange; it is refreshed between
	// commands so a long batch session never expires mid-probe (default 15s).
	ReadTimeout time.Duration
	// MaxAttempts is how many times a transient (greylisting) response is
	// retried with exponential backoff before the result is Inconclusive
	// (default 3).
	MaxAttempts int
	// RetryBaseDelay is the first backoff sleep between retries; the delay
	// doubles each attempt (default 500ms).
	RetryBaseDelay time.Duration
	// InterProbeDelay is the politeness pause between addresses on the same
	// domain (default 250ms; 0 disables).
	InterProbeDelay time.Duration
	// Port is the SMTP port used for probes (default 25). Only override it
	// in tests that run a local server.
	Port int
}

// NewVerifier returns a Verifier with sane defaults and the standard DNS and
// TCP backends.
func NewVerifier() *Verifier {
	return &Verifier{
		Resolver:        net.DefaultResolver,
		Connector:       &net.Dialer{},
		ConnectTimeout:  defaultConnectTimeout,
		ReadTimeout:     defaultReadTimeout,
		MaxAttempts:     defaultMaxAttempts,
		RetryBaseDelay:  defaultRetryBaseDelay,
		InterProbeDelay: defaultInterProbeDelay,
		Port:            defaultSMTPPort,
	}
}

func (v *Verifier) ensureDefaults() {
	if v.Resolver == nil {
		v.Resolver = net.DefaultResolver
	}
	if v.Connector == nil {
		v.Connector = &net.Dialer{}
	}
}

func (v *Verifier) connectTimeout() time.Duration {
	if v.ConnectTimeout <= 0 {
		return defaultConnectTimeout
	}
	return v.ConnectTimeout
}

func (v *Verifier) readTimeout() time.Duration {
	if v.ReadTimeout <= 0 {
		return defaultReadTimeout
	}
	return v.ReadTimeout
}

func (v *Verifier) maxAttempts() int {
	if v.MaxAttempts < 1 {
		return defaultMaxAttempts
	}
	return v.MaxAttempts
}

func (v *Verifier) retryBaseDelay() time.Duration {
	if v.RetryBaseDelay <= 0 {
		return defaultRetryBaseDelay
	}
	return v.RetryBaseDelay
}

func (v *Verifier) interProbeDelay() time.Duration {
	if v.InterProbeDelay < 0 {
		return 0
	}
	return v.InterProbeDelay
}

func (v *Verifier) port() int {
	if v.Port <= 0 {
		return defaultSMTPPort
	}
	return v.Port
}

// Verify checks a single email address for deliverability using DNS and an
// SMTP RCPT TO probe. It never sends mail and never panics: every outcome —
// including every network failure — is reported through Verification.Status
// and Verification.Reason, so callers always get an explicit answer to "what
// do we know about this address?".
func (v *Verifier) Verify(ctx context.Context, email string) Verification {
	v.ensureDefaults()
	start := time.Now()
	email = normalizeEmail(email)
	finish := func(vr Verification) Verification {
		vr.Email = email
		vr.Duration = time.Since(start)
		return vr
	}

	if !validEmail(email) {
		return finish(Verification{Status: StatusInvalid, Reason: reasonMalformed, Detail: "email address is malformed"})
	}
	if isDisposableDomain(email) {
		return finish(Verification{Status: StatusInvalid, Reason: reasonDisposable, Detail: "disposable/temporary email domain — not suitable for outreach"})
	}
	domain := domainOf(email)
	hosts, nullMX := resolveMailHosts(ctx, v.Resolver, domain, v.port())
	if nullMX {
		return finish(Verification{Status: StatusInvalid, Reason: reasonNullMX, Detail: "domain has a null MX record (RFC 7505) — it accepts no email"})
	}
	if len(hosts) == 0 {
		return finish(Verification{Status: StatusInvalid, Reason: reasonNoMailHost, Detail: "no mail server found (no MX or A record)"})
	}

	p := newProbeSession(v, ctx, hosts, domain)
	defer p.close()

	if err := p.open(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return finish(Verification{Status: StatusInconclusive, Reason: reasonCancelled, Detail: "verification cancelled"})
		}
		return finish(Verification{Status: StatusInconclusive, Reason: reasonUnreachable, Detail: "mail server unreachable — port 25 may be blocked on this network"})
	}

	catchAll, known := p.checkCatchAll(domain)
	if catchAll {
		return finish(Verification{Status: StatusCatchAll, Confidence: confidenceCatchAll, Reason: reasonCatchAll, Detail: "domain is catch-all — accepts any address"})
	}

	code, at, err := p.checkAddress(email)
	status, reason, detail := classifyProbe(code, at, err)
	if status == StatusValid {
		if known {
			return finish(Verification{Status: StatusValid, Confidence: confidenceConfirmed, Reason: reasonConfirmed, Detail: "smtp verified"})
		}
		return finish(Verification{Status: StatusValid, Confidence: confidenceConfirmedCatchAllUnknown, Reason: reasonConfirmedCatchAllUkn, Detail: "smtp verified; catch-all could not be ruled out"})
	}
	return finish(verificationFor(email, status, reason, detail))
}

// VerifyContacts checks every non-empty contact email in one batch: mail
// servers are resolved once per domain, one SMTP session is reused for the
// whole domain, and a small politeness delay separates probes. Contacts are
// never dropped or reordered: a definitive rejection zeroes the confidence,
// an inconclusive result keeps the source's confidence with a reason in
// Notes. Empty emails are left untouched (there is nothing to verify).
func (v *Verifier) VerifyContacts(ctx context.Context, contacts []Contact) []Contact {
	v.ensureDefaults()
	if len(contacts) == 0 {
		return contacts
	}

	out := make([]Contact, len(contacts))
	results := make(map[string]Verification, len(contacts))
	byDomain := make(map[string][]string, 8)
	seen := make(map[string]bool, len(contacts))

	for i, c := range contacts {
		email := normalizeEmail(c.Email)
		if email == "" {
			out[i] = c
			continue
		}
		if seen[email] {
			continue // verified once; the shared result is applied below
		}
		seen[email] = true
		if isDisposableDomain(email) {
			results[email] = Verification{Email: email, Status: StatusInvalid, Reason: reasonDisposable, Detail: "disposable/temporary email domain — not suitable for outreach"}
			continue
		}
		d := domainOf(email)
		byDomain[d] = append(byDomain[d], email)
	}

	domains := make([]string, 0, len(byDomain))
	for d := range byDomain {
		domains = append(domains, d)
	}
	slices.Sort(domains)

	for _, d := range domains {
		if d == "" {
			// Addresses with no usable @domain are malformed; never probe them.
			for _, e := range byDomain[d] {
				results[e] = Verification{Email: e, Status: StatusInvalid, Reason: reasonMalformed, Detail: "email address is malformed"}
			}
			continue
		}
		v.verifyDomain(ctx, d, byDomain[d], results)
	}

	for i, c := range contacts {
		email := normalizeEmail(c.Email)
		if email == "" {
			continue
		}
		vr, ok := results[email]
		if !ok {
			vr = Verification{Email: email, Status: StatusInvalid, Reason: reasonMalformed, Detail: "email address is malformed"}
		}
		out[i] = applyVerification(c, vr)
	}
	return out
}

// Verification is the outcome of checking a single email address.
type Verification struct {
	// Email is the normalized address that was checked.
	Email string
	// Status classifies how certain the outcome is (see VerificationStatus).
	Status VerificationStatus
	// Confidence is a 0-100 score derived from the verification (0 when the
	// verification adds no positive evidence).
	Confidence int
	// Reason is a stable machine-readable identifier for the outcome, e.g.
	// "confirmed", "rejected", "catchall", "unreachable".
	Reason string
	// Detail is a human-readable explanation suitable for Contact.Notes.
	Detail string
	// Duration is how long the check took.
	Duration time.Duration
}

// verificationFor builds an inconclusive Verification, carrying the
// domain-reachable confidence bump so pattern guesses on reachable domains
// are not flattened to zero by a network hiccup.
func verificationFor(email string, status VerificationStatus, reason, detail string) Verification {
	conf := 0
	if status == StatusInconclusive {
		conf = confidenceDomainReachable
	}
	return Verification{Email: email, Status: status, Confidence: conf, Reason: reason, Detail: detail}
}

// classifyProbe maps a raw SMTP probe result to a verification status.
func classifyProbe(code int, at string, err error) (VerificationStatus, string, string) {
	if err == nil {
		return StatusValid, reasonConfirmed, "smtp verified"
	}
	switch {
	case code/100 == 5 && at == "rcpt":
		return StatusInvalid, reasonRejected, fmt.Sprintf("mail server rejected the address (SMTP %d)", code)
	case code/100 == 5:
		return StatusInconclusive, reasonSenderRejected, fmt.Sprintf("mail server refused the probe sender (SMTP %d)", code)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return StatusInconclusive, reasonCancelled, "verification cancelled"
	case errors.Is(err, errTransient):
		return StatusInconclusive, reasonGreylisted, "mail server kept returning transient errors (greylisting)"
	case errors.Is(err, errUnreachable):
		return StatusInconclusive, reasonUnreachable, "mail server unreachable — port 25 may be blocked on this network"
	default:
		return StatusInconclusive, reasonConnectFailed, "mail server connection failed mid-probe"
	}
}

// verifyDomain checks every email at one domain, reusing a single SMTP
// session and caching the domain's catch-all verdict. Results are stored in
// results by normalized email.
func (v *Verifier) verifyDomain(ctx context.Context, domain string, emails []string, results map[string]Verification) {
	hosts, nullMX := resolveMailHosts(ctx, v.Resolver, domain, v.port())
	if nullMX {
		for _, e := range emails {
			results[e] = Verification{Email: e, Status: StatusInvalid, Reason: reasonNullMX, Detail: "domain has a null MX record (RFC 7505) — it accepts no email"}
		}
		return
	}
	if len(hosts) == 0 {
		for _, e := range emails {
			results[e] = Verification{Email: e, Status: StatusInvalid, Reason: reasonNoMailHost, Detail: "no mail server found (no MX or A record)"}
		}
		return
	}

	p := newProbeSession(v, ctx, hosts, domain)
	defer p.close()

	if err := p.open(); err != nil {
		reason, detail := reasonUnreachable, "mail server unreachable — port 25 may be blocked on this network"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			reason, detail = reasonCancelled, "verification cancelled"
		}
		for _, e := range emails {
			results[e] = Verification{Email: e, Status: StatusInconclusive, Reason: reason, Detail: detail}
		}
		return
	}

	catchAll, known := p.checkCatchAll(domain)
	if catchAll {
		for _, e := range emails {
			results[e] = Verification{Email: e, Status: StatusCatchAll, Confidence: confidenceCatchAll, Reason: reasonCatchAll, Detail: "domain is catch-all — accepts any address"}
		}
		return
	}

	for i, e := range emails {
		if !validEmail(e) {
			results[e] = Verification{Email: e, Status: StatusInvalid, Reason: reasonMalformed, Detail: "email address is malformed"}
			continue
		}
		code, at, err := p.checkAddress(e)
		status, reason, detail := classifyProbe(code, at, err)
		switch status {
		case StatusValid:
			if known {
				results[e] = Verification{Email: e, Status: StatusValid, Confidence: confidenceConfirmed, Reason: reasonConfirmed, Detail: "smtp verified"}
			} else {
				results[e] = Verification{Email: e, Status: StatusValid, Confidence: confidenceConfirmedCatchAllUnknown, Reason: reasonConfirmedCatchAllUkn, Detail: "smtp verified; catch-all could not be ruled out"}
			}
		case StatusInvalid:
			results[e] = Verification{Email: e, Status: StatusInvalid, Reason: reason, Detail: detail}
		default:
			conf := confidenceDomainReachable
			if reason == reasonCancelled {
				conf = 0
			}
			results[e] = Verification{Email: e, Status: status, Confidence: conf, Reason: reason, Detail: detail}
		}
		// Politeness: pause between probes to the same mail server.
		if i < len(emails)-1 && !sleep(ctx, v.interProbeDelay()) {
			for _, rest := range emails[i+1:] {
				results[rest] = Verification{Email: rest, Status: StatusInconclusive, Reason: reasonCancelled, Detail: "verification cancelled"}
			}
			return
		}
	}
}

// applyVerification folds a verification result into a contact's Confidence
// and Notes without ever dropping the contact. Verified and catch-all results
// never lower a higher external confidence (Hunter/Apollo/GitHub); definitive
// invalid results zero it; inconclusive results keep the source confidence,
// with a modest bump for pattern guesses when the domain was reachable.
func applyVerification(c Contact, v Verification) Contact {
	switch v.Status {
	case StatusValid, StatusCatchAll, StatusInconclusive:
		if v.Confidence > c.Confidence {
			c.Confidence = v.Confidence
		}
	case StatusInvalid:
		c.Confidence = 0
	}
	if v.Detail != "" {
		c.Notes = v.Detail
	}
	return c
}

// sleep waits for d while honoring ctx cancellation; it returns false when
// the context was cancelled.
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
