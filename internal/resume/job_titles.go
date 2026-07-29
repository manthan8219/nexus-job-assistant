package resume

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SuggestJobTitles asks the LLM to expand a free-text job intent into concrete
// title keywords that match real job-board listings.
func SuggestJobTitles(ctx context.Context, ai AIOptions, intent, yearsExp string, hintRoles []string) ([]string, error) {
	intent = strings.TrimSpace(intent)
	if intent == "" {
		return nil, fmt.Errorf("describe the kind of job you want first")
	}
	if !ai.Enabled {
		return nil, fmt.Errorf("turn on AI Assist in Config first")
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
	}
	raw, err := complete(ctx, ai, jobTitlesPrompt(intent, yearsExp, hintRoles))
	if err != nil {
		return nil, err
	}
	titles, err := parseJobTitles(raw)
	if err != nil {
		return nil, err
	}
	if len(titles) == 0 {
		return nil, fmt.Errorf("AI returned no titles — try a clearer description")
	}
	return titles, nil
}

func jobTitlesPrompt(intent, yearsExp string, hintRoles []string) string {
	yrs := strings.TrimSpace(yearsExp)
	if yrs == "" {
		yrs = "unspecified"
	}
	hints := "none"
	if len(hintRoles) > 0 {
		hints = strings.Join(hintRoles, ", ")
	}
	return fmt.Sprintf(`You help a job seeker pick TARGET JOB TITLE keywords for automated job-board search.

Their intent (what they want to apply for):
"""
%s
"""

Years of experience: %s
Roles already suggested from their resume (optional hints): %s

Return ONLY compact JSON (no markdown):
{"titles":["Title One","Title Two",...]}

Rules:
- 8 to 12 titles max
- Use real listing titles recruiters post (e.g. "Backend Engineer", "Senior Software Engineer", "Platform Engineer")
- Include common synonyms / seniority variants that match the intent
- Prefer searchable keywords over long sentences
- Do NOT invent unrelated domains
- No duplicates; keep each title short (2–5 words typical)
`, intent, yrs, hints)
}

func parseJobTitles(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}
	var doc struct {
		Titles []string `json:"titles"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		// Fallback: try a bare JSON array
		var arr []string
		if err2 := json.Unmarshal([]byte(raw), &arr); err2 != nil {
			return nil, fmt.Errorf("model returned invalid JSON: %w", err)
		}
		doc.Titles = arr
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(doc.Titles))
	for _, t := range doc.Titles {
		t = strings.TrimSpace(t)
		t = strings.Trim(t, "\"'")
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
		if len(out) >= 12 {
			break
		}
	}
	return out, nil
}
