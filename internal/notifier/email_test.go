package notifier

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeSMTPServer is a minimal in-process SMTP server that captures the DATA
// payload of the first message. It accepts EHLO/AUTH/MAIL/RCPT/DATA/QUIT.
func fakeSMTPServer(t *testing.T) (addr string, messages chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	messages = make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		w := bufio.NewWriter(conn)
		reply := func(code int, msg string) {
			fmt.Fprintf(w, "%d %s\r\n", code, msg)
			w.Flush()
		}
		reply(220, "localhost ESMTP")
		var payload strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if inData {
				if line == "." {
					inData = false
					reply(250, "2.0.0 queued")
					continue
				}
				payload.WriteString(line)
				payload.WriteString("\n")
				continue
			}
			upper := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(upper, "EHLO"):
				reply(250, "localhost")
			case strings.HasPrefix(upper, "AUTH"):
				reply(235, "2.7.0 ok")
			case strings.HasPrefix(upper, "MAIL FROM"):
				reply(250, "2.1.0 ok")
			case strings.HasPrefix(upper, "RCPT TO"):
				reply(250, "2.1.5 ok")
			case strings.HasPrefix(upper, "DATA"):
				reply(354, "end with .")
				inData = true
			case strings.HasPrefix(upper, "QUIT"):
				reply(221, "2.0.0 bye")
				messages <- payload.String()
				return
			}
		}
	}()
	return ln.Addr().String(), messages
}

func TestEmailNotifierSend(t *testing.T) {
	addr, messages := fakeSMTPServer(t)
	n := &EmailNotifier{from: "me@gmail.com", password: "app-pass", server: addr}

	ev := Event{
		Kind:         EventRunComplete,
		Timestamp:    time.Now(),
		TotalApplied: 3,
	}
	if err := n.Send(context.Background(), ev); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case msg := <-messages:
		if !strings.Contains(msg, "Subject: ⚡ Nexus run complete") {
			t.Errorf("missing subject in:\n%s", msg)
		}
		if !strings.Contains(msg, "Applications submitted: 3") {
			t.Errorf("missing body count in:\n%s", msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the SMTP message")
	}
}

func TestEmailNotifierNoOpWithoutCredentials(t *testing.T) {
	n := &EmailNotifier{} // no from/password
	if err := n.Send(context.Background(), Event{Kind: EventRunComplete}); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestEmailNotifierRender(t *testing.T) {
	n := &EmailNotifier{}
	tests := []struct {
		ev      Event
		wantSub string
		wantOK  bool
	}{
		{ev: Event{Kind: EventJobApplied, JobTitle: "Engineer", Company: "Acme"}, wantSub: "✅ Applied: Engineer @ Acme", wantOK: true},
		{ev: Event{Kind: EventCustom, Title: "Test", Message: "hi"}, wantSub: "Test", wantOK: true},
		{ev: Event{Kind: EventReplyReceived}, wantSub: "", wantOK: false},
	}
	for _, tt := range tests {
		sub, body := n.render(tt.ev)
		if tt.wantOK {
			if sub != tt.wantSub {
				t.Errorf("render(%v) subject = %q; want %q", tt.ev.Kind, sub, tt.wantSub)
			}
			if body == "" {
				t.Errorf("render(%v) empty body", tt.ev.Kind)
			}
		} else if sub != "" {
			t.Errorf("render(%v) subject = %q; want empty", tt.ev.Kind, sub)
		}
	}
}
