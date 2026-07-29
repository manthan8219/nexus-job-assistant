package resume

import "testing"

func TestParseProfile_JSON(t *testing.T) {
	raw := `{
	  "summary": "Backend engineer with 5 years experience.",
	  "strengths": ["Go", "Distributed systems"],
	  "strength_scores": [{"name":"Go","score":9}],
	  "suitable_roles": ["Backend Engineer"],
	  "role_fit": [{"name":"Backend Engineer","score":9}],
	  "skills": ["Go", "Postgres"],
	  "skill_scores": [{"name":"Go","score":9}],
	  "experience_level": "mid",
	  "years_estimate": 5,
	  "industries": ["SaaS"],
	  "improvements": ["Add quantifiable impact"]
	}`
	p, err := parseProfile(raw)
	if err != nil {
		t.Fatal(err)
	}
	normalizeScores(p)
	if p.Summary == "" || p.ExperienceLevel != "mid" || len(p.StrengthScores) == 0 {
		t.Fatalf("unexpected profile: %+v", p)
	}
}

func TestParseProfile_Fenced(t *testing.T) {
	raw := "```json\n{\"summary\":\"Hi\",\"strengths\":[\"A\"],\"suitable_roles\":[],\"skills\":[],\"experience_level\":\"junior\",\"industries\":[],\"improvements\":[]}\n```"
	p, err := parseProfile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Summary != "Hi" {
		t.Fatalf("got %q", p.Summary)
	}
}

func TestAssertReadable_RejectsJunk(t *testing.T) {
	err := assertReadableResumeText("0 g 0 G cm BT /F1 Td[<0030>] stream endstream " + string(make([]byte, 100)))
	if err == nil {
		t.Fatal("expected reject")
	}
}

func TestExtractUserResumeReadable(t *testing.T) {
	path := "/Users/manthanmanthan/Downloads/Resume.pdf"
	text, err := ExtractText(path)
	if err != nil {
		t.Fatal(err)
	}
	if !containsFold(text, "manthan") && !containsFold(text, "backend") {
		t.Fatalf("expected resume content, got %q", text[:min(200, len(text))])
	}
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		stringIndexFold(s, sub) >= 0)
}

func stringIndexFold(s, sub string) int {
	ls, lsub := len(s), len(sub)
	for i := 0; i+lsub <= ls; i++ {
		if equalFoldASCII(s[i:i+lsub], sub) {
			return i
		}
	}
	return -1
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
