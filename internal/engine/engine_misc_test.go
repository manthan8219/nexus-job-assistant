package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func TestResetRecreatesChannels(t *testing.T) {
	e := &Engine{}
	e.Reset()
	if e.LogCh == nil || e.ResultCh == nil || e.ProgressCh == nil {
		t.Fatal("Reset must recreate the engine channels")
	}
}

func TestCfgReturnsConfig(t *testing.T) {
	cfg := &config.Config{ApplyConsent: true}
	e := &Engine{cfg: cfg}
	if e.Cfg() != cfg {
		t.Error("Cfg must return the configured config")
	}
}

func TestProviderNames(t *testing.T) {
	e := &Engine{providers: []provider.Provider{
		&fakeProvider{name: "alpha"},
		&fakeProvider{name: "beta"},
	}}
	got := e.ProviderNames()
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Errorf("ProviderNames = %v, want [alpha beta]", got)
	}
}

func TestCriteriaFromConfig(t *testing.T) {
	cfg := &config.Config{
		TargetJobTitles: "Backend Engineer, Go Developer",
		TargetLocations: "Remote",
		WorkType:        "Remote",
		MinSalary:       "120000",
	}
	c := criteriaFromConfig(cfg)
	if len(c.Titles) != 2 {
		t.Errorf("Titles = %v, want 2 entries", c.Titles)
	}
	if c.WorkType != "Remote" {
		t.Errorf("WorkType = %q, want Remote", c.WorkType)
	}
	if c.MinSalary != 120000 {
		t.Errorf("MinSalary = %d, want 120000", c.MinSalary)
	}
}

func TestEngineNewBuildsProviders(t *testing.T) {
	st, err := store.OpenPath(filepath.Join(t.TempDir(), "new.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	e, err := New(&config.Config{}, st, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	found := false
	for _, name := range e.ProviderNames() {
		if name == "greenhouse" {
			found = true
		}
	}
	if !found {
		t.Errorf("greenhouse not registered; got %v", e.ProviderNames())
	}
}

func TestNotifierFromConfig(t *testing.T) {
	if got := notifierFromCfg(&config.Config{}); len(got) != 0 {
		t.Errorf("empty config must produce no channels; got %d", len(got))
	}
	withDiscord := notifierFromCfg(&config.Config{DiscordWebhookURL: "https://discord.com/api/webhooks/abc/def"})
	if len(withDiscord) == 0 {
		t.Error("discord webhook config must produce a channel")
	}

	// Email must be wired through the engine notifier so a run-complete /
	// daily digest reaches the inbox (not just Discord/Telegram).
	withEmail := notifierFromCfg(&config.Config{
		Email:              "me@gmail.com",
		GmailAppPassword:   "app-pass",
		EmailNotifications: true,
	})
	if !notifierChannelPresent(withEmail, "email") {
		t.Errorf("email config must produce the email channel; got %v", notifierNames(withEmail))
	}
}

func notifierChannelPresent(mn notifier.MultiNotifier, name string) bool {
	for _, n := range mn {
		if n.Name() == name {
			return true
		}
	}
	return false
}

func notifierNames(mn notifier.MultiNotifier) []string {
	var out []string
	for _, n := range mn {
		out = append(out, n.Name())
	}
	return out
}

func TestRebuildNotifier(t *testing.T) {
	e := newTestEngine(t, &config.Config{})
	e.RebuildNotifier(&config.Config{DiscordWebhookURL: "https://discord.com/api/webhooks/abc/def"})
	if len(e.Notifier) == 0 {
		t.Error("RebuildNotifier must pick up the discord webhook")
	}
}

func TestSendProgress(t *testing.T) {
	e := &Engine{ProgressCh: make(chan ProviderProgress, 4)}
	e.sendProgress(ProviderProgress{Provider: "greenhouse", Status: "searching"})
	select {
	case p := <-e.ProgressCh:
		if p.Provider != "greenhouse" || p.Status != "searching" {
			t.Errorf("progress = %+v", p)
		}
	default:
		t.Error("sendProgress must deliver to ProgressCh")
	}
}

func TestLoadResumeTextErrorPaths(t *testing.T) {
	// nil config
	e := &Engine{}
	e.loadResumeText()
	if e.resumeTextErr == nil {
		t.Error("nil config must set resumeTextErr")
	}

	// empty resume path
	e = &Engine{cfg: &config.Config{}}
	e.loadResumeText()
	if e.resumeTextErr == nil {
		t.Error("empty resume path must set resumeTextErr")
	}

	// unsupported file format
	path := filepath.Join(t.TempDir(), "resume.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	e = &Engine{cfg: &config.Config{ResumePath: path}}
	e.loadResumeText()
	if e.resumeTextErr == nil {
		t.Error("unsupported resume format must set resumeTextErr")
	}
	if e.resumeText != "" {
		t.Error("resume text must stay empty on error")
	}
}

// writeResumeDOCX builds a minimal but readable .docx resume (zip with a
// word/document.xml blob) so ExtractText succeeds hermetically.
func writeResumeDOCX(t *testing.T, text string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "resume.docx")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadResumeTextSuccess(t *testing.T) {
	const resumeBody = "Software Engineer with experience building backend systems, education in computer science, and strong project work across distributed services and developer tooling."
	path := writeResumeDOCX(t, resumeBody)
	e := &Engine{cfg: &config.Config{ResumePath: path}}
	e.loadResumeText()
	if e.resumeTextErr != nil {
		t.Fatalf("loadResumeText: %v", e.resumeTextErr)
	}
	if e.resumeText == "" {
		t.Fatal("resume text must be loaded from a readable .docx")
	}
}

func TestTruncateReason(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"short stays", "short reason", 80, "short reason"},
		{"long truncated", "1234567890", 5, "1234…"},
		{"empty stays", "", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateReason(tt.in, tt.n); got != tt.want {
				t.Errorf("truncateReason(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
			}
		})
	}
}

func TestScoreJobAIOff(t *testing.T) {
	e := newTestEngine(t, &config.Config{AIAssist: false})
	score, summary := e.scoreJob(context.Background(), provider.Job{Title: "Engineer", Company: "Acme"})
	if score != 0 || summary != "" {
		t.Errorf("scoreJob with AI off = (%d, %q), want (0, \"\")", score, summary)
	}
}

func TestRegionSummary(t *testing.T) {
	if got := regionSummary(nil); got == "" {
		t.Error("empty region summary must not be empty")
	}
	if got := regionSummary([]string{"IN", "US"}); got != "countries=IN,US" {
		t.Errorf("regionSummary = %q, want countries=IN,US", got)
	}
}

// RunOnce with AutoApply off must queue discovered jobs (hermetic — the fake
// provider makes no network calls).
func TestRunOnceQueuesJobsWithoutAutoApply(t *testing.T) {
	cfg := &config.Config{ApplyConsent: true, AIAssist: false}
	e := newTestEngine(t, cfg)
	e.providers = []provider.Provider{&fakeProvider{
		name: "fake",
		jobs: []provider.Job{
			{ID: "1", Title: "Backend Engineer", Company: "Acme", URL: "https://example.com/1", Provider: "fake", Board: "fake"},
			{ID: "2", Title: "Frontend Engineer", Company: "Globex", URL: "https://example.com/2", Provider: "fake", Board: "fake"},
		},
	}}
	e.OnlyProvider = "fake"

	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	apps, err := e.store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("RunOnce queued %d jobs, want 2", len(apps))
	}
	for _, a := range apps {
		if a.Status != store.StatusQueued {
			t.Errorf("job %s status = %q, want queued", a.URL, a.Status)
		}
	}
}
