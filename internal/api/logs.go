package api

import (
	"net/http"
	"time"
)

// LogLine represents a log entry returned by the API.
type LogLine struct {
	Lines []string `json:"lines"`
}

// handleGetLogs returns engine log lines filtered by query.
func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")

	rs := s.runFor(r)
	rs.mu.RLock()
	lines := rs.logLines
	rs.mu.RUnlock()
	// Always emit a JSON array (not `null`) even when the buffer is empty.
	if lines == nil {
		lines = []string{}
	}

	if filter != "" {
		filtered := make([]string, 0)
		for _, l := range lines {
			if contains(l, filter) {
				filtered = append(filtered, l)
			}
		}
		lines = filtered
	}

	writeJSON(w, http.StatusOK, LogLine{Lines: lines})
}

// handleDeleteLogs clears the in-memory log buffer.
func (s *Server) handleDeleteLogs(w http.ResponseWriter, r *http.Request) {
	rs := s.runFor(r)
	rs.mu.Lock()
	rs.logLines = nil
	rs.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// UsageSnapshot mirrors the frontend UsageSnapshot type.
type UsageSnapshot struct {
	DataDir      string `json:"dataDir"`
	TotalBytes   int64  `json:"totalBytes"`
	DBBytes      int64  `json:"dbBytes"`
	ResumesBytes int64  `json:"resumesBytes"`
	MetaBytes    int64  `json:"metaBytes"`
	OtherBytes   int64  `json:"otherBytes"`
	JobCount     int    `json:"jobCount"`
	HeapAlloc    int64  `json:"heapAlloc"`
	SysBytes     int64  `json:"sysBytes"`
	Goroutines   int    `json:"goroutines"`
	AIMode       string `json:"aiMode"`
	CollectedAt  string `json:"collectedAt"`
	Err          string `json:"err"`
}

// handleGetUsage returns disk/memory usage info.
func (s *Server) handleGetUsage(w http.ResponseWriter, r *http.Request) {
	usage := UsageSnapshot{
		CollectedAt: time.Now().UTC().Format(time.RFC3339),
		AIMode:      "off",
	}
	if cfg := s.cfgFor(r); cfg != nil && cfg.AIAssist {
		usage.AIMode = cfg.AIProvider
	}
	writeJSON(w, http.StatusOK, usage)
}

func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
