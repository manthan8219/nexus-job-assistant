package osint

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP is a scriptable in-process SMTP server for hermetic tests. It
// answers every command from a configurable policy, so tests can simulate
// acceptance, rejection, greylisting, catch-all domains, STARTTLS, and more.
type fakeSMTP struct {
	t *testing.T

	ln net.Listener
	mu sync.Mutex

	// recipients maps a lowercased full address to the SMTP code RCPT TO
	// should answer (default 550 when absent).
	recipients map[string]int
	// catchAll replies 250 to every RCPT TO.
	catchAll bool
	// transient maps an address to how many initial 451 responses to send
	// (simulates greylisting before accepting).
	transient map[string]int
	// transientPrefix maps a local-part prefix to the number of initial 451
	// responses to send (used to make the random catch-all probe ambiguous).
	transientPrefix map[string]int
	// mailCode is the MAIL FROM reply code (default 250).
	mailCode int
	// rsetCode is the RSET reply code (default 250; some servers reply 502).
	rsetCode int
	// startTLS advertises and performs STARTTLS when set.
	startTLS bool

	conns int
	rcpts []string
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("fake smtp listen: %v", err)
	}
	f := &fakeSMTP{
		t:               t,
		ln:              ln,
		recipients:      map[string]int{},
		transient:       map[string]int{},
		transientPrefix: map[string]int{},
		mailCode:        250,
		rsetCode:        250,
	}
	t.Cleanup(func() { ln.Close() })
	go f.serve()
	return f
}

func (f *fakeSMTP) addr() string { return f.ln.Addr().String() }

// connCount reports how many TCP connections the server has accepted.
func (f *fakeSMTP) connCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.conns
}

func (f *fakeSMTP) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.conns++
		f.mu.Unlock()
		go f.handle(conn)
	}
}

func (f *fakeSMTP) handle(conn net.Conn) {
	defer conn.Close()
	tp := textproto.NewConn(conn)
	defer tp.Close()

	if err := tp.PrintfLine("220 fake.test ESMTP ready"); err != nil {
		return
	}
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		upper := strings.ToUpper(strings.TrimSpace(line))
		cmd := upper
		if i := strings.IndexByte(upper, ' '); i >= 0 {
			cmd = upper[:i]
		}
		switch cmd {
		case "EHLO":
			if f.startTLS {
				tp.PrintfLine("250-fake.test hello\r\n250-STARTTLS\r\n250 8BITMIME")
			} else {
				tp.PrintfLine("250-fake.test hello\r\n250 8BITMIME")
			}
		case "HELO":
			tp.PrintfLine("250 fake.test hello")
		case "STARTTLS":
			if !f.startTLS {
				tp.PrintfLine("454 4.7.0 TLS not available")
				continue
			}
			tp.PrintfLine("220 2.0.0 ready to start TLS")
			tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{fakeTLSCert(f.t)}})
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			tp = textproto.NewConn(tlsConn)
		case "MAIL":
			if f.mailCode != 250 {
				tp.PrintfLine("%d 5.7.1 sender refused", f.mailCode)
			} else {
				tp.PrintfLine("250 2.1.0 sender ok")
			}
		case "RCPT":
			addr := parseRcptAddr(line)
			f.mu.Lock()
			if f.transient[addr] > 0 {
				f.transient[addr]--
				f.mu.Unlock()
				tp.PrintfLine("451 4.7.1 greylisted, try again later")
				continue
			}
			if pref, ok := f.matchTransientPrefix(addr); ok {
				f.transientPrefix[pref]--
				f.mu.Unlock()
				tp.PrintfLine("451 4.7.1 greylisted, try again later")
				continue
			}
			f.rcpts = append(f.rcpts, addr)
			code, configured := f.recipients[addr]
			f.mu.Unlock()
			switch {
			case f.catchAll:
				tp.PrintfLine("250 2.1.5 ok")
			case configured && code == 250:
				tp.PrintfLine("250 2.1.5 ok")
			case configured:
				tp.PrintfLine("%d 5.1.1 user unknown", code)
			default:
				tp.PrintfLine("550 5.1.1 user unknown")
			}
		case "RSET":
			tp.PrintfLine("%d 2.0.0 ok", f.rsetCode)
		case "QUIT":
			tp.PrintfLine("221 2.0.0 bye")
			return
		default:
			tp.PrintfLine("502 5.5.1 command not implemented")
		}
	}
}

// matchTransientPrefix returns a transientPrefix key that is a prefix of the
// address's local part and still has budget, if any.
func (f *fakeSMTP) matchTransientPrefix(addr string) (string, bool) {
	local := addr
	if i := strings.IndexByte(local, '@'); i >= 0 {
		local = local[:i]
	}
	for p, n := range f.transientPrefix {
		if n > 0 && strings.HasPrefix(local, p) {
			return p, true
		}
	}
	return "", false
}

// parseRcptAddr extracts the bare address from an SMTP "RCPT TO:<addr>" line.
func parseRcptAddr(line string) string {
	line = strings.TrimSpace(line)
	if i := strings.IndexByte(line, '<'); i >= 0 {
		line = line[i+1:]
	}
	if i := strings.LastIndexByte(line, '>'); i >= 0 {
		line = line[:i]
	}
	return strings.ToLower(strings.TrimSpace(line))
}

// fakeTLSCert returns a fresh self-signed certificate for STARTTLS tests.
// ECDSA P-256 key generation is fast enough to create per test.
func fakeTLSCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "fake.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"fake.test", "mx1.acme.test"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// fakeResolver returns scripted DNS records, never touching the network.
type fakeResolver struct {
	mx     map[string][]string // domain -> MX hostnames in preference order
	hosts  map[string][]string // hostname -> A/AAAA addresses
	nullMX map[string]bool
}

func (r *fakeResolver) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	if r.nullMX[name] {
		return []*net.MX{{Host: "."}}, nil
	}
	if hs, ok := r.mx[name]; ok {
		out := make([]*net.MX, len(hs))
		for i, h := range hs {
			out[i] = &net.MX{Host: h}
		}
		return out, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func (r *fakeResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	if addrs, ok := r.hosts[host]; ok {
		return addrs, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

// fakeConnector dials a fixed address (the fake SMTP server) no matter what
// host:port the verifier asks for, so tests can use realistic hostnames in
// their fake DNS records.
type fakeConnector struct{ addr string }

func (c *fakeConnector) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, c.addr)
}

// testVerifier returns a Verifier whose DNS resolves mxHosts for domain and
// whose connections all go to the fake SMTP server.
func testVerifier(t *testing.T, srv *fakeSMTP, domain string, mxHosts []string) *Verifier {
	t.Helper()
	v := NewVerifier()
	v.Resolver = &fakeResolver{mx: map[string][]string{domain: mxHosts}}
	v.Connector = &fakeConnector{addr: srv.addr()}
	v.RetryBaseDelay = time.Millisecond
	v.InterProbeDelay = 0
	v.MaxAttempts = 3
	return v
}
