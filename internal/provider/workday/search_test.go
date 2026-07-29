package workday

import (
	"strings"
	"testing"
	"time"
)

func TestParseCareersURL(t *testing.T) {
	cases := []struct {
		raw      string
		tenant   string
		instance string
		site     string
		wantErr  bool
	}{
		{"https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite", "nvidia", "wd5", "NVIDIAExternalCareerSite", false},
		{"https://adobe.wd5.myworkdayjobs.com/external_experienced", "adobe", "wd5", "external_experienced", false},
		{"https://23andme.wd5.myworkdayjobs.com/23", "23andme", "wd5", "23", false},
		{"https://jnj.wd5.myworkdayjobs.com/JJC", "jnj", "wd5", "JJC", false},
		{"https://roche.wd3.myworkdayjobs.com/roche-ext", "roche", "wd3", "roche-ext", false},
		{"https://company.wd1.myworkdayjobs.com/en-US/site", "company", "wd1", "site", false},
		{"https://company.wd1.myworkdayjobs.com/fr/site", "company", "wd1", "site", false},
		{"http://nvidia.wd5.myworkdayjobs.com/site", "", "", "", true},
		{"https://nvidia.example.com/site", "", "", "", true},
		{"not-a-url", "", "", "", true},
	}
	for _, c := range cases {
		tenant, instance, site, err := parseCareersURL(c.raw)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseCareersURL(%q) expected error", c.raw)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseCareersURL(%q) unexpected error: %v", c.raw, err)
			continue
		}
		if tenant != c.tenant {
			t.Errorf("parseCareersURL(%q) tenant = %q, want %q", c.raw, tenant, c.tenant)
		}
		if instance != c.instance {
			t.Errorf("parseCareersURL(%q) instance = %q, want %q", c.raw, instance, c.instance)
		}
		if site != c.site {
			t.Errorf("parseCareersURL(%q) site = %q, want %q", c.raw, site, c.site)
		}
	}
}

func TestParsePostedOn(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		label string
		want  string
	}{
		{"Posted Today", "2026-07-28T12:00:00Z"},
		{"Posted Yesterday", "2026-07-27T12:00:00Z"},
		{"Posted 5 Days Ago", "2026-07-23T12:00:00Z"},
		{"Posted 30+ Days Ago", "0001-01-01T00:00:00Z"},
		{"", "0001-01-01T00:00:00Z"},
		{"garbage", "0001-01-01T00:00:00Z"},
	}
	for _, c := range cases {
		got := parsePostedOn(c.label, now)
		want, _ := time.Parse(time.RFC3339, c.want)
		if !got.Equal(want) {
			t.Errorf("parsePostedOn(%q) = %v, want %v", c.label, got, want)
		}
	}
}

func TestParsePostings(t *testing.T) {
	baseURL := "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite"
	companyName := "NVIDIA"

	postings := []wdayPosting{
		{Title: "Senior GPU Engineer", ExternalPath: "/job/Senior-GPU-Engineer_JR123", LocationsText: "Santa Clara, CA, US", PostedOn: "Posted 3 Days Ago"},
		{Title: "Remote Software Engineer", ExternalPath: "/job/Remote-Software-Engineer_JR456", LocationsText: "Remote", PostedOn: "Posted Today"},
		{Title: "", ExternalPath: "/job/empty_JR789"},
	}

	jobs := parsePostings(postings, baseURL, companyName)
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}

	j0 := jobs[0]
	if j0.Title != "Senior GPU Engineer" {
		t.Errorf("job 0 title = %q", j0.Title)
	}
	if j0.Company != "NVIDIA" {
		t.Errorf("job 0 company = %q", j0.Company)
	}
	if !strings.Contains(j0.Location, "Santa Clara") {
		t.Errorf("job 0 location = %q", j0.Location)
	}
	if j0.Remote {
		t.Error("job 0 should not be remote")
	}
	if !strings.HasPrefix(j0.URL, baseURL) {
		t.Errorf("job 0 url = %q, want %q prefix", j0.URL, baseURL)
	}
	if j0.PostedAt.IsZero() {
		t.Error("job 0 expected non-zero postedAt")
	}

	j1 := jobs[1]
	if !j1.Remote {
		t.Error("job 1 should be remote")
	}
	if j1.Title != "Remote Software Engineer" {
		t.Errorf("job 1 title = %q", j1.Title)
	}
}
