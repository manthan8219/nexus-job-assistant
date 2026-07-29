package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── Unit: Payload builders ───────────────────────────────────────────────────

func TestDiscord_JobApplied_ProducesValidPayload(t *testing.T) {
	d := NewDiscordNotifier("https://discord.com/api/webhooks/xxx")
	ev := Event{
		Kind:     EventJobApplied,
		JobTitle: "Software Engineer",
		Company:  "Acme Corp",
		Location: "Remote",
		Provider: "greenhouse",
	}

	payload := d.buildPayload(ev)
	if payload == nil {
		t.Fatal("expected non-nil payload")
	}
	if len(payload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(payload.Embeds))
	}
	e := payload.Embeds[0]
	if e.Color != colorGreen {
		t.Fatalf("expected green color %d, got %d", colorGreen, e.Color)
	}
	if !strings.Contains(e.Description, "Acme Corp") {
		t.Fatalf("expected company in desc, got: %s", e.Description)
	}
	if !strings.Contains(e.Description, "Remote") {
		t.Fatalf("expected location in desc, got: %s", e.Description)
	}
}

func TestDiscord_JobFailed_IncludesReason(t *testing.T) {
	d := NewDiscordNotifier("https://discord.com/api/webhooks/xxx")
	ev := Event{
		Kind:    EventJobFailed,
		JobTitle: "Backend Engineer",
		Company:  "Startup Inc",
		Reason:   "Resume not accepted",
	}

	payload := d.buildPayload(ev)
	if payload == nil {
		t.Fatal("expected non-nil payload")
	}
	e := payload.Embeds[0]
	if e.Color != colorRed {
		t.Fatalf("expected red color %d, got %d", colorRed, e.Color)
	}
	found := false
	for _, f := range e.Fields {
		if f.Name == "Reason" && f.Value == "Resume not accepted" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected Reason field in failed embed")
	}
}

func TestDiscord_CAPTCHA_IncludesURL(t *testing.T) {
	d := NewDiscordNotifier("https://discord.com/api/webhooks/xxx")
	ev := Event{
		Kind:       EventCAPTCHA,
		CAPTCHAURL: "https://example.com/captcha",
	}

	payload := d.buildPayload(ev)
	if payload == nil {
		t.Fatal("expected non-nil payload")
	}
	e := payload.Embeds[0]
	if e.Color != colorOrange {
		t.Fatalf("expected orange color %d, got %d", colorOrange, e.Color)
	}
	if !strings.Contains(e.Description, "example.com") {
		t.Fatalf("expected captcha URL in desc, got: %s", e.Description)
	}
}

func TestDiscord_CustomEvent(t *testing.T) {
	d := NewDiscordNotifier("https://discord.com/api/webhooks/xxx")
	ev := Event{
		Kind:    EventCustom,
		Title:   "Test Alert",
		Message: "This is a test notification",
		Fields:  map[string]string{"Key1": "Value1", "Key2": "Value2"},
	}

	payload := d.buildPayload(ev)
	if payload == nil {
		t.Fatal("expected non-nil payload")
	}
	e := payload.Embeds[0]
	if e.Title != "Test Alert" {
		t.Fatalf("expected title 'Test Alert', got: %s", e.Title)
	}
	if len(e.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(e.Fields))
	}
}

func TestDiscord_RunCompletePayload(t *testing.T) {
	d := NewDiscordNotifier("https://discord.com/api/webhooks/xxx")
	ev := Event{
		Kind:         EventRunComplete,
		TotalApplied: 5,
		TotalFailed:  1,
		TotalSkipped: 3,
		RunDuration:  2*time.Minute + 30*time.Second,
	}

	payload := d.buildPayload(ev)
	if payload == nil {
		t.Fatal("expected non-nil payload")
	}
	e := payload.Embeds[0]
	if !strings.Contains(e.Description, "5") {
		t.Fatalf("expected applied count in desc, got: %s", e.Description)
	}
	if !strings.Contains(e.Description, "2m 30s") {
		t.Fatalf("expected duration in desc, got: %s", e.Description)
	}
}

func TestDiscord_EmptyWebhook_IsNoop(t *testing.T) {
	d := NewDiscordNotifier("")
	err := d.Send(context.Background(), Event{Kind: EventJobApplied})
	if err != nil {
		t.Fatalf("expected no error from no-op, got: %v", err)
	}
}

func TestDiscord_UsernameOverride(t *testing.T) {
	d := NewDiscordNotifier("https://discord.com/api/webhooks/xxx")
	d.SetUsername("Nexus Bot")
	ev := Event{Kind: EventCustom, Message: "hello"}
	payload := d.buildPayload(ev)
	if payload.Username != "Nexus Bot" {
		t.Fatalf("expected username 'Nexus Bot', got: %s", payload.Username)
	}
}

// ── Integration: HTTP server round-trip ──────────────────────────────────────

func TestDiscord_Send_HappyPath(t *testing.T) {
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf)
		receivedBody = buf
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	d := NewDiscordNotifier(ts.URL)
	ev := Event{
		Kind:     EventJobApplied,
		JobTitle: "DevOps Engineer",
		Company:  "Cloud Co",
	}

	err := d.Send(context.Background(), ev)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var payload discordWebhookPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("expected valid JSON, got: %v", err)
	}
	if len(payload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(payload.Embeds))
	}
	if payload.Embeds[0].Title != "✅ Application Submitted" {
		t.Fatalf("unexpected embed title: %s", payload.Embeds[0].Title)
	}
}

func TestDiscord_Send_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	d := NewDiscordNotifier(ts.URL)
	ev := Event{Kind: EventJobApplied}
	err := d.Send(context.Background(), ev)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}

func TestDiscord_Send_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server does not support hijacking")
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer ts.Close()

	d := NewDiscordNotifier(ts.URL)
	ev := Event{Kind: EventJobApplied}
	err := d.Send(context.Background(), ev)
	if err == nil {
		t.Fatal("expected error for closed connection")
	}
}

// ── MultiNotifier tests ──────────────────────────────────────────────────────

type mockNotifier struct {
	name string
	send func(ctx context.Context, ev Event) error
}

func (m *mockNotifier) Name() string  { return m.name }
func (m *mockNotifier) Send(ctx context.Context, ev Event) error { return m.send(ctx, ev) }

func TestMultiNotifier_FansOut(t *testing.T) {
	var calls []string
	n1 := &mockNotifier{
		name: "n1",
		send: func(ctx context.Context, ev Event) error {
			calls = append(calls, "n1")
			return nil
		},
	}
	n2 := &mockNotifier{
		name: "n2",
		send: func(ctx context.Context, ev Event) error {
			calls = append(calls, "n2")
			return nil
		},
	}

	mn := MultiNotifier{n1, n2}
	errs := mn.Send(context.Background(), Event{Kind: EventCustom})
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
}

func TestMultiNotifier_EmptyIsNoop(t *testing.T) {
	mn := MultiNotifier{}
	errs := mn.Send(context.Background(), Event{Kind: EventCustom})
	if len(errs) != 0 {
		t.Fatalf("expected no errors from empty MultiNotifier, got: %v", errs)
	}
}
