package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// fakeProvider is a controllable Provider for engine tests: it records Apply
// calls and can return canned jobs or an apply error. No network.
type fakeProvider struct {
	name     string
	jobs     []provider.Job
	applyN   int
	applyErr error
	result   provider.ApplyResult
	applied  []provider.Job
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Search(ctx context.Context, c provider.SearchCriteria) ([]provider.Job, error) {
	return f.jobs, nil
}
func (f *fakeProvider) Apply(ctx context.Context, j provider.Job, p provider.Profile) (provider.ApplyResult, error) {
	f.applyN++
	f.applied = append(f.applied, j)
	if f.result.Status == "" {
		return provider.ApplyResult{Status: "applied"}, f.applyErr
	}
	return f.result, f.applyErr
}

// newTestEngine builds a hermetic Engine: temp-dir SQLite store, empty
// provider list, buffered channels, empty MultiNotifier (Send is a no-op),
// AI off (so background scoring short-circuits with no network).
func newTestEngine(t *testing.T, cfg *config.Config) *Engine {
	t.Helper()
	st, err := store.OpenPath(filepath.Join(t.TempDir(), "engine-test.db"))
	if err != nil {
		t.Fatalf("store.OpenPath: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return &Engine{
		cfg:          cfg,
		store:        st,
		Notifier:     notifier.MultiNotifier{},
		LogCh:        make(chan string, 200),
		ResultCh:     make(chan Result, 500),
		ProgressCh:   make(chan ProviderProgress, 200),
		MaxPerRun:    10,
		MinDelay:     0, // zero delay so tests never sleep
		resumeLoaded: true,
	}
}

func jobFor(name string) provider.Job {
	return provider.Job{
		ID: name + "-1", Title: "Software Engineer", Company: "Acme",
		URL: "https://example.com/" + name + "/1", Provider: name, Board: name,
	}
}

func drainResults(ch chan Result) []Result {
	var out []Result
	for {
		select {
		case r := <-ch:
			out = append(out, r)
		default:
			return out
		}
	}
}

// --- consent & rate-limit guards (AGENTS.md section 14) --------------------

func TestSyncApplySafety_ConsentGate(t *testing.T) {
	tests := []struct {
		name          string
		autoApply     bool
		consent       bool
		wantAutoApply bool
	}{
		{"consent given stays applied", true, true, true},
		{"no consent blocks auto-apply", true, false, false},
		{"manual mode unaffected by consent", false, true, false},
		{"manual mode without consent", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t, &config.Config{ApplyConsent: tt.consent})
			e.AutoApply = tt.autoApply
			e.syncApplySafety()
			if e.AutoApply != tt.wantAutoApply {
				t.Errorf("AutoApply after syncApplySafety = %v; want %v (consent=%v)",
					e.AutoApply, tt.wantAutoApply, tt.consent)
			}
		})
	}
}

func TestSyncApplySafety_NilCfgIsNoop(t *testing.T) {
	e := newTestEngine(t, nil)
	e.AutoApply = true
	e.MaxPerRun = 7
	e.MinDelay = 9
	e.syncApplySafety()
	if !e.AutoApply {
		t.Error("nil cfg must not flip AutoApply off")
	}
	if e.MaxPerRun != 7 || e.MinDelay != 9 {
		t.Errorf("nil cfg must not change limits; got run=%d delay=%d", e.MaxPerRun, e.MinDelay)
	}
}

func TestSyncApplySafety_LimitsFromConfig(t *testing.T) {
	tests := []struct {
		name           string
		cfg            *config.Config
		maxSet, delay  int
		wantMax, wantD int
	}{
		{"uses config values", &config.Config{MaxAppsPerRun: 5, ApplyDelaySec: 7}, 0, 0, 5, 7},
		{"zero config keeps engine defaults", &config.Config{}, 3, 2, 3, 2},
		{"zero config with zero defaults falls back", &config.Config{}, 0, 0, 10, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t, tt.cfg)
			e.MaxPerRun = tt.maxSet
			e.MinDelay = tt.delay
			e.syncApplySafety()
			if e.MaxPerRun != tt.wantMax {
				t.Errorf("MaxPerRun = %d; want %d", e.MaxPerRun, tt.wantMax)
			}
			if e.MinDelay != tt.wantD {
				t.Errorf("MinDelay = %d; want %d", e.MinDelay, tt.wantD)
			}
		})
	}
}

func TestMaxAppsPerDay(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want int
	}{
		{"configured", &config.Config{MaxAppsPerDay: 40}, 40},
		{"zero defaults to 25", &config.Config{}, 25},
		{"nil cfg defaults to 25", nil, 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newTestEngine(t, tt.cfg).maxAppsPerDay(); got != tt.want {
				t.Errorf("maxAppsPerDay() = %d; want %d", got, tt.want)
			}
		})
	}
}

func TestMinFitScore(t *testing.T) {
	if got := newTestEngine(t, &config.Config{MinFitScore: 70}).minFitScore(); got != 70 {
		t.Errorf("minFitScore = %d; want 70", got)
	}
	if got := newTestEngine(t, nil).minFitScore(); got != 0 {
		t.Errorf("minFitScore nil cfg = %d; want 0", got)
	}
}

func TestCompanyBlocked(t *testing.T) {
	tests := []struct {
		company, blocklist string
		want               bool
	}{
		{"Acme", "acme", true},
		{"ACME Corp", "acme", true},
		{"  Foo  ", "foo", true},
		{"Foo Inc", "foo,bar", true},
		{"Innocent Co", "acme,bad", false},
		{"", "acme", false},
		{"Acme", "", false},
		{"Acme", "  ,  ", false},
	}
	for _, tt := range tests {
		if got := companyBlocked(tt.company, tt.blocklist); got != tt.want {
			t.Errorf("companyBlocked(%q, %q) = %v; want %v", tt.company, tt.blocklist, got, tt.want)
		}
	}
}
