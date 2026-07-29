package scraper

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Backend is a supported scraping library the user can install.
type Backend struct {
	ID          string // internal key stored in config
	Name        string // display name
	Notes       string // one-line description shown in the picker
	Requires    string // human hint e.g. "Python 3.10+ · Ollama"
	PipPackages []string
	PostInstall []string // extra commands after pip, e.g. ["playwright install chromium", "crawl4ai-setup"]
}

// Catalog is every scraper backend we support.
var Catalog = []Backend{
	{
		ID:          "playwright",
		Name:        "Playwright",
		Notes:       "No LLM · fastest · works on most sites · recommended",
		Requires:    "Python 3.10+",
		PipPackages: []string{"playwright", "beautifulsoup4"},
		PostInstall: []string{"playwright install chromium"},
	},
	{
		ID:          "scrapegraphai",
		Name:        "ScrapeGraphAI",
		Notes:       "LLM-powered · best accuracy · needs Ollama 8B+",
		Requires:    "Python 3.10+ · Ollama 8B+ model (llama3.1 or better)",
		PipPackages: []string{"scrapegraphai", "playwright"},
		PostInstall: []string{"playwright install chromium"},
	},
	{
		ID:          "crawl4ai",
		Name:        "Crawl4AI",
		Notes:       "Fast async · needs Ollama 8B+",
		Requires:    "Python 3.10+ · Ollama 8B+ model",
		PipPackages: []string{"crawl4ai"},
		PostInstall: []string{"crawl4ai-setup"},
	},
}

// BackendByID returns the backend with the given ID, or nil.
func BackendByID(id string) *Backend {
	for i := range Catalog {
		if Catalog[i].ID == id {
			return &Catalog[i]
		}
	}
	return nil
}

// InstalledBackends returns IDs of backends whose pip package is importable
// inside the scraper venv.
func InstalledBackends() []string {
	py, err := Python3Path()
	if err != nil {
		return nil
	}

	checkMap := map[string]string{
		"scrapegraphai": "scrapegraphai",
		"crawl4ai":      "crawl4ai",
		"playwright":    "playwright",
	}

	var installed []string
	for _, b := range Catalog {
		pkg := checkMap[b.ID]
		cmd := exec.Command(py, "-c", "import "+pkg)
		if cmd.Run() == nil {
			installed = append(installed, b.ID)
		}
	}
	return installed
}

// IsBackendInstalled reports whether a specific backend is available in the venv.
func IsBackendInstalled(id string) bool {
	for _, ins := range InstalledBackends() {
		if ins == id {
			return true
		}
	}
	return false
}

// InstallBackend installs a backend into the scraper venv.
func InstallBackend(ctx context.Context, b Backend, onProgress ProgressFunc) error {
	pip, err := pipPath()
	if err != nil {
		return err
	}
	py, err := Python3Path()
	if err != nil {
		return err
	}

	// pip install packages
	args := append([]string{"install"}, b.PipPackages...)
	progress(onProgress, "pip install "+b.Name+"...")
	if err := run(ctx, onProgress, pip, args...); err != nil {
		return err
	}

	// post-install commands (playwright install chromium, crawl4ai-setup, etc.)
	for _, cmd := range b.PostInstall {
		parts := splitCmd(cmd)
		if len(parts) == 0 {
			continue
		}
		progress(onProgress, "Running: "+cmd)
		// run via venv python -m <module> <args> or direct binary
		fullArgs := append([]string{"-m"}, parts...)
		if err := run(ctx, onProgress, py, fullArgs...); err != nil {
			// non-fatal: log but continue
			progress(onProgress, "warning: "+cmd+" failed: "+err.Error())
		}
	}
	return nil
}

// pipPath returns path to pip inside the venv.
func pipPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	rel := "bin/pip"
	if runtime.GOOS == "windows" {
		rel = "Scripts/pip.exe"
	}
	return filepath.Join(dir, "venv", rel), nil
}

func splitCmd(s string) []string {
	// naive split on space — enough for our fixed post-install commands
	var out []string
	cur := ""
	for _, c := range s {
		if c == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
