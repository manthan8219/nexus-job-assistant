package resume

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseImproved parses raw model output into an ImprovedDoc, tolerating
// common model drift (education as objects, etc.). Exported for agent
// pipelines outside this package (e.g. internal/tailor) that receive
// ImprovedDoc-shaped JSON from their own writer agents.
func ParseImproved(raw string) (ImprovedDoc, error) {
	return parseImproved(raw)
}

// parseImproved tolerates common model drift (education as objects, etc.).
func parseImproved(raw string) (ImprovedDoc, error) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		return ImprovedDoc{}, fmt.Errorf("model returned invalid JSON: %w", err)
	}

	doc := ImprovedDoc{
		FullName:   flexString(top["full_name"]),
		Headline:   flexString(top["headline"]),
		Summary:    flexString(top["summary"]),
		TargetRole: flexString(top["target_role"]),
		Skills:     flexStringList(top["skills"]),
		Education:  flexEducation(top["education"]),
		Notes:      flexStringList(top["notes"]),
		Experience: flexExperience(top["experience"]),
	}

	if doc.Summary == "" && len(doc.Experience) == 0 {
		return ImprovedDoc{}, fmt.Errorf("empty improved resume from model")
	}
	return doc, nil
}

func flexString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	// number / bool → stringify
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return strings.TrimSpace(string(raw))
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strings.TrimSpace(fmt.Sprint(t))
	case map[string]any:
		// {"text": "..."} style
		for _, k := range []string{"text", "value", "name", "title"} {
			if s, ok := t[k].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func flexStringList(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	// Already []string
	var ss []string
	if err := json.Unmarshal(raw, &ss); err == nil {
		return cleanStrings(ss)
	}
	// []any with mixed
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		// single string
		if s := flexString(raw); s != "" {
			return []string{s}
		}
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s := flexString(item); s != "" {
			out = append(out, s)
			continue
		}
		// object → join useful fields
		var obj map[string]any
		if json.Unmarshal(item, &obj) == nil {
			if line := joinEduObj(obj); line != "" {
				out = append(out, line)
			}
		}
	}
	return out
}

func flexEducation(raw json.RawMessage) []string {
	return flexStringList(raw)
}

func joinEduObj(obj map[string]any) string {
	keys := []string{"degree", "school", "university", "institution", "field", "major", "year", "years", "period", "location", "name", "title", "text"}
	var parts []string
	seen := map[string]bool{}
	for _, k := range keys {
		v, ok := obj[k]
		if !ok {
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" || s == "<nil>" || seen[s] {
			continue
		}
		seen[s] = true
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		// fallback: any string values
		for _, v := range obj {
			if s, ok := v.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					parts = append(parts, s)
				}
			}
		}
	}
	return strings.Join(parts, ", ")
}

func flexExperience(raw json.RawMessage) []ImprovedRole {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	out := make([]ImprovedRole, 0, len(arr))
	for _, item := range arr {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(item, &obj); err != nil {
			continue
		}
		role := ImprovedRole{
			Title:   firstFlex(obj, "title", "role", "position"),
			Org:     firstFlex(obj, "org", "organization", "company", "employer"),
			Period:  firstFlex(obj, "period", "dates", "date", "duration"),
			Bullets: flexStringList(obj["bullets"]),
		}
		if len(role.Bullets) == 0 {
			role.Bullets = flexStringList(obj["achievements"])
		}
		if len(role.Bullets) == 0 {
			role.Bullets = flexStringList(obj["highlights"])
		}
		if role.Title == "" && role.Org == "" && len(role.Bullets) == 0 {
			continue
		}
		out = append(out, role)
	}
	return out
}

func firstFlex(obj map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		if s := flexString(obj[k]); s != "" {
			return s
		}
	}
	return ""
}

func cleanStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
