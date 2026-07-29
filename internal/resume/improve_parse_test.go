package resume

import "testing"

func TestParseImproved_EducationObjects(t *testing.T) {
	raw := `{
  "full_name": "Manthan Bhatia",
  "headline": "Backend Engineer",
  "summary": "Ships services.",
  "skills": ["Java", {"name": "Spring Boot"}],
  "experience": [{
    "title": "Engineer",
    "org": "Acme",
    "period": "2023 – Present",
    "bullets": ["Built APIs", {"text": "Owned on-call"}]
  }],
  "education": [
    {"degree": "B.Tech CSE", "university": "Example University", "year": "2021"},
    "Online course in distributed systems"
  ],
  "notes": ["Fixed education shape"],
  "target_role": "Senior Backend Engineer"
}`
	doc, err := parseImproved(raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc.FullName != "Manthan Bhatia" {
		t.Fatalf("name: %q", doc.FullName)
	}
	if len(doc.Education) < 2 {
		t.Fatalf("education: %#v", doc.Education)
	}
	if doc.Education[0] == "" || doc.Education[1] == "" {
		t.Fatalf("empty education lines: %#v", doc.Education)
	}
	if len(doc.Skills) < 2 {
		t.Fatalf("skills: %#v", doc.Skills)
	}
	if len(doc.Experience) != 1 || len(doc.Experience[0].Bullets) < 2 {
		t.Fatalf("experience: %#v", doc.Experience)
	}
}

func TestParseImproved_StrictHappyPath(t *testing.T) {
	raw := `{
  "full_name": "Ada",
  "headline": "Engineer",
  "summary": "Builds things.",
  "skills": ["Go"],
  "experience": [{"title": "Dev", "org": "X", "period": "2024", "bullets": ["Shipped"]}],
  "education": ["B.S. CS, State U, 2020"],
  "notes": ["ok"],
  "target_role": "Engineer"
}`
	doc, err := parseImproved(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Education) != 1 || doc.Education[0] != "B.S. CS, State U, 2020" {
		t.Fatalf("%#v", doc.Education)
	}
}
