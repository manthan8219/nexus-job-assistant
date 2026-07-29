package resume

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNativePDFAndLibrary(t *testing.T) {
	dir := t.TempDir()
	// Point resumes dir via writing directly then Register
	doc := ImprovedDoc{
		FullName:    "Test User",
		Headline:    "Engineer",
		Summary:     "Builds things.",
		Skills:      []string{"Go"},
		Experience:  []ImprovedRole{{Title: "Dev", Org: "Acme", Period: "2024", Bullets: []string{"Shipped API"}}},
		GeneratedAt: time.Now(),
	}
	pdf := filepath.Join(dir, "out.pdf")
	if err := RenderNativePDF(doc, pdf); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(pdf)
	if err != nil || info.Size() < 100 {
		t.Fatalf("pdf missing or tiny: %v %#v", err, info)
	}

	md := filepath.Join(dir, "out.md")
	tex := filepath.Join(dir, "out.tex")
	_ = os.WriteFile(md, []byte(RenderMarkdown(doc)), 0600)
	_ = os.WriteFile(tex, []byte(RenderLaTeX(doc)), 0600)
	dest := filepath.Join(dir, "ensured.pdf")
	conv, err := EnsurePDF(doc, md, tex, dest)
	if err != nil {
		t.Fatal(err)
	}
	if conv.Method != "native" && conv.Method != "latex" && conv.Method != "pandoc" {
		t.Fatalf("unexpected method %q", conv.Method)
	}
	if _, err := os.Stat(conv.PDFPath); err != nil {
		t.Fatal(err)
	}
}

func TestConvertPDFCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.pdf")
	dst := filepath.Join(dir, "b.pdf")
	doc := ImprovedDoc{FullName: "A", Summary: "x"}
	if err := RenderNativePDF(doc, src); err != nil {
		t.Fatal(err)
	}
	conv, err := ConvertFileToPDF(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if conv.Method != "copy" {
		t.Fatalf("got %s", conv.Method)
	}
}
