package resume

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testLetter() CoverLetter {
	return CoverLetter{
		Subject:    "Application for Backend Engineer — Ada Lovelace",
		Greeting:   "Dear Acme Hiring Team,",
		Paragraphs: []string{"First paragraph.", "Second paragraph.", "", "Third paragraph."},
		Closing:    "Sincerely,",
		Signature:  "Ada Lovelace",
	}
}

func TestRenderCoverLetterMarkdown(t *testing.T) {
	md := RenderCoverLetterMarkdown(testLetter())
	for _, want := range []string{"**Application for Backend Engineer", "Dear Acme Hiring Team,", "First paragraph.", "Third paragraph.", "Sincerely,", "Ada Lovelace"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "\n\n\n\n") {
		t.Errorf("blank paragraph leaked into markdown:\n%s", md)
	}
}

func TestRenderCoverLetterMarkdownDefaults(t *testing.T) {
	md := RenderCoverLetterMarkdown(CoverLetter{Paragraphs: []string{"Body."}})
	if !strings.Contains(md, "Dear Hiring Team,") || !strings.Contains(md, "Sincerely,") {
		t.Errorf("defaults missing:\n%s", md)
	}
}

func TestRenderCoverLetterLaTeXEscapes(t *testing.T) {
	cl := testLetter()
	cl.Paragraphs = []string{"Cut cost by 100% & saved $5k — see C# notes_"}
	tex := RenderCoverLetterLaTeX(cl)
	for _, want := range []string{`\documentclass`, `\end{document}`, `100\%`, `\&`, `\$5k`, `notes\_`} {
		if !strings.Contains(tex, want) {
			t.Errorf("latex missing %q:\n%s", want, tex)
		}
	}
}

func TestRenderNativeCoverPDF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cover.pdf")
	if err := RenderNativeCoverPDF(testLetter(), path); err != nil {
		t.Fatalf("RenderNativeCoverPDF: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 5 || string(data[:4]) != "%PDF" {
		t.Fatalf("cover.pdf invalid (err=%v, size=%d)", err, len(data))
	}
}

func TestEnsureCoverPDFFallsBackToNative(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "cover.pdf")
	// Missing .tex file → native fallback must still produce a PDF.
	res, err := EnsureCoverPDF(testLetter(), filepath.Join(dir, "missing.tex"), dest)
	if err != nil {
		t.Fatalf("EnsureCoverPDF: %v", err)
	}
	if res.PDFPath != dest {
		t.Errorf("PDFPath = %q; want %q", res.PDFPath, dest)
	}
	info, err := os.Stat(dest)
	if err != nil || info.Size() == 0 {
		t.Fatalf("dest pdf missing (err=%v)", err)
	}

	if _, err := EnsureCoverPDF(testLetter(), "", ""); err == nil {
		t.Fatal("empty destination: expected error")
	}
}
