package resume

import "testing"

func TestRenderMarkdownAndLaTeX(t *testing.T) {
	doc := ImprovedDoc{
		FullName: "Ada Lovelace",
		Headline: "Systems Engineer",
		Summary:  "Builds reliable platforms.",
		Skills:   []string{"Go", "SQL"},
		Experience: []ImprovedRole{{
			Title: "Engineer", Org: "Analytical Engine", Period: "1843",
			Bullets: []string{"Designed algorithms", "Documented the machine"},
		}},
		Education: []string{"Self-taught mathematics"},
	}
	md := RenderMarkdown(doc)
	if !containsAll(md, "Ada Lovelace", "Systems Engineer", "Go · SQL", "Analytical Engine") {
		t.Fatalf("markdown missing pieces:\n%s", md)
	}
	tex := RenderLaTeX(doc)
	if !containsAll(tex, `Ada Lovelace`, `Systems Engineer`, `\begin{document}`) {
		t.Fatalf("latex missing pieces:\n%s", tex)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
