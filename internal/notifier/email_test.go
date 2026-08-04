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
		for _, want := range []string{"Applied:", "Failed:", "Skipped:", "Duration:"} {
			if !strings.Contains(msg, want) {
				t.Errorf("missing %q in enriched run-complete body:\n%s", want, msg)
			}
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
		{ev: Event{Kind: EventJobFailed, JobTitle: "Engineer", Company: "Acme", Reason: "captcha"}, wantSub: "❌ Application failed: Engineer", wantOK: true},
		{ev: Event{Kind: EventRunComplete, TotalApplied: 3}, wantSub: "⚡ Nexus run complete", wantOK: true},
		{ev: Event{Kind: EventDailySummary, TotalApplied: 3, TotalFailed: 1, TotalSkipped: 2}, wantSub: "📊 Nexus daily summary", wantOK: true},
		{ev: Event{Kind: EventWeeklySummary, TotalApplied: 10, TotalFailed: 2, TotalSkipped: 5}, wantSub: "📊 Nexus weekly summary", wantOK: true},
		{ev: Event{Kind: EventCustom, Title: "Test", Message: "hi"}, wantSub: "Test", wantOK: true},
		{ev: Event{Kind: EventReplyReceived, ReplyFrom: "jane@acme.com", ReplySubject: "Re: intro"}, wantSub: "📩 Reply received: Re: intro", wantOK: true},
		{ev: Event{Kind: EventRunStarted}, wantSub: "", wantOK: false},
		{ev: Event{Kind: EventCAPTCHA, JobTitle: "Engineer", Company: "Acme", CAPTCHAURL: "https://acme.example.com/job/1"}, wantSub: "⛔ CAPTCHA — complete manually to continue", wantOK: true},
		{ev: Event{Kind: EventError, Message: "boom"}, wantSub: "🚨 Nexus error", wantOK: true},
	}
	for _, tt := range tests {
		t.Run(string(tt.ev.Kind), func(t *testing.T) {
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
		})
	}
}

// TestEmailNotifierRichBodies verifies the enriched, information-dense body
// content: job URL, board, posted date, and honest dry-run wording.
func TestEmailNotifierRichBodies(t *testing.T) {
	n := &EmailNotifier{}
	posted := time.Now().Add(-48 * time.Hour)

	tests := []struct {
		name    string
		ev      Event
		wantSub string
		bodyHas []string
	}{
		{
			name: "applied email carries the apply link and board",
			ev: Event{
				Kind: EventJobApplied, JobTitle: "Engineer", Company: "Acme",
				Location: "Remote", Provider: "reed", Board: "reed",
				JobURL: "https://www.reed.co.uk/jobs/123", PostedAt: posted,
			},
			wantSub: "✅ Applied: Engineer @ Acme",
			bodyHas: []string{"Company:", "Acme", "Source:", "reed", "Posted:", "ago", "Apply page:", "https://www.reed.co.uk/jobs/123"},
		},
		{
			name: "failed email carries the reason and manual link",
			ev: Event{
				Kind: EventJobFailed, JobTitle: "Engineer", Company: "Acme",
				Provider: "lever", Board: "lever", Reason: "captcha required",
				JobURL: "https://jobs.lever.co/acme/1",
			},
			wantSub: "❌ Application failed: Engineer",
			bodyHas: []string{"Reason:", "captcha required", "apply manually", "https://jobs.lever.co/acme/1"},
		},
		{
			name: "dry-run digest is honest about zero submissions",
			ev: Event{
				Kind: EventRunComplete, DryRun: true, Scanned: 12, Found: 12,
				RunDuration: 90 * time.Second,
			},
			wantSub: "⚡ Daily job digest — 12 jobs found",
			bodyHas: []string{"Scraped:", "12", "Submitted:", "0", "dry run", "Nothing was submitted"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, body := n.render(tt.ev)
			if sub != tt.wantSub {
				t.Errorf("subject = %q; want %q", sub, tt.wantSub)
			}
			for _, want := range tt.bodyHas {
				if !strings.Contains(body, want) {
					t.Errorf("body missing %q:\n%s", want, body)
				}
			}
		})
	}
}

// TestEmailNotifierSendDailySummary verifies the daily-summary email that the
// dashboard's "notify summary" action delivers: subject "📊 Nexus daily
// summary" with the Applied/Failed/Skipped body line.
func TestEmailNotifierSendDailySummary(t *testing.T) {
	addr, messages := fakeSMTPServer(t)
	n := &EmailNotifier{from: "me@gmail.com", password: "app-pass", server: addr}

	ev := Event{
		Kind:         EventDailySummary,
		Timestamp:    time.Now(),
		TotalApplied: 3,
		TotalFailed:  1,
		TotalSkipped: 2,
	}
	if err := n.Send(context.Background(), ev); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case msg := <-messages:
		if !strings.Contains(msg, "Subject: 📊 Nexus daily summary") {
			t.Errorf("missing subject in:\n%s", msg)
		}
		for _, want := range []string{"Applied:", "Failed:", "Skipped:", "Generated:"} {
			if !strings.Contains(msg, want) {
				t.Errorf("missing %q in enriched summary body:\n%s", want, msg)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the SMTP message")
	}
}

// TestEmailNotifierDigestListsJobs verifies the consolidated digest lists the
// applied jobs and the failed jobs needing manual action — one email instead
// of one per job.
func TestEmailNotifierDigestListsJobs(t *testing.T) {
	n := &EmailNotifier{}
	sub, body := n.render(Event{
		Kind:         EventRunComplete,
		Timestamp:    time.Now(),
		TotalApplied: 2,
		TotalFailed:  1,
		Jobs: []JobEvent{
			{Title: "Engineer", Company: "Acme", URL: "https://acme.example.com/1", Status: "applied"},
			{Title: "Backend", Company: "Beta", URL: "https://beta.example.com/2", Status: "applied"},
			{Title: "Platform", Company: "Gamma", URL: "https://gamma.example.com/3", Status: "failed", Reason: "captcha"},
		},
	})
	if sub != "⚡ Nexus run complete" {
		t.Errorf("subject = %q", sub)
	}
	for _, want := range []string{"✓ Applied:", "Engineer @ Acme", "https://acme.example.com/1", "Backend @ Beta", "✗ Needs manual action:", "Platform @ Gamma", "reason: captcha", "https://gamma.example.com/3"} {
		if !strings.Contains(body, want) {
			t.Errorf("digest body missing %q:\n%s", want, body)
		}
	}
}

// TestEmailNotifierConsolidatesByDefault proves per-job applied/failed events
// produce no individual email (the digest at run_complete is the only email).
func TestEmailNotifierConsolidatesByDefault(t *testing.T) {
	addr, messages := fakeSMTPServer(t)
	n := &EmailNotifier{from: "me@gmail.com", password: "app-pass", server: addr}

	if err := n.Send(context.Background(), Event{Kind: EventJobApplied, JobTitle: "Engineer", Company: "Acme"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := n.Send(context.Background(), Event{Kind: EventJobFailed, JobTitle: "Engineer", Company: "Acme", Reason: "captcha"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	select {
	case <-messages:
		t.Fatal("per-job email was sent when only the digest should be")
	case <-time.After(300 * time.Millisecond):
		// expected: nothing queued
	}
}

// TestEmailNotifierPerJobOptIn proves the EmailPerJob toggle restores
// individual applied emails.
func TestEmailNotifierPerJobOptIn(t *testing.T) {
	addr, messages := fakeSMTPServer(t)
	n := &EmailNotifier{from: "me@gmail.com", password: "app-pass", server: addr, perJob: true}

	if err := n.Send(context.Background(), Event{Kind: EventJobApplied, JobTitle: "Engineer", Company: "Acme"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	select {
	case msg := <-messages:
		if !strings.Contains(msg, "Subject: ✅ Applied: Engineer @ Acme") {
			t.Errorf("unexpected subject in:\n%s", msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the per-job email")
	}
}

// TestEmailNotifierInstantAlerts verifies CAPTCHA / reply / error are emailed
// immediately (they need attention, unlike routine outcomes).
func TestEmailNotifierInstantAlerts(t *testing.T) {
	for _, tt := range []struct {
		name    string
		ev      Event
		wantSub string
	}{
		{"captcha", Event{Kind: EventCAPTCHA, JobTitle: "Engineer", Company: "Acme", CAPTCHAURL: "https://acme.example.com/1"}, "⛔ CAPTCHA — complete manually to continue"},
		{"reply", Event{Kind: EventReplyReceived, ReplyFrom: "jane@acme.com", ReplySubject: "Re: intro"}, "📩 Reply received: Re: intro"},
		{"error", Event{Kind: EventError, Message: "engine init failed"}, "🚨 Nexus error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			addr, messages := fakeSMTPServer(t)
			n := &EmailNotifier{from: "me@gmail.com", password: "app-pass", server: addr}
			if err := n.Send(context.Background(), tt.ev); err != nil {
				t.Fatalf("Send: %v", err)
			}
			select {
			case msg := <-messages:
				if !strings.Contains(msg, "Subject: "+tt.wantSub) {
					t.Errorf("unexpected subject in:\n%s", msg)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for the alert email")
			}
		})
	}
}

// TestEmailNotifierHTMLBody verifies the styled HTML part of the digest: it is
// present in the multipart message, carries the stat cards, the job links, the
// section headings, and HTML-escapes scraped content.
func TestEmailNotifierHTMLBody(t *testing.T) {
	addr, messages := fakeSMTPServer(t)
	n := &EmailNotifier{from: "me@gmail.com", password: "app-pass", server: addr}

	ev := Event{
		Kind:         EventRunComplete,
		Timestamp:    time.Now(),
		TotalApplied: 1,
		TotalFailed:  1,
		Found:        3,
		Jobs: []JobEvent{
			{Title: "Engineer & Analyst", Company: "Acme", URL: "https://acme.example.com/1", Status: "applied"},
			{Title: "Platform", Company: "Beta", URL: "https://beta.example.com/2", Status: "failed", Reason: "captcha"},
		},
	}
	if err := n.Send(context.Background(), ev); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case msg := <-messages:
		if !strings.Contains(msg, "Content-Type: multipart/alternative") {
			t.Errorf("message is not multipart/alternative:\n%.400s", msg)
		}
		if !strings.Contains(msg, "Content-Type: text/html; charset=utf-8") {
			t.Errorf("message has no HTML part:\n%.400s", msg)
		}
		for _, want := range []string{
			"<!DOCTYPE html>", "⚡ Nexus — Run complete",
			"scraped", "applied", "failed",
			"https://acme.example.com/1", "href=", "<body", "text/html",
			"Engineer &amp; Analyst", // escaped
		} {
			if !strings.Contains(msg, want) {
				t.Errorf("HTML body missing %q:\n%.800s", want, msg)
			}
		}
		if !strings.Contains(msg, "Engineer &amp; Analyst") {
			t.Errorf("scraped title must be HTML-escaped")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the SMTP message")
	}
}

// TestEmailNotifierHTMLDryDigest verifies the daily-nudge HTML digest lists
// the matched jobs and carries the "scraped / matched / submitted" stat labels.
func TestEmailNotifierHTMLDryDigest(t *testing.T) {
	addr, messages := fakeSMTPServer(t)
	n := &EmailNotifier{from: "me@gmail.com", password: "app-pass", server: addr}

	ev := Event{
		Kind:      EventRunComplete,
		Timestamp: time.Now(),
		DryRun:    true,
		Found:     2,
		Scanned:   2,
		Jobs: []JobEvent{
			{Title: "Go Developer", Company: "GitLab", URL: "https://boards.greenhouse.io/gitlab/jobs/7", Status: "found"},
		},
	}
	if err := n.Send(context.Background(), ev); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case msg := <-messages:
		if !strings.Contains(msg, "Subject: ⚡ Daily job digest — 2 jobs found") {
			t.Errorf("subject wrong:\n%.300s", msg)
		}
		for _, want := range []string{"Matched — dry run", "Go Developer", "https://boards.greenhouse.io/gitlab/jobs/7", "scraped", "matched", "submitted"} {
			if !strings.Contains(msg, want) {
				t.Errorf("dry digest HTML missing %q", want)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the SMTP message")
	}
}
