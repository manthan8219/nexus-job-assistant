package companies

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFindByCountry(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_ = db.Upsert(Company{
		Name: "Razorpay", Website: "https://razorpay.com",
		ATS: "greenhouse", Board: "razorpay", BoardURL: "https://boards.greenhouse.io/razorpay",
		HireCountries: []string{"India"}, HireCountryCodes: []string{"IN"},
		Kind: "tech", Source: "test",
	})
	_ = db.Upsert(Company{
		Name: "Stripe", Website: "https://stripe.com",
		ATS: "lever", Board: "stripe", BoardURL: "https://jobs.lever.co/stripe",
		HireCountries: []string{"United States", "India"}, HireCountryCodes: []string{"US", "IN"},
		Kind: "tech", Source: "test",
	})
	_ = db.Upsert(Company{
		Name: "Acme US", BoardURL: "https://boards.greenhouse.io/acmeus",
		ATS: "greenhouse", Board: "acmeus",
		HireCountries: []string{"United States"}, HireCountryCodes: []string{"US"},
		Source: "test",
	})

	in, err := db.FindByCountry("IN")
	if err != nil {
		t.Fatal(err)
	}
	if len(in) != 2 {
		t.Fatalf("IN want 2 got %d %#v", len(in), names(in))
	}
	in2, err := db.FindByCountry("India")
	if err != nil || len(in2) != 2 {
		t.Fatalf("India want 2 got %d err=%v", len(in2), err)
	}
	us, err := db.FindByCountry("usa")
	if err != nil || len(us) != 2 {
		t.Fatalf("usa want 2 got %d err=%v", len(us), err)
	}
}

func TestImportOpenJobsJSON(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	raw := `[
	  {"name":"GameEon","website":"https://gameeon.in/","industry_category":"gaming",
	   "ats_links":["https://gameeon.in/careers/"],"countries":["India"]},
	  {"name":"Linear","website":"https://linear.app","industry_category":"tech",
	   "ats_links":["https://jobs.ashbyhq.com/linear"],"countries":["United States"]}
	]`
	n, err := db.ImportOpenJobsJSON(strings.NewReader(raw))
	if err != nil || n != 2 {
		t.Fatalf("import n=%d err=%v", n, err)
	}
	in, _ := db.FindByCountry("india")
	if len(in) != 1 || in[0].Name != "GameEon" {
		t.Fatalf("india: %#v", in)
	}
	ats, _ := db.FindByCountry("US")
	if len(ats) != 1 || ats[0].ATS != "ashby" || ats[0].Board != "linear" {
		t.Fatalf("us ats: %#v", ats)
	}
}

func TestParseATSURL(t *testing.T) {
	ats, board := ParseATSURL("https://boards.greenhouse.io/anthropic")
	if ats != "greenhouse" || board != "anthropic" {
		t.Fatalf("gh %s %s", ats, board)
	}
	ats, board = ParseATSURL("https://jobs.lever.co/netflix")
	if ats != "lever" || board != "netflix" {
		t.Fatalf("lever %s %s", ats, board)
	}
}

func names(cs []Company) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}

func TestBoardsByATS(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_ = db.Upsert(Company{Name: "PhonePe", ATS: "greenhouse", Board: "phonepe", HireCountryCodes: []string{"IN"}, HireCountries: []string{"India"}})
	_ = db.Upsert(Company{Name: "Cred", ATS: "lever", Board: "cred", HireCountryCodes: []string{"IN"}, HireCountries: []string{"India"}})
	_ = db.Upsert(Company{Name: "USOnly", ATS: "greenhouse", Board: "usonly", HireCountryCodes: []string{"US"}, HireCountries: []string{"United States"}})

	// Point OpenDefault-style helper: BoardsByATS uses OpenDefault which is ~/.nexus
	// so test DB method path instead via Find + HasATSBoard; call BoardsByATS only if we can inject.
	in, err := db.BoardsForATS("IN", "greenhouse")
	if err != nil {
		t.Fatal(err)
	}
	if len(in) != 1 || in[0].Name != "PhonePe" {
		t.Fatalf("got %#v", in)
	}
}

func TestBoardTokenWorkday(t *testing.T) {
	c := Company{ATS: "workday", BoardURL: "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite"}
	if BoardToken(c) == "" || !HasATSBoard(c) {
		t.Fatalf("workday URL should count as board token")
	}
}
