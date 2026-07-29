package usage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Snapshot is a point-in-time view of Nexus local footprint + process memory.
type Snapshot struct {
	DataDir      string
	TotalBytes   int64
	DBBytes      int64
	ResumesBytes int64
	MetaBytes    int64 // config, outreach, work, analysis json, etc.
	OtherBytes   int64
	JobCount     int
	HeapAlloc    uint64
	SysBytes     uint64
	Goroutines   int
	AIMode       string // off | api | local
	CollectedAt  time.Time
	Err          string
}

// Collect walks dataDir and reads process memory stats.
func Collect(dataDir string, jobCount int, aiMode string) Snapshot {
	s := Snapshot{
		DataDir:     dataDir,
		JobCount:    jobCount,
		AIMode:      strings.ToLower(strings.TrimSpace(aiMode)),
		Goroutines:  runtime.NumGoroutine(),
		CollectedAt: time.Now(),
	}
	if s.AIMode == "" {
		s.AIMode = "off"
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	s.HeapAlloc = ms.Alloc
	s.SysBytes = ms.Sys

	if dataDir == "" {
		s.Err = "data dir unknown"
		return s
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		s.Err = err.Error()
		return s
	}
	if !info.IsDir() {
		s.Err = "data path is not a directory"
		return s
	}

	_ = filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		n := fi.Size()
		s.TotalBytes += n
		rel, _ := filepath.Rel(dataDir, path)
		rel = filepath.ToSlash(rel)
		switch {
		case strings.HasSuffix(rel, "applications.db") || strings.Contains(rel, "applications.db"):
			s.DBBytes += n
		case strings.HasPrefix(rel, "resumes/"):
			s.ResumesBytes += n
		case rel == "config.json", rel == "outreach.json", rel == "work_context.json", rel == "resume_analysis.json":
			s.MetaBytes += n
		default:
			s.OtherBytes += n
		}
		return nil
	})
	return s
}

// Bytes formats a byte count for the TUI.
func Bytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.2f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.0f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// FitCostHint explains where scoring work/cost actually lands.
func FitCostHint(aiMode string) string {
	switch strings.ToLower(strings.TrimSpace(aiMode)) {
	case "local":
		return "local LLM — scoring uses your machine CPU/GPU; one job at a time"
	case "api":
		return "cloud API — little local CPU; cost is provider tokens; one job at a time"
	default:
		return "AI Assist off — no fit scoring calls"
	}
}
