package resume

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
)

// ExtractText pulls plain text from a resume file for AI analysis.
func ExtractText(path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	var (
		text string
		err  error
	)
	switch ext {
	case ".docx":
		text, err = extractDOCX(path)
	case ".pdf":
		text, err = extractPDF(path)
	case ".doc":
		return "", fmt.Errorf("binary .doc is not readable — save as .pdf or .docx for AI analysis")
	default:
		return "", fmt.Errorf("unsupported format %s", ext)
	}
	if err != nil {
		return "", err
	}
	text = collapseSpace(text)
	if err := assertReadableResumeText(text); err != nil {
		return "", err
	}
	if len(text) > 14000 {
		text = text[:14000]
	}
	return text, nil
}

func extractDOCX(path string) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer r.Close()

	for _, zf := range r.File {
		if zf.Name != "word/document.xml" {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return "", err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		dec := xml.NewDecoder(bytes.NewReader(data))
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			if cd, ok := tok.(xml.CharData); ok {
				s := strings.TrimSpace(string(cd))
				if s != "" {
					sb.WriteString(s)
					sb.WriteByte(' ')
				}
			}
		}
		text := collapseSpace(sb.String())
		if text == "" {
			return "", fmt.Errorf("DOCX has no extractable text")
		}
		return text, nil
	}
	return "", fmt.Errorf("DOCX missing word/document.xml")
}

func extractPDF(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open PDF: %w", err)
	}
	defer f.Close()

	rd, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("PDF text extract failed: %w", err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(rd); err != nil {
		return "", err
	}
	text := strings.TrimSpace(buf.String())
	if text == "" {
		return "", fmt.Errorf("PDF has no extractable text — export as text-based PDF or .docx")
	}
	return text, nil
}

// assertReadableResumeText rejects PDF operator junk / empty extractions
// before we waste an LLM call.
func assertReadableResumeText(text string) error {
	if len(text) < 80 {
		return fmt.Errorf("too little text extracted — try a .docx export")
	}
	letters := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	ratio := float64(letters) / float64(len(text))
	if ratio < 0.45 {
		return fmt.Errorf("extracted text looks corrupted (not real resume wording) — try exporting as .docx")
	}
	lower := strings.ToLower(text)
	hits := 0
	for _, kw := range []string{
		"experience", "education", "skills", "engineer", "developer",
		"university", "bachelor", "master", "project", "work", "summary",
		"internship", "software", "backend", "frontend",
	} {
		if strings.Contains(lower, kw) {
			hits++
		}
	}
	if hits < 2 {
		return fmt.Errorf("text does not look like a resume — check the file or use .docx")
	}
	// Reject obvious PDF graphics operator dumps.
	for _, junk := range []string{" endstream", " 0 g ", " Td[", " cm ", "/F1 "} {
		if strings.Contains(text, junk) && hits < 4 {
			return fmt.Errorf("PDF parse produced graphics junk — export as text PDF or .docx")
		}
	}
	return nil
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
