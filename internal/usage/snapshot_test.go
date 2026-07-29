package usage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollect(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "applications.db"), make([]byte, 1500), 0600)
	_ = os.MkdirAll(filepath.Join(dir, "resumes"), 0700)
	_ = os.WriteFile(filepath.Join(dir, "resumes", "a.pdf"), make([]byte, 400), 0600)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{}`), 0600)

	s := Collect(dir, 3, "api")
	if s.JobCount != 3 || s.AIMode != "api" {
		t.Fatalf("meta: %+v", s)
	}
	if s.DBBytes < 1500 || s.ResumesBytes < 400 || s.TotalBytes < 1900 {
		t.Fatalf("sizes: db=%d resumes=%d total=%d", s.DBBytes, s.ResumesBytes, s.TotalBytes)
	}
	if Bytes(1500) == "" || FitCostHint("api") == "" {
		t.Fatal("formatters empty")
	}
}
