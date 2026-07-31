package agentx

import "testing"

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"clean object", `{"a":1}`, `{"a":1}`},
		{"fenced", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"fenced bare", "```\n{\"a\":1}\n```", `{"a":1}`},
		{"prose around", `Here is the review: {"a":1} hope that helps`, `{"a":1}`},
		{"nested widest span", `{"o":{"b":[1,2]},"z":0}`, `{"o":{"b":[1,2]},"z":0}`},
		{"no json", "no object here", ""},
		{"empty", "", ""},
		{"only opening", `{ "a": `, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractJSON(tt.raw); got != tt.want {
				t.Errorf("ExtractJSON(%q) = %q; want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseJSON(t *testing.T) {
	type doc struct {
		Name  string   `json:"name"`
		Score int      `json:"score"`
		Tags  []string `json:"tags"`
	}

	t.Run("valid with prose", func(t *testing.T) {
		got, err := ParseJSON[doc](`Sure! {"name":"ada","score":7,"tags":["go"]}`)
		if err != nil {
			t.Fatalf("ParseJSON error: %v", err)
		}
		if got.Name != "ada" || got.Score != 7 || len(got.Tags) != 1 {
			t.Errorf("ParseJSON = %+v; want ada/7/1 tag", got)
		}
	})

	t.Run("no json", func(t *testing.T) {
		if _, err := ParseJSON[doc]("plain text"); err == nil {
			t.Fatal("ParseJSON(plain text) expected error, got nil")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		if _, err := ParseJSON[doc](`{"name": 123abc}`); err == nil {
			t.Fatal("ParseJSON(invalid) expected error, got nil")
		}
	})
}
