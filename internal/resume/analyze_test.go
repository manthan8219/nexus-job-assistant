package resume

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func writeTmp(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func makePDF(keywords []string) []byte {
	content := "%PDF-1.4\n"
	for _, kw := range keywords {
		content += kw + "\n"
	}
	content += "%%EOF"
	return []byte(content)
}

func makeDOCX(text string) []byte {
	docXML := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	ct, _ := zw.Create("[Content_Types].xml")
	ct.Write([]byte(`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`))
	doc, _ := zw.Create("word/document.xml")
	doc.Write([]byte(docXML))
	zw.Close()
	return buf.Bytes()
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestAnalyze_FileNotFound(t *testing.T) {
	r := Analyze("/nonexistent/path/resume.pdf")
	if r.Valid {
		t.Fatal("expected invalid for missing file")
	}
	if r.Err == "" {
		t.Fatal("expected non-empty Err")
	}
	t.Logf("error: %s", r.Err)
}

func TestAnalyze_UnsupportedExtension(t *testing.T) {
	path := writeTmp(t, "resume.txt", []byte("some text"))
	r := Analyze(path)
	if r.Valid {
		t.Fatal("expected invalid for .txt")
	}
	t.Logf("error: %s", r.Err)
}

func TestAnalyze_FakePDF_WrongMagicBytes(t *testing.T) {
	path := writeTmp(t, "fake.pdf", []byte("This is just text pretending to be a PDF"))
	r := Analyze(path)
	if r.Valid {
		t.Fatal("expected invalid for fake PDF")
	}
	t.Logf("error: %s", r.Err)
}

func TestAnalyze_ValidPDF_WithResumeKeywords(t *testing.T) {
	data := makePDF([]string{"experience", "education", "skills", "projects", "certifications"})
	path := writeTmp(t, "resume.pdf", data)
	r := Analyze(path)
	if !r.Valid {
		t.Fatalf("expected valid PDF resume, got error: %s", r.Err)
	}
	if r.FileType != "PDF" {
		t.Fatalf("expected FileType=PDF, got %s", r.FileType)
	}
	t.Logf("ok: %s", r.Message)
}

func TestAnalyze_PDF_AcceptedWithoutKeywords(t *testing.T) {
	// PDF text streams are typically compressed — keyword scanning is unreliable.
	// Any valid PDF (magic bytes) should be accepted.
	data := makePDF([]string{"hello", "world"})
	path := writeTmp(t, "recipe.pdf", data)
	r := Analyze(path)
	if !r.Valid {
		t.Fatalf("expected valid PDF to be accepted regardless of keywords, got: %s", r.Err)
	}
	t.Logf("ok: %s", r.Message)
}

func TestAnalyze_ValidDOCX_WithResumeKeywords(t *testing.T) {
	text := "John Doe Experience Education Skills Projects Bachelor University employment"
	data := makeDOCX(text)
	path := writeTmp(t, "resume.docx", data)
	r := Analyze(path)
	if !r.Valid {
		t.Fatalf("expected valid DOCX resume, got error: %s", r.Err)
	}
	if r.FileType != "DOCX" {
		t.Fatalf("expected FileType=DOCX, got %s", r.FileType)
	}
	t.Logf("ok: %s", r.Message)
}

func TestAnalyze_DOCX_TooFewKeywords(t *testing.T) {
	data := makeDOCX("This is a grocery list: milk, eggs, bread")
	path := writeTmp(t, "grocery.docx", data)
	r := Analyze(path)
	if r.Valid {
		t.Fatal("expected invalid — no resume keywords in DOCX")
	}
	t.Logf("error: %s", r.Err)
}

func TestAnalyze_FakeDOCX_NotZip(t *testing.T) {
	path := writeTmp(t, "fake.docx", []byte("not a zip file at all"))
	r := Analyze(path)
	if r.Valid {
		t.Fatal("expected invalid for fake DOCX")
	}
	t.Logf("error: %s", r.Err)
}

func TestAnalyze_DOC_AcceptedOnExistence(t *testing.T) {
	path := writeTmp(t, "resume.doc", []byte("some binary content"))
	r := Analyze(path)
	if !r.Valid {
		t.Fatalf("expected .doc to be accepted, got error: %s", r.Err)
	}
	t.Logf("ok: %s", r.Message)
}

func TestAnalyze_Directory(t *testing.T) {
	r := Analyze(t.TempDir())
	if r.Valid {
		t.Fatal("expected invalid for directory path")
	}
	t.Logf("error: %s", r.Err)
}
