// Package agentx is the shared agent foundation for Nexus on top of the Eino
// framework: one LLM call with a typed input, a persona prompt, and a typed
// JSON output. Every Nexus agent (HR reviewer, CV writer, outreach writer, …)
// is built from these primitives so agents stay small and testable.
package agentx

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExtractJSON returns the widest {...} span in raw model output, stripping
// markdown fences and surrounding prose. It returns "" when no JSON object
// is present.
func ExtractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```JSON")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	i := strings.Index(raw, "{")
	j := strings.LastIndex(raw, "}")
	if i < 0 || j <= i {
		return ""
	}
	return raw[i : j+1]
}

// ParseJSON tolerantly unmarshals raw model output into T: fences and prose
// around the JSON object are ignored via ExtractJSON.
func ParseJSON[T any](raw string) (T, error) {
	var out T
	span := ExtractJSON(raw)
	if span == "" {
		return out, fmt.Errorf("agentx: no JSON object found in model output")
	}
	if err := json.Unmarshal([]byte(span), &out); err != nil {
		return out, fmt.Errorf("agentx: model returned invalid JSON: %w", err)
	}
	return out, nil
}
