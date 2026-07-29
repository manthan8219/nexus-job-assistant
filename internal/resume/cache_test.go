package resume

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadCache(t *testing.T) {
	dir := t.TempDir()
	// Point HOME at temp so we don't touch real ~/.nexus
	t.Setenv("HOME", dir)

	resumeFile := filepath.Join(dir, "r.pdf")
	if err := os.WriteFile(resumeFile, []byte("%PDF-1.4 fake"), 0600); err != nil {
		t.Fatal(err)
	}

	res := Result{
		Valid:    true,
		FileType: "PDF",
		Message:  "PDF · valid",
		Profile:  &Profile{Summary: "Backend engineer"},
	}
	if err := SaveCache(resumeFile, true, res); err != nil {
		t.Fatal(err)
	}
	c, ok := LoadFreshCache(resumeFile, true)
	if !ok || c.Result.Profile.Summary != "Backend engineer" {
		t.Fatalf("load failed: ok=%v c=%+v", ok, c)
	}

	// AI required but cache without profile → miss
	res2 := Result{Valid: true, FileType: "PDF", Message: "ok"}
	_ = SaveCache(resumeFile, false, res2)
	if _, ok := LoadFreshCache(resumeFile, true); ok {
		t.Fatal("expected miss when AI on but no profile")
	}
}
