package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

const maxResumeUploadBytes = 20 << 20 // 20 MB

// handlePostResumeUpload saves an uploaded PDF resume into ~/.nexus/resumes
// and returns the absolute path so the config can reference it directly.
func (s *Server) handlePostResumeUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxResumeUploadBytes)

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "upload: "+err.Error())
		return
	}
	defer file.Close()

	if !strings.EqualFold(filepath.Ext(header.Filename), ".pdf") {
		writeError(w, http.StatusBadRequest, "upload: only PDF files are supported")
		return
	}

	dir, err := config.Dir()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload: "+err.Error())
		return
	}
	resumesDir := filepath.Join(dir, "resumes")

	path, name, err := saveResumeUpload(file, header.Filename, resumesDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path, "name": name})
}

// saveResumeUpload writes an uploaded resume into dir with a sanitized,
// timestamped filename and returns the absolute path. Kept as a pure helper
// so tests can point it at a temp dir instead of ~/.nexus.
func saveResumeUpload(src io.Reader, filename, dir string) (path, name string, err error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", err
	}
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	name = sanitizeResumeName(base) + "-" + time.Now().Format("20060102-150405") + ".pdf"
	path = filepath.Join(dir, name)

	dst, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(path)
		return "", "", err
	}
	if err := dst.Close(); err != nil {
		os.Remove(path)
		return "", "", err
	}
	return path, name, nil
}

// sanitizeResumeName keeps alphanumerics and -/_ in a resume base name.
func sanitizeResumeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "resume"
	}
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}
