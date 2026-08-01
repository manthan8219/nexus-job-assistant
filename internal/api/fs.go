package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// handleFSAutocomplete provides file path autocomplete for the resume path field.
func (s *Server) handleFSAutocomplete(w http.ResponseWriter, r *http.Request) {
	prefix := strings.TrimSpace(r.URL.Query().Get("path"))
	if prefix == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	dir := prefix
	if !strings.HasSuffix(dir, string(os.PathSeparator)) {
		dir = filepath.Dir(dir)
	}
	if dir == "." {
		dir = "."
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		// Directory not readable — return empty
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	base := filepath.Base(prefix)
	var suggestions []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(base)) {
			continue
		}
		full := filepath.Join(dir, name)
		if e.IsDir() {
			suggestions = append(suggestions, full+string(os.PathSeparator))
		} else {
			ext := strings.ToLower(filepath.Ext(name))
			if ext == ".pdf" || ext == ".docx" || ext == ".doc" {
				suggestions = append(suggestions, full)
			}
		}
		if len(suggestions) >= 8 {
			break
		}
	}

	writeJSON(w, http.StatusOK, suggestions)
}
