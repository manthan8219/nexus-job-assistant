package resume

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/nexusdir"
)

// cacheVersion bumps whenever the on-disk cache format changes (e.g. Profile
// JSON field renames). Old-format caches are rejected so callers re-analyze.
const cacheVersion = 2

// CachedAnalysis is the on-disk snapshot of the last resume analysis.
type CachedAnalysis struct {
	Version     int       `json:"version"`
	ResumePath  string    `json:"resume_path"`
	ModTimeUnix int64     `json:"mod_time_unix"`
	Size        int64     `json:"size"`
	AIEnabled   bool      `json:"ai_enabled"`
	AnalyzedAt  time.Time `json:"analyzed_at"`
	Result      Result    `json:"result"`
}

func cachePath() (string, error) {
	return filepath.Join(nexusdir.Home(), "resume_analysis.json"), nil
}

func fileMeta(path string) (modUnix, size int64, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	return info.ModTime().Unix(), info.Size(), nil
}

// LoadFreshCache returns cached analysis when path + file bytes identity match
// and the cache satisfies the current AI setting.
func LoadFreshCache(path string, aiEnabled bool) (*CachedAnalysis, bool) {
	path = filepath.Clean(path)
	cp, err := cachePath()
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(cp)
	if err != nil {
		return nil, false
	}
	var c CachedAnalysis
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, false
	}
	// Reject caches written by an older schema (field renames) — force re-analysis.
	if c.Version != cacheVersion {
		return nil, false
	}
	if filepath.Clean(c.ResumePath) != path || !c.Result.Valid {
		return nil, false
	}
	mod, size, err := fileMeta(path)
	if err != nil || mod != c.ModTimeUnix || size != c.Size {
		return nil, false
	}
	// AI Assist on → require a real profile in cache from an AI run.
	if aiEnabled {
		if !c.AIEnabled || c.Result.Profile == nil || c.Result.Profile.Summary == "" {
			return nil, false
		}
	}
	return &c, true
}

// SaveCache persists analysis so startup can skip re-running the LLM.
func SaveCache(path string, aiEnabled bool, result Result) error {
	mod, size, err := fileMeta(path)
	if err != nil {
		return err
	}
	hasProfile := result.Profile != nil && result.Profile.Summary != ""
	c := CachedAnalysis{
		Version:     cacheVersion,
		ResumePath:  filepath.Clean(path),
		ModTimeUnix: mod,
		Size:        size,
		AIEnabled:   aiEnabled && hasProfile,
		AnalyzedAt:  time.Now(),
		Result:      result,
	}
	cp, err := cachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cp), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cp, data, 0600)
}
