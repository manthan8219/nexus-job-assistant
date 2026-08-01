package lever

import (
	"html"
	"net/http"
	"net/http/httptest"
	"testing"
)

// cardInput builds the hidden Lever card input HTML for one card, with the
// given card ID and baseTemplate JSON (HTML-escaped so it round-trips through
// the cardInputRE capture + html.UnescapeString in FetchFormInfo).
func cardInput(cardID, cardJSON string) string {
	return `<input type="hidden" value="` + html.EscapeString(cardJSON) + `" name="cards[` + cardID + `][baseTemplate]">`
}

func TestFetchFormInfo(t *testing.T) {
	cardJSON := `{"fields":[{"type":"text","text":"Why this role?","required":true,"id":"f1"}]}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("<html>" + cardInput("abc-123", cardJSON) + "</html>"))
	}))
	defer ts.Close()

	orig := leverApplyURLFmt
	leverApplyURLFmt = ts.URL + "/%s/%s/apply"
	defer func() { leverApplyURLFmt = orig }()

	info, err := FetchFormInfo("acme", "123")
	if err != nil {
		t.Fatalf("FetchFormInfo: %v", err)
	}
	if info.RequiresCaptcha {
		t.Error("expected RequiresCaptcha=false")
	}
	if len(info.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(info.Questions))
	}
	q := info.Questions[0]
	if q.Text != "Why this role?" {
		t.Errorf("text = %q; want \"Why this role?\"", q.Text)
	}
	if !q.Required {
		t.Error("expected required=true")
	}
	if q.Type != "text" {
		t.Errorf("type = %q; want \"text\"", q.Type)
	}
	if q.FieldName != "cards[abc-123][field0]" {
		t.Errorf("fieldName = %q; want \"cards[abc-123][field0]\"", q.FieldName)
	}
}

func TestFetchFormInfo_DropdownOptions(t *testing.T) {
	cardJSON := `{"fields":[{"type":"dropdown","text":"Authorized?","options":[{"text":"Yes","optionId":"o1"},{"text":"No","optionId":"o2"}]}]}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("<html>" + cardInput("c1", cardJSON) + "</html>"))
	}))
	defer ts.Close()

	orig := leverApplyURLFmt
	leverApplyURLFmt = ts.URL + "/%s/%s/apply"
	defer func() { leverApplyURLFmt = orig }()

	info, err := FetchFormInfo("acme", "1")
	if err != nil {
		t.Fatalf("FetchFormInfo: %v", err)
	}
	if len(info.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(info.Questions))
	}
	if len(info.Questions[0].Options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(info.Questions[0].Options))
	}
	if info.Questions[0].Options[0] != "Yes" || info.Questions[0].Options[1] != "No" {
		t.Errorf("options = %v; want [Yes No]", info.Questions[0].Options)
	}
}

// AGENTS.md section 14: hCaptcha detection is a hard stop — automated apply
// must halt and surface to the user. This test guards that detection.
func TestFetchFormInfo_CaptchaDetected(t *testing.T) {
	cardJSON := `{"fields":[{"type":"text","text":"Q?","id":"f1"}]}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("<html>" + cardInput("c1", cardJSON) + `<div class="h-captcha-response"></div></html>`))
	}))
	defer ts.Close()

	orig := leverApplyURLFmt
	leverApplyURLFmt = ts.URL + "/%s/%s/apply"
	defer func() { leverApplyURLFmt = orig }()

	info, err := FetchFormInfo("acme", "1")
	if err != nil {
		t.Fatalf("FetchFormInfo: %v", err)
	}
	if !info.RequiresCaptcha {
		t.Error("expected RequiresCaptcha=true when h-captcha-response is present")
	}
}

func TestFetchFormInfo_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	defer ts.Close()

	orig := leverApplyURLFmt
	leverApplyURLFmt = ts.URL + "/%s/%s/apply"
	defer func() { leverApplyURLFmt = orig }()

	if _, err := FetchFormInfo("acme", "1"); err == nil {
		t.Fatal("expected error for 404 apply page")
	}
}

func TestFetchFormInfo_MalformedCardSkipped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("<html>" + cardInput("c1", "not json") + "</html>"))
	}))
	defer ts.Close()

	orig := leverApplyURLFmt
	leverApplyURLFmt = ts.URL + "/%s/%s/apply"
	defer func() { leverApplyURLFmt = orig }()

	info, err := FetchFormInfo("acme", "1")
	if err != nil {
		t.Fatalf("FetchFormInfo: %v", err)
	}
	if len(info.Questions) != 0 {
		t.Errorf("malformed card should yield 0 questions, got %d", len(info.Questions))
	}
}

func TestFetchFormInfo_MultipleCards(t *testing.T) {
	c1 := `{"fields":[{"type":"text","text":"First?","id":"f1"}]}`
	c2 := `{"fields":[{"type":"textarea","text":"Second?","id":"f2"}]}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("<html>" + cardInput("c1", c1) + cardInput("c2", c2) + "</html>"))
	}))
	defer ts.Close()

	orig := leverApplyURLFmt
	leverApplyURLFmt = ts.URL + "/%s/%s/apply"
	defer func() { leverApplyURLFmt = orig }()

	info, err := FetchFormInfo("acme", "1")
	if err != nil {
		t.Fatalf("FetchFormInfo: %v", err)
	}
	if len(info.Questions) != 2 {
		t.Fatalf("expected 2 questions, got %d", len(info.Questions))
	}
	if info.Questions[0].Text != "First?" || info.Questions[1].Text != "Second?" {
		t.Errorf("texts = %q, %q; want \"First?\", \"Second?\"", info.Questions[0].Text, info.Questions[1].Text)
	}
	if info.Questions[0].FieldName != "cards[c1][field0]" || info.Questions[1].FieldName != "cards[c2][field0]" {
		t.Errorf("fieldNames = %q, %q", info.Questions[0].FieldName, info.Questions[1].FieldName)
	}
}

func TestFetchFormInfo_EmptyTextSkipped(t *testing.T) {
	cardJSON := `{"fields":[{"type":"text","text":"","id":"f1"}]}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("<html>" + cardInput("c1", cardJSON) + "</html>"))
	}))
	defer ts.Close()

	orig := leverApplyURLFmt
	leverApplyURLFmt = ts.URL + "/%s/%s/apply"
	defer func() { leverApplyURLFmt = orig }()

	info, err := FetchFormInfo("acme", "1")
	if err != nil {
		t.Fatalf("FetchFormInfo: %v", err)
	}
	if len(info.Questions) != 0 {
		t.Errorf("empty-text field should be skipped, got %d questions", len(info.Questions))
	}
}
