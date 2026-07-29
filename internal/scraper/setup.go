// Package scraper manages the embedded Python career-page scraper microservice.
// It installs, starts, stops, and health-checks the service locally at ~/.nexus/scraper/.
package scraper

import (
	_ "embed"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

//go:embed py/main.py
var mainPy []byte

//go:embed py/requirements.txt
var requirementsTxt []byte

const (
	Port        = "8765"
	BaseURL     = "http://localhost:" + Port
	serviceName = "nexus-scraper"
)

// Dir returns ~/.nexus/scraper
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".nexus", "scraper"), nil
}

// Installed reports whether the scraper venv + files exist.
func Installed() bool {
	dir, err := Dir()
	if err != nil {
		return false
	}
	venv := filepath.Join(dir, "venv", "bin", "python3")
	if runtime.GOOS == "windows" {
		venv = filepath.Join(dir, "venv", "Scripts", "python.exe")
	}
	_, err = os.Stat(venv)
	return err == nil
}

// Running pings the service health endpoint.
func Running() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Python3Path returns the path to python3 inside the venv.
func Python3Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, "venv", "Scripts", "python.exe"), nil
	}
	return filepath.Join(dir, "venv", "bin", "python3"), nil
}

// UvicornPath returns the path to uvicorn inside the venv.
func UvicornPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, "venv", "Scripts", "uvicorn.exe"), nil
	}
	return filepath.Join(dir, "venv", "bin", "uvicorn"), nil
}

// ProgressFunc receives log lines during Install.
type ProgressFunc func(line string)

// Install sets up the Python venv and installs the core service dependencies
// (fastapi, uvicorn, beautifulsoup4). Scraper backends are installed separately
// via InstallBackend. Writes progress lines to onProgress if non-nil.
func Install(ctx context.Context, onProgress ProgressFunc) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	// 1. Write embedded Python files
	if err := os.WriteFile(filepath.Join(dir, "main.py"), mainPy, 0644); err != nil {
		return fmt.Errorf("write main.py: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), requirementsTxt, 0644); err != nil {
		return fmt.Errorf("write requirements.txt: %w", err)
	}
	progress(onProgress, "Files extracted to "+dir)

	// 2. Find system python3
	python, err := exec.LookPath("python3")
	if err != nil {
		python, err = exec.LookPath("python")
		if err != nil {
			return fmt.Errorf("python3 not found — install Python 3.10+ first")
		}
	}

	// 3. Create venv
	venvDir := filepath.Join(dir, "venv")
	if _, err := os.Stat(venvDir); os.IsNotExist(err) {
		progress(onProgress, "Creating Python venv...")
		if err := run(ctx, onProgress, python, "-m", "venv", venvDir); err != nil {
			return fmt.Errorf("create venv: %w", err)
		}
	}

	// 4. pip install
	pip := filepath.Join(venvDir, "bin", "pip")
	if runtime.GOOS == "windows" {
		pip = filepath.Join(venvDir, "Scripts", "pip.exe")
	}
	progress(onProgress, "Installing Python dependencies (this may take a minute)...")
	if err := run(ctx, onProgress, pip, "install", "-r", filepath.Join(dir, "requirements.txt")); err != nil {
		return fmt.Errorf("pip install: %w", err)
	}

	progress(onProgress, "Base install done. Pick a scraper backend to continue.")
	return nil
}

// Start launches the uvicorn server in the background.
// Returns immediately — use Running() to check readiness.
func Start(ollamaModel, ollamaURL string) error {
	if Running() {
		return nil // already up
	}

	uvicorn, err := UvicornPath()
	if err != nil {
		return err
	}
	dir, err := Dir()
	if err != nil {
		return err
	}

	// Pick active backend: first installed one wins
	activeBackend := ""
	for _, ins := range InstalledBackends() {
		activeBackend = ins
		break
	}

	if ollamaModel == "" {
		ollamaModel = "llama3.2"
	}
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	cmd := exec.Command(uvicorn, "main:app", "--host", "127.0.0.1", "--port", Port, "--log-level", "warning")
	cmd.Dir = dir
	env := append(os.Environ(),
		"OLLAMA_MODEL="+ollamaModel,
		"OLLAMA_BASE_URL="+ollamaURL,
	)
	if activeBackend != "" {
		env = append(env, "SCRAPER_BACKEND="+activeBackend)
	}
	cmd.Env = env

	logPath := filepath.Join(dir, "scraper.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start scraper: %w", err)
	}
	return nil
}

// WaitReady waits up to maxWait for the service to become healthy.
func WaitReady(maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if Running() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("scraper not ready after %s", maxWait)
}

// SetupOptions returns UI action options shown when scraper is not installed/running.
type SetupOption struct {
	ID    string
	Label string
	Hint  string
}

func SetupOptions() []SetupOption {
	opts := []SetupOption{
		{
			ID:    "install",
			Label: "Install Career Scraper",
			Hint:  "one-time setup · needs Python 3.10+",
		},
		{
			ID:    "start",
			Label: "Start Career Scraper",
			Hint:  "already installed · just start the service",
		},
		{
			ID:    "retry",
			Label: "Retry connection",
			Hint:  "ping localhost:" + Port + " again",
		},
	}
	// Hide "Start" if not installed yet
	if !Installed() {
		return opts[:1] // only "Install"
	}
	// Add "Scan" only when running and at least one backend installed
	if Running() && len(InstalledBackends()) > 0 {
		opts = append(opts, SetupOption{
			ID:    "scan",
			Label: "Scan companies for jobs",
			Hint:  "discover career pages · scrape jobs · save to store",
		})
	}
	return opts
}

// ── helpers ───────────────────────────────────────────────────────────────────

func progress(fn ProgressFunc, line string) {
	if fn != nil {
		fn(line)
	}
}

// run executes a command, streaming stdout/stderr lines to onProgress.
func run(ctx context.Context, onProgress ProgressFunc, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if onProgress != nil && len(out) > 0 {
		onProgress(string(out))
	}
	return err
}
