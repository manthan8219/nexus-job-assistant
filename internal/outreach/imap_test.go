package outreach

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeIMAPServer plays a scripted Gmail-ish server over one end of a
// net.Pipe. headers maps message sequence number → raw header block.
func fakeIMAPServer(srv net.Conn, headers map[string]string) {
	defer srv.Close()
	r := bufio.NewReader(srv)
	fmt.Fprint(srv, "* OK Gimap ready for requests\r\n")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		fields := strings.SplitN(strings.TrimRight(line, "\r\n"), " ", 3)
		if len(fields) < 2 {
			return
		}
		tag, cmd := fields[0], strings.ToUpper(fields[1])
		rest := ""
		if len(fields) == 3 {
			rest = fields[2]
		}
		switch cmd {
		case "LOGIN":
			fmt.Fprintf(srv, "%s OK LOGIN completed\r\n", tag)
		case "SELECT":
			fmt.Fprintf(srv, "* %d EXISTS\r\n%s OK SELECT completed\r\n", len(headers), tag)
		case "SEARCH":
			nums := make([]string, 0, len(headers))
			for n := range headers {
				nums = append(nums, n)
			}
			fmt.Fprintf(srv, "* SEARCH %s\r\n%s OK SEARCH completed\r\n", strings.Join(nums, " "), tag)
		case "FETCH":
			num := strings.Fields(rest)[0]
			hdr := headers[num]
			fmt.Fprintf(srv, "* %s FETCH (BODY[HEADER.FIELDS (FROM SUBJECT DATE)] {%d}\r\n%s\r\n)\r\n%s OK FETCH completed\r\n",
				num, len(hdr), hdr, tag)
		case "LOGOUT":
			fmt.Fprintf(srv, "* BYE\r\n%s OK LOGOUT completed\r\n", tag)
			return
		default:
			fmt.Fprintf(srv, "%s BAD unknown command\r\n", tag)
		}
	}
}

// fakeIMAPServerBodyFirst plays a server that returns the BODY literal before
// the HEADER literal (the ordering Gmail actually uses), to exercise the
// content-based header detection in fetchMessageWithBody.
func fakeIMAPServerBodyFirst(srv net.Conn, hdr, body string) {
	defer srv.Close()
	r := bufio.NewReader(srv)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		fields := strings.SplitN(strings.TrimRight(line, "\r\n"), " ", 3)
		if len(fields) < 2 {
			return
		}
		tag, cmd := fields[0], strings.ToUpper(fields[1])
		if cmd == "FETCH" {
			fmt.Fprintf(srv, "* 5 FETCH (BODY[TEXT] {%d}\r\n%s\r\n BODY[HEADER.FIELDS (FROM SUBJECT DATE MESSAGE-ID)] {%d}\r\n%s\r\n)\r\n%s OK FETCH completed\r\n",
				len(body), body, len(hdr), hdr, tag)
			continue
		}
		fmt.Fprintf(srv, "%s OK ok\r\n", tag)
	}
}

func TestFetchMessageWithBodyHandlesBodyFirstOrder(t *testing.T) {
	hdr := "From: Jane <jane@acme.com>\r\nSubject: Interview invitation\r\nDate: Wed, 29 Jul 2026 09:12:00 +0000\r\nMessage-ID: <abc123>\r\n\r\n"
	body := "We would love to schedule an interview with you.\r\n"
	client, server := net.Pipe()
	defer client.Close()
	go fakeIMAPServerBodyFirst(server, hdr, body)

	c := newIMAPConn(client)
	rep, err := c.fetchMessageWithBody("5")
	if err != nil {
		t.Fatalf("fetchMessageWithBody: %v", err)
	}
	if rep == nil {
		t.Fatal("expected a reply, got nil")
	}
	if rep.From != "jane@acme.com" {
		t.Errorf("From = %q; want jane@acme.com", rep.From)
	}
	if rep.Subject != "Interview invitation" {
		t.Errorf("Subject = %q; want Interview invitation", rep.Subject)
	}
	if rep.MessageID != "<abc123>" {
		t.Errorf("MessageID = %q; want <abc123>", rep.MessageID)
	}
	if !strings.Contains(rep.Body, "schedule an interview") {
		t.Errorf("Body = %q; want to contain the body text", rep.Body)
	}
}

func TestCollectRepliesOverPipe(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	headers := map[string]string{
		"2": "From: \"Jane Recruiter\" <jane@acme.com>\r\n" +
			"Subject: Re: Quick note — Backend Engineer at Acme\r\n" +
			"Date: Wed, 29 Jul 2026 09:12:00 +0000\r\n\r\n",
		"1": "From: old@other.io\r\n" +
			"Subject: an old thread\r\n" +
			"Date: Mon, 01 Jun 2026 10:00:00 +0000\r\n\r\n",
	}
	go fakeIMAPServer(server, headers)

	c := newIMAPConn(client)
	if err := c.greet(); err != nil {
		t.Fatalf("greet: %v", err)
	}
	if err := c.login("user@gmail.com", "apppassword"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := c.selectMailbox("INBOX"); err != nil {
		t.Fatalf("select: %v", err)
	}

	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	reps, err := collectReplies(context.Background(), c, since)
	if err != nil {
		t.Fatalf("collectReplies: %v", err)
	}
	c.logout()

	if len(reps) != 1 {
		t.Fatalf("expected 1 reply inside the window, got %d: %+v", len(reps), reps)
	}
	r := reps[0]
	if r.From != "jane@acme.com" {
		t.Errorf("From = %q; want jane@acme.com", r.From)
	}
	if r.FromName != "Jane Recruiter" {
		t.Errorf("FromName = %q; want Jane Recruiter", r.FromName)
	}
	if !strings.Contains(r.Subject, "Quick note") {
		t.Errorf("Subject = %q; want it to mention Quick note", r.Subject)
	}
	wantDate := time.Date(2026, 7, 29, 9, 12, 0, 0, time.UTC)
	if !r.Date.Equal(wantDate) {
		t.Errorf("Date = %v; want %v", r.Date, wantDate)
	}
}

func TestCollectRepliesLoginFailure(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		r := bufio.NewReader(server)
		fmt.Fprint(server, "* OK ready\r\n")
		line, _ := r.ReadString('\n')
		tag := strings.SplitN(line, " ", 2)[0]
		fmt.Fprintf(server, "%s NO [AUTHENTICATIONFAILED] Invalid credentials\r\n", tag)
	}()

	c := newIMAPConn(client)
	if err := c.greet(); err != nil {
		t.Fatalf("greet: %v", err)
	}
	err := c.login("user@gmail.com", "wrongpassword")
	if err == nil {
		t.Fatal("login with bad password should fail")
	}
	if !strings.Contains(err.Error(), "app password") {
		t.Errorf("error should point at the app password, got: %v", err)
	}
}

func TestLiteralSize(t *testing.T) {
	tests := []struct {
		in   string
		want int
		ok   bool
	}{
		{"* 2 FETCH (BODY[HEADER] {152}\r", 152, true},
		{"* 1 FETCH (FLAGS ())", 0, false},
		{"nexus1 OK done", 0, false},
		{"* SEARCH 1 2", 0, false},
		{"{notanumber}", 0, false},
	}
	for _, tt := range tests {
		got, ok := literalSize(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("literalSize(%q) = %d, %v; want %d, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestLiteralBytes(t *testing.T) {
	hdr := "From: a@b.com\r\nSubject: hi\r\n\r\n"
	resp := "* 2 FETCH (BODY[HEADER] {" + fmt.Sprint(len(hdr)) + "}\r\n" + hdr + "\r\n)\r\nnexus1 OK\r\n"
	got := literalBytes(resp)
	if string(got) != hdr {
		t.Errorf("literalBytes = %q; want %q", got, hdr)
	}
	if literalBytes("* 1 FETCH (FLAGS ())") != nil {
		t.Errorf("no literal → want nil")
	}
}

func TestQuoteIMAP(t *testing.T) {
	if got := quoteIMAP(`us"er\name`); got != `"us\"er\\name"` {
		t.Errorf("quoteIMAP escaping = %q", got)
	}
	if got := quoteIMAP("plain@gmail.com"); got != `"plain@gmail.com"` {
		t.Errorf("quoteIMAP plain = %q", got)
	}
}
