package osint

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// errTransient reports that a server kept returning transient (4xx) responses
// until the retry budget was exhausted (typical for greylisting).
var errTransient = errors.New("transient SMTP error (greylisting)")

// errUnreachable reports that none of the domain's mail servers could be
// connected to (blocked port 25, down host, unreachable network).
var errUnreachable = errors.New("no mail server reachable")

// smtpMailHost is one mail server to probe, in preference order.
type smtpMailHost struct {
	host string
	port int
}

// resolveMailHosts finds the servers that accept mail for domain, in
// preference order, using the given Resolver. It recognizes the RFC 7505
// null-MX signal (an MX record whose host is "." — the domain accepts no
// email at all) and falls back from MX records to the domain's own A/AAAA
// records, then the conventional smtp.<domain> / mail.<domain> hosts.
func resolveMailHosts(ctx context.Context, r Resolver, domain string, port int) (hosts []smtpMailHost, nullMX bool) {
	if mxs, err := r.LookupMX(ctx, domain); err == nil && len(mxs) > 0 {
		allNull := true
		for _, mx := range mxs {
			h := strings.TrimSuffix(mx.Host, ".")
			if h == "" {
				continue // RFC 7505 null-MX marker: this record accepts nothing
			}
			allNull = false
			hosts = append(hosts, smtpMailHost{host: h, port: port})
		}
		if allNull {
			return nil, true
		}
		if len(hosts) > 0 {
			return hosts, false
		}
	}
	// No usable MX records. Some small domains run mail directly on their
	// A/AAAA address or on conventional subdomains without declaring MX.
	for _, candidate := range []string{domain, "smtp." + domain, "mail." + domain} {
		if addrs, err := r.LookupHost(ctx, candidate); err == nil && len(addrs) > 0 {
			return []smtpMailHost{{host: candidate, port: port}}, false
		}
	}
	return nil, false
}

// smtpSession is one live SMTP connection to a mail server.
type smtpSession struct {
	conn        net.Conn
	c           *smtp.Client
	readTimeout time.Duration
}

// dialMailHost connects to host and completes the SMTP greeting. It
// opportunistically upgrades to STARTTLS when the server advertises it (many
// modern servers refuse plaintext MAIL otherwise), skipping certificate
// identity — the probe sends no secrets, so verifying the cert would only
// cause false inconclusives on valid servers.
func dialMailHost(ctx context.Context, connector Connector, host smtpMailHost, connectTimeout, readTimeout time.Duration) (*smtpSession, error) {
	addr := net.JoinHostPort(host.host, strconv.Itoa(host.port))
	dialCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	nc, err := connector.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	nc.SetDeadline(time.Now().Add(readTimeout)) //nolint:errcheck
	c, err := smtp.NewClient(nc, host.host)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("greeting from %s: %w", host.host, err)
	}
	s := &smtpSession{conn: nc, c: c, readTimeout: readTimeout}
	if ok, _ := c.Extension("STARTTLS"); ok {
		// Opportunistic only: a broken TLS upgrade must not fail the whole
		// verification when the server also accepts plaintext. Certificate
		// identity is skipped on purpose — the probe transmits no secrets,
		// and servers with private CAs would otherwise never verify.
		_ = c.StartTLS(&tls.Config{
			ServerName:         host.host,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, //nolint:gosec // probe carries no secrets
		})
	}
	return s, nil
}

// probe runs MAIL FROM + RCPT TO for email and reports the SMTP reply code
// and which command produced it ("mail" or "rcpt"). A zero code with a
// non-nil error means a network or protocol failure; the session is then
// unusable and must be re-dialed.
func (s *smtpSession) probe(sender, email string) (code int, at string, err error) {
	s.conn.SetDeadline(time.Now().Add(s.readTimeout)) //nolint:errcheck
	// RSET is best-effort: per RFC 5321 a fresh MAIL FROM implicitly clears
	// any previous envelope, so servers that do not implement RSET (502)
	// must not break the probe.
	_ = s.c.Reset()
	if err := s.c.Mail(sender); err != nil {
		return textprotoCode(err), "mail", err
	}
	if err := s.c.Rcpt(email); err != nil {
		return textprotoCode(err), "rcpt", err
	}
	return 250, "rcpt", nil
}

// textprotoCode extracts the SMTP reply code from an error, or 0 when the
// error is not an SMTP-level response (timeouts, connection resets, …).
func textprotoCode(err error) int {
	var tperr *textproto.Error
	if errors.As(err, &tperr) {
		return tperr.Code
	}
	return 0
}

// probeSession manages SMTP connections to one domain's mail servers: it
// reconnects after network failures, retries transient (greylisting)
// responses with bounded backoff, and reuses a single connection across the
// addresses of the same domain.
type probeSession struct {
	v      *Verifier
	ctx    context.Context
	hosts  []smtpMailHost
	sender string
	sess   *smtpSession
}

// newProbeSession creates a session manager for the given mail hosts. sender
// is the envelope-from used for MAIL FROM — the probed domain's own address,
// because servers are far more tolerant of a sender domain they host.
func newProbeSession(v *Verifier, ctx context.Context, hosts []smtpMailHost, domain string) *probeSession {
	return &probeSession{v: v, ctx: ctx, hosts: hosts, sender: "probe@" + domain}
}

// open dials the first reachable mail server, trying every host in preference
// order. Context cancellation is propagated distinctly so callers can report
// it honestly instead of as a generic connectivity failure.
func (p *probeSession) open() error {
	var lastErr error
	for _, h := range p.hosts {
		s, err := dialMailHost(p.ctx, p.v.Connector, h, p.v.connectTimeout(), p.v.readTimeout())
		if err == nil {
			p.sess = s
			return nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("%w (%d host(s) tried, last: %v)", errUnreachable, len(p.hosts), lastErr)
}

// close politely quits the session if one is open.
func (p *probeSession) close() {
	if p.sess != nil {
		_ = p.sess.c.Quit()
		p.sess = nil
	}
}

// drop closes and discards the current session so the next probe reconnects.
func (p *probeSession) drop() {
	if p.sess != nil {
		_ = p.sess.c.Close()
		p.sess = nil
	}
}

// checkAddress probes one address, retrying transient responses with
// exponential backoff and reconnecting after network-level failures. It
// returns the last SMTP code (0 for network-level failure) and the command
// that produced it.
func (p *probeSession) checkAddress(email string) (code int, at string, err error) {
	delay := p.v.retryBaseDelay()
	for attempt := 0; attempt < p.v.maxAttempts(); attempt++ {
		if attempt > 0 {
			if !sleep(p.ctx, delay) {
				return 0, "rcpt", context.Canceled
			}
			delay *= 2
		}
		if p.sess == nil {
			if err := p.open(); err != nil {
				return 0, "connect", err
			}
		}
		code, at, err = p.sess.probe(p.sender, email)
		switch code / 100 {
		case 2, 5:
			return code, at, err
		case 4:
			continue // greylisting / transient — retry after backoff
		default:
			// Network or protocol failure: probe dropped the connection, so
			// drop the session and let the next iteration reconnect. A
			// genuinely dead host surfaces in open().
			p.drop()
			continue
		}
	}
	return 0, "rcpt", errTransient
}

// checkCatchAll probes a random local part to see whether the domain accepts
// every address. known is false when the answer is ambiguous (transient or
// network failure); the per-address verdicts then stand on their own but the
// domain cannot be cleared as "definitely not catch-all". Only a definitive
// 5xx rejection of the random recipient clears the domain.
func (p *probeSession) checkCatchAll(domain string) (catchAll, known bool) {
	code, at, _ := p.checkAddress(randomLocalPart() + "@" + domain)
	switch {
	case code/100 == 2:
		return true, true
	case code/100 == 5 && at == "rcpt":
		return false, true
	default:
		return false, false
	}
}

// randomLocalPart returns a random local part unlikely to belong to a real
// mailbox, for catch-all detection. crypto/rand failure falls back to a
// time-based value rather than failing the probe.
func randomLocalPart() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "verif" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "verif" + hex.EncodeToString(b)
}
