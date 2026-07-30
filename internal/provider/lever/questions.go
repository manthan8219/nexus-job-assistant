package lever

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Question is one custom application question extracted from a Lever
// apply page's embedded card schema.
type Question struct {
	FieldName string // e.g. "cards[<cardId>][field2]" — the POST field name
	Type      string // "text" | "textarea" | "dropdown" | "multiple-select" | ...
	Text      string // the question prompt shown to the applicant
	Required  bool
	Options   []string // choice text for dropdown/multiple-select; empty for free text
}

// FormInfo is everything discovered about a Lever job's apply form.
type FormInfo struct {
	Questions       []Question
	RequiresCaptcha bool // hCaptcha present — cannot be solved by a plain HTTP POST
}

// cardField mirrors the JSON Lever embeds in each hidden
// `cards[<id>][baseTemplate]` input's value attribute.
type cardField struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Required bool   `json:"required"`
	ID       string `json:"id"`
	Options  []struct {
		Text     string `json:"text"`
		OptionID string `json:"optionId"`
	} `json:"options"`
}

type cardTemplate struct {
	Fields []cardField `json:"fields"`
}

var cardInputRE = regexp.MustCompile(
	`(?s)<input[^>]*value="([^"]*)"[^>]*name="cards\[([a-zA-Z0-9-]+)\]\[baseTemplate\]"`)

// FetchFormInfo fetches a Lever job's public apply page and extracts its
// custom question schema plus whether it requires solving hCaptcha.
// This is a read-only GET — it does not submit anything.
func FetchFormInfo(board, jobID string) (*FormInfo, error) {
	applyURL := fmt.Sprintf("https://jobs.lever.co/%s/%s/apply", board, jobID)

	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodGet, applyURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("apply page HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	rawHTML := string(body)

	info := &FormInfo{
		RequiresCaptcha: strings.Contains(rawHTML, "h-captcha-response"),
	}

	for _, m := range cardInputRE.FindAllStringSubmatch(rawHTML, -1) {
		encodedJSON, cardID := m[1], m[2]
		decoded := html.UnescapeString(encodedJSON)
		var tmpl cardTemplate
		if err := json.Unmarshal([]byte(decoded), &tmpl); err != nil {
			continue // skip cards we can't parse rather than fail the whole page
		}
		for i, f := range tmpl.Fields {
			q := Question{
				FieldName: fmt.Sprintf("cards[%s][field%d]", cardID, i),
				Type:      f.Type,
				Text:      strings.TrimSpace(f.Text),
				Required:  f.Required,
			}
			for _, o := range f.Options {
				q.Options = append(q.Options, o.Text)
			}
			if q.Text != "" {
				info.Questions = append(info.Questions, q)
			}
		}
	}
	return info, nil
}
