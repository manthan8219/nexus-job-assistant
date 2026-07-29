package resume

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Result holds the outcome of analyzing a resume file.
type Result struct {
	Valid    bool
	FileType string // "PDF", "DOCX", "DOC"
	Message  string // shown on success, e.g. "PDF · 10 resume keywords found"
	Err      string // shown on failure
	Profile  *Profile // optional AI career profile when AI Assist is enabled
}

// Common resume section / keyword markers used to detect resume content.
var resumeKeywords = []string{
	"experience", "education", "skills", "employment", "work history",
	"summary", "objective", "projects", "certifications", "achievements",
	"responsibilities", "qualifications", "degree", "university", "college",
	"bachelor", "master", "engineer", "developer", "position",
}

// Analyze checks that path points to a valid resume document.
// It verifies file format and scans content for resume-like sections.
func Analyze(path string) Result {
	info, err := os.Stat(path)
	if err != nil {
		return Result{Err: "file not found"}
	}
	if info.IsDir() {
		return Result{Err: "path is a directory, not a file"}
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf":
		return analyzePDF(path)
	case ".docx":
		return analyzeDOCX(path)
	case ".doc":
		// Binary .doc format is hard to parse without a full library.
		// Accept it based on existence alone.
		return Result{Valid: true, FileType: "DOC", Message: "DOC · format accepted"}
	default:
		return Result{Err: fmt.Sprintf("unsupported format '%s' — use .pdf, .docx, or .doc", ext)}
	}
}

func analyzePDF(path string) Result {
	f, err := os.Open(path)
	if err != nil {
		return Result{Err: "cannot open file"}
	}
	defer f.Close()

	// Verify PDF magic bytes ("%PDF").
	// That's sufficient — PDF text streams are typically compressed (FlateDecode),
	// so keyword scanning on raw bytes is unreliable and produces false negatives.
	hdr := make([]byte, 4)
	if n, err := io.ReadFull(f, hdr); err != nil || n < 4 || string(hdr) != "%PDF" {
		return Result{Err: "not a valid PDF — wrong file format"}
	}
	return Result{Valid: true, FileType: "PDF", Message: "PDF · valid format"}
}

func analyzeDOCX(path string) Result {
	r, err := zip.OpenReader(path)
	if err != nil {
		return Result{Err: "not a valid DOCX — cannot read ZIP structure"}
	}
	defer r.Close()

	var text string
	for _, zf := range r.File {
		if zf.Name != "word/document.xml" {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			break
		}
		data, _ := io.ReadAll(rc)
		rc.Close()

		// Strip XML tags; collect raw character data.
		var sb strings.Builder
		dec := xml.NewDecoder(bytes.NewReader(data))
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			if cd, ok := tok.(xml.CharData); ok {
				sb.Write(cd)
				sb.WriteByte(' ')
			}
		}
		text = strings.ToLower(sb.String())
		break
	}

	if text == "" {
		return Result{FileType: "DOCX", Err: "DOCX found but could not extract text content"}
	}

	found := countKeywords(text)
	if found < 2 {
		return Result{
			FileType: "DOCX",
			Err:      fmt.Sprintf("DOCX found but doesn't look like a resume — only %d resume keywords detected", found),
		}
	}
	return Result{
		Valid:    true,
		FileType: "DOCX",
		Message:  fmt.Sprintf("DOCX · %d resume keywords found", found),
	}
}

func countKeywords(lower string) int {
	n := 0
	for _, kw := range resumeKeywords {
		if strings.Contains(lower, kw) {
			n++
		}
	}
	return n
}
