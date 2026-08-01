// Package nexusdir resolves the Nexus data directory with graceful fallbacks.
//
// Nexus stores its config, SQLite databases, resume cache, and generated
// resumes under a single data directory (locally ~/.nexus). The classic
// lookup, os.UserHomeDir, depends on $HOME — which is NOT defined in some
// environments (Vercel Functions, Docker scratch images, CI runners), so the
// process crashed at startup with "config: $HOME is not defined".
//
// Resolution order for Home():
//  1. NEXUS_HOME env var — explicit override (deployments, containers).
//  2. $HOME/.nexus — the normal local layout.
//  3. <os.TempDir()>/nexus — serverless/CI environments with no $HOME but a
//     writable temp dir.
package nexusdir

import (
	"os"
	"path/filepath"
)

// Home returns the Nexus data directory. It never returns an error: every
// environment has some writable path to fall back to.
func Home() string {
	if v := os.Getenv("NEXUS_HOME"); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".nexus")
	}
	return filepath.Join(os.TempDir(), "nexus")
}

// UserHome returns the user's home directory for "~/" expansion, falling back
// to the temp directory when $HOME is undefined.
func UserHome() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return os.TempDir()
}
