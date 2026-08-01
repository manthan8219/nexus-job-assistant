package resume

import "testing"

func TestParseProfile_JSON(t *testing.T) {
	raw := `{
	  "summary": "Backend engineer with 5 years experience.",
	  "strengths": ["Go", "Distributed systems"],
	  "strengthScores": [{"name":"Go","score":9}],
	  "suitableRoles": ["Backend Engineer"],
	  "roleFit": [{"name":"Backend Engineer","score":9}],
	  "skills": ["Go", "Postgres"],
	  "skillScores": [{"name":"Go","score":9}],
	  "experienceLevel": "mid",
	  "yearsEstimate": 5,
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

func TestParseProfile_SanitizesImageRefs(t *testing.T) {
	raw := `{
	  "summary": "Backend engineer. See [Image 1] for skills chart.",
	  "whatsGood": ["Clear progression [Chart 2]", "Strong Go experience"],
	  "whatsWrong": ["[Figure 3] Missing metrics"],
	  "strengths": ["Go systems design"],
	  "strengthScores": [{"name":"Go","score":9}],
	  "suitableRoles": ["Backend Engineer"],
	  "roleFit": [{"name":"Backend Engineer","score":9}],
	  "skills": ["Go", "Postgres"],
	  "skillScores": [{"name":"Go","score":9}],
	  "experienceLevel": "mid",
	  "yearsEstimate": 5,
	  "industries": ["SaaS"],
	  "improvements": ["Add [Table 4] quantifiable impact"]
	}`
	p, err := parseProfile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Summary != "Backend engineer. See for skills chart." {
		t.Errorf("summary not sanitized: %q", p.Summary)
	}
	if len(p.WhatsGood) != 2 || p.WhatsGood[0] != "Clear progression" {
		t.Errorf("whatsGood not sanitized: %#v", p.WhatsGood)
	}
	if len(p.WhatsWrong) != 1 || p.WhatsWrong[0] != "Missing metrics" {
		t.Errorf("whatsWrong not sanitized: %#v", p.WhatsWrong)
	}
	if len(p.Improvements) != 1 || p.Improvements[0] != "Add quantifiable impact" {
		t.Errorf("improvements not sanitized: %#v", p.Improvements)
	}
}

func TestParseProfile_Fenced(t *testing.T) {
	raw := "```json\n{\"summary\":\"Hi\",\"strengths\":[\"A\"],\"suitableRoles\":[],\"skills\":[],\"experienceLevel\":\"junior\",\"industries\":[],\"improvements\":[]}\n```"
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
	// reason: requires a real sample resume PDF, but none is committed because
	// resumes contain personal data (AGENTS.md §14). ExtractText is instead
	// exercised hermetically by the parse/fit tests via in-memory text. Replace
	// this skip with a t.TempDir()-generated fixture (gofpdf) when feasible.
	t.Skip("no committed sample resume PDF fixture — kept hermetic")
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
