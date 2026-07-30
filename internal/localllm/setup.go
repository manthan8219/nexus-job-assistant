package localllm

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// RuntimeOption is a local inference app the user can install/open.
type RuntimeOption struct {
	ID          string
	Label       string
	DownloadURL string
	Hint        string
}

// SetupOptions are shown when no local runtime is reachable.
func SetupOptions() []RuntimeOption {
	return []RuntimeOption{
		{
			ID:    "start-ollama",
			Label: "Start Ollama",
			Hint:  "if already installed but not running",
		},
		{
			ID:          "install-ollama",
			Label:       "Install Ollama",
			DownloadURL: "https://ollama.com/download",
			Hint:        "recommended · free · CLI + app",
		},
		{
			ID:          "install-lmstudio",
			Label:       "Install LM Studio",
			DownloadURL: "https://lmstudio.ai",
			Hint:        "GUI app · OpenAI-compatible API",
		},
		{
			ID:    "retry",
			Label: "Retry connection",
			Hint:  "ping localhost again",
		},
	}
}

// OllamaInstalled reports whether the ollama binary is on PATH.
func OllamaInstalled() bool {
	_, err := exec.LookPath("ollama")
	return err == nil
}

// StartOllama launches `ollama serve` in the background if the binary exists.
func StartOllama() error {
	path, err := exec.LookPath("ollama")
	if err != nil {
		return fmt.Errorf("ollama not installed — pick Install Ollama")
	}
	cmd := exec.Command(path, "serve")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ollama: %w", err)
	}
	// Give the daemon a moment to bind :11434
	time.Sleep(800 * time.Millisecond)
	return nil
}

// OpenURL opens a URL in the default browser.
func OpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open %s: %w", url, err)
	}
	return nil
}
