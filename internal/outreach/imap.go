package outreach

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// gmailIMAPAddr is Gmail's IMAP-over-TLS endpoint.
const gmailIMAPAddr = "imap.gmail.com:993"

// maxMessagesPerCheck bounds how many headers one pass downloads.
const maxMessagesPerCheck = 300

// GmailIMAPFetcher reads inbox message headers over IMAP with the Gmail
// app password. (The OAuth token is scoped gmail.send only, so IMAP is
// the read path — app passwords work with IMAP out of the box.)
type GmailIMAPFetcher struct {
	Addr     string // defaults to gmailIMAPAddr when empty
	User     string
	Password string
}

// NewGmailIMAPFetcher builds a fetcher from config, or nil when the Gmail
// app password (or From address) is not configured.
func NewGmailIMAPFetcher(cfg *config.Config) *GmailIMAPFetcher {
	if cfg == nil {
		return nil
	}
	user := strings.TrimSpace(cfg.Email)
	pass := strings.TrimSpace(cfg.GmailAppPassword)
	if user == "" || pass == "" {
		return nil
	}
	return &GmailIMAPFetcher{Addr: gmailIMAPAddr, User: user, Password: pass}
}

// dialSelect opens a TLS connection to the IMAP server, greets, logs in and
// selects INBOX. The caller owns the returned connection (close it, and call
// logout best-effort).
func (f *GmailIMAPFetcher) dialSelect(ctx context.Context) (*imapConn, error) {
	addr := f.Addr
	if addr == "" {
		addr = gmailIMAPAddr
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	dialer := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 20 * time.Second},
		Config:    &tls.Config{ServerName: host},
	}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("imap dial %s: %w", addr, err)
	}
	c := newIMAPConn(conn)
	if err := c.greet(); err != nil {
		conn.Close()
		return nil, err
	}
	if err := c.login(f.User, f.Password); err != nil {
		conn.Close()
		return nil, err
	}
	if err := c.selectMailbox("INBOX"); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

// FetchMessages returns inbox message headers received since the given time,
// newest first, capped at maxMessagesPerCheck.
func (f *GmailIMAPFetcher) FetchMessages(ctx context.Context, since time.Time) ([]Reply, error) {
	c, err := f.dialSelect(ctx)
	if err != nil {
		return nil, err
	}
	defer c.logout()
	defer c.Close()
	return collectReplies(ctx, c, since)
}

// FetchMessagesWithBodies returns inbox messages received since the given
// time (headers plus decoded plain-text bodies), newest first, capped at max.
// Used by the inbox hiring-highlights scan; max <= 0 falls back to the
// standard per-check cap.
func (f *GmailIMAPFetcher) FetchMessagesWithBodies(ctx context.Context, since time.Time, max int) ([]Reply, error) {
	if max <= 0 {
		max = maxMessagesPerCheck
	}
	c, err := f.dialSelect(ctx)
	if err != nil {
		return nil, err
	}
	defer c.logout()
	defer c.Close()
	return collectMessagesWithBodies(ctx, c, since, max)
}

// collectReplies searches and downloads message headers on an open,
// selected connection. Split from FetchMessages so tests can drive it
// over an in-memory pipe.
func collectReplies(ctx context.Context, c *imapConn, since time.Time) ([]Reply, error) {
	nums, err := c.searchSince(since)
	if err != nil {
		return nil, err
	}
	if len(nums) > maxMessagesPerCheck {
		nums = nums[len(nums)-maxMessagesPerCheck:]
	}

	var out []Reply
	// Newest first: walk the sequence numbers backwards.
	for i := len(nums) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rep, err := c.fetchReply(nums[i])
		if err != nil {
			return nil, err
		}
		if rep == nil {
			continue // unparseable headers — skip, not fatal
		}
		if rep.Date.Before(since) {
			continue // SINCE is day-granular; refine with the Date header
		}
		out = append(out, *rep)
	}
	return out, nil
}

// collectMessagesWithBodies downloads headers + bodies for every message
// received on/after since, capped at max, newest first.
func collectMessagesWithBodies(ctx context.Context, c *imapConn, since time.Time, max int) ([]Reply, error) {
	nums, err := c.searchSince(since)
	if err != nil {
		return nil, fmt.Errorf("imap search: %w", err)
	}
	if len(nums) > max {
		nums = nums[len(nums)-max:]
	}
	var out []Reply
	for i := len(nums) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rep, err := c.fetchMessageWithBody(nums[i])
		if err != nil {
			return nil, err
		}
		if rep == nil {
			continue
		}
		if rep.Date.Before(since) {
			continue
		}
		out = append(out, *rep)
	}
	return out, nil
}

// fetchMessageWithBody downloads the header fields plus the plain-text body
// of one message. Returns nil when the headers can't be parsed.
func (c *imapConn) fetchMessageWithBody(num string) (*Reply, error) {
	resp, err := c.command("FETCH %s (BODY.PEEK[HEADER.FIELDS (FROM SUBJECT DATE MESSAGE-ID)] BODY.PEEK[TEXT])", num)
	if err != nil {
		return nil, fmt.Errorf("imap fetch %s: %w", num, err)
	}
	blocks := extractLiterals(resp)
	if len(blocks) == 0 {
		return nil, nil
	}
	msg, err := mail.ReadMessage(bytes.NewReader(blocks[0]))
	if err != nil {
		return nil, nil
	}
	addr, err := mail.ParseAddress(msg.Header.Get("From"))
	if err != nil || addr == nil {
		return nil, nil
	}
	date, _ := mail.ParseDate(msg.Header.Get("Date"))
	body := ""
	if len(blocks) > 1 {
		body = trimBody(blocks[1])
	}
	return &Reply{
		From:      strings.ToLower(addr.Address),
		FromName:  addr.Name,
		Subject:   decodeHeaderText(msg.Header.Get("Subject")),
		Date:      date,
		MessageID: msg.Header.Get("Message-Id"),
		Body:      body,
	}, nil
}

// trimBody collapses whitespace and bounds the stored body preview.
func trimBody(b []byte) string {
	s := strings.Join(strings.Fields(strings.TrimSpace(string(b))), " ")
	const maxBody = 4000
	if len(s) > maxBody {
		s = s[:maxBody]
	}
	return s
}

// ── minimal synchronous IMAP4rev1 client ──────────────────────────────────

// imapConn implements just enough of RFC 3501 for Gmail reply checks.
// One command at a time, no IDLE, no pipelines.
type imapConn struct {
	conn net.Conn
	r    *bufio.Reader
	n    int // tag counter
}

func newIMAPConn(conn net.Conn) *imapConn {
	return &imapConn{conn: conn, r: bufio.NewReader(conn)}
}

// Close closes the underlying network connection.
func (c *imapConn) Close() error { return c.conn.Close() }

func (c *imapConn) greet() error {
	line, err := c.readLine()
	if err != nil {
		return fmt.Errorf("imap greeting: %w", err)
	}
	if !strings.HasPrefix(line, "* OK") {
		return fmt.Errorf("imap greeting: unexpected %q", line)
	}
	return nil
}

// searchSince returns the sequence numbers of messages received on/after
// the date (IMAP SINCE is day-granular).
func (c *imapConn) searchSince(since time.Time) ([]string, error) {
	resp, err := c.command("SEARCH SINCE %s", since.Format("2-Jan-2006"))
	if err != nil {
		return nil, fmt.Errorf("imap search: %w", err)
	}
	var nums []string
	for _, line := range strings.Split(resp, "\r\n") {
		if !strings.HasPrefix(line, "* SEARCH") {
			continue
		}
		for _, f := range strings.Fields(strings.TrimPrefix(line, "* SEARCH")) {
			if _, err := strconv.Atoi(f); err == nil {
				nums = append(nums, f)
			}
		}
	}
	return nums, nil
}

// fetchReply downloads the From/Subject/Date headers of one message.
// Returns nil when the headers can't be parsed (skipped, not fatal).
func (c *imapConn) fetchReply(num string) (*Reply, error) {
	resp, err := c.command("FETCH %s (BODY.PEEK[HEADER.FIELDS (FROM SUBJECT DATE)])", num)
	if err != nil {
		return nil, fmt.Errorf("imap fetch %s: %w", num, err)
	}
	hdr := literalBytes(resp)
	if len(hdr) == 0 {
		return nil, nil
	}
	msg, err := mail.ReadMessage(bytes.NewReader(hdr))
	if err != nil {
		return nil, nil
	}
	addr, err := mail.ParseAddress(msg.Header.Get("From"))
	if err != nil || addr == nil {
		return nil, nil
	}
	date, _ := mail.ParseDate(msg.Header.Get("Date"))
	return &Reply{
		From:     strings.ToLower(addr.Address),
		FromName: addr.Name,
		Subject:  decodeHeaderText(msg.Header.Get("Subject")),
		Date:     date,
	}, nil
}

// command sends one tagged command and returns the full untagged payload
// (lines and literal data) up to the tagged completion. NO/BAD → error.
func (c *imapConn) command(format string, args ...any) (string, error) {
	c.n++
	tag := fmt.Sprintf("nexus%d", c.n)
	_ = c.conn.SetDeadline(time.Now().Add(30 * time.Second))
	if _, err := fmt.Fprintf(c.conn, "%s %s\r\n", tag, fmt.Sprintf(format, args...)); err != nil {
		return "", err
	}
	var b strings.Builder
	for {
		line, err := c.readLine()
		if err != nil {
			return "", err
		}
		b.WriteString(line)
		if n, ok := literalSize(line); ok {
			buf := make([]byte, n)
			if _, err := io.ReadFull(c.r, buf); err != nil {
				return "", err
			}
			b.Write(buf)
			continue
		}
		if strings.HasPrefix(line, tag+" ") {
			rest := strings.TrimSpace(strings.TrimPrefix(line, tag))
			if strings.HasPrefix(rest, "OK") {
				return b.String(), nil
			}
			return "", fmt.Errorf("%s", rest)
		}
	}
}

// readLine returns the raw line including the CRLF — response offsets must
// stay exact or literal extraction ({N} byte blocks) goes out of sync.
func (c *imapConn) readLine() (string, error) {
	return c.r.ReadString('\n')
}

// quoteIMAP wraps s in an IMAP quoted string.
func quoteIMAP(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// literalSize reports whether the line ends an IMAP literal marker ({N})
// and returns N.
func literalSize(line string) (int, bool) {
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasSuffix(line, "}") {
		return 0, false
	}
	i := strings.LastIndex(line, "{")
	if i < 0 {
		return 0, false
	}
	n, err := strconv.Atoi(line[i+1 : len(line)-1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// extractLiterals returns every literal ({N} byte block) from a response, in
// order -- used to pull both the header and body literals of a FETCH reply.
func extractLiterals(resp string) [][]byte {
	var out [][]byte
	rest := resp
	for {
		i := strings.Index(rest, "{")
		if i < 0 {
			break
		}
		j := strings.Index(rest[i:], "}")
		if j < 0 {
			break
		}
		n, err := strconv.Atoi(rest[i+1 : i+j])
		if err != nil || n <= 0 {
			break
		}
		start := i + j + 1
		if strings.HasPrefix(rest[start:], "\r\n") {
			start += 2
		} else if strings.HasPrefix(rest[start:], "\n") {
			start++
		}
		if start+n > len(rest) {
			break
		}
		out = append(out, []byte(rest[start:start+n]))
		rest = rest[start+n:]
	}
	return out
}

// literalBytes extracts the first literal data block from a FETCH response.
func literalBytes(resp string) []byte {
	i := strings.Index(resp, "{")
	if i < 0 {
		return nil
	}
	j := strings.Index(resp[i:], "}")
	if j < 0 {
		return nil
	}
	n, err := strconv.Atoi(resp[i+1 : i+j])
	if err != nil || n <= 0 {
		return nil
	}
	start := i + j + 1
	// Skip the CRLF between the marker and the literal data.
	if strings.HasPrefix(resp[start:], "\r\n") {
		start += 2
	} else if strings.HasPrefix(resp[start:], "\n") {
		start++
	}
	if start+n > len(resp) {
		return nil
	}
	return []byte(resp[start : start+n])
}

// decodeHeaderText decodes RFC 2047 encoded words when present.
func decodeHeaderText(s string) string {
	if s == "" || !strings.Contains(s, "=?") {
		return s
	}
	dec := new(mime.WordDecoder)
	out, err := dec.DecodeHeader(s)
	if err != nil {
		return s
	}
	return out
}

func (c *imapConn) login(user, pass string) error {
	if _, err := c.command("LOGIN %s %s", quoteIMAP(user), quoteIMAP(pass)); err != nil {
		return fmt.Errorf("imap login (check the Gmail app password): %w", err)
	}
	return nil
}

func (c *imapConn) selectMailbox(name string) error {
	if _, err := c.command("SELECT %s", quoteIMAP(name)); err != nil {
		return fmt.Errorf("imap select %s: %w", name, err)
	}
	return nil
}

func (c *imapConn) logout() {
	_, _ = c.command("LOGOUT")
}
