package resume

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ConvertResult is the outcome of producing a PDF.
type ConvertResult struct {
	PDFPath string
	Method  string // "latex", "pandoc", "native", "copy"
	Note    string
}

// EnsurePDF writes destPDF from the improved doc and optional intermediate files.
// Prefer LaTeX engine → pandoc(MD) → native Go PDF (always works). Uses the
// Classic template (kept for TUI/tailor callers).
func EnsurePDF(doc ImprovedDoc, mdPath, texPath, destPDF string) (ConvertResult, error) {
	tpl, _ := GetTemplate(TemplateClassic)
	return EnsurePDFFor(doc, tpl, mdPath, texPath, destPDF)
}

// EnsurePDFFor is the template-aware variant of EnsurePDF: the LaTeX and
// native fallback outputs follow the template manifest.
func EnsurePDFFor(doc ImprovedDoc, tpl Template, mdPath, texPath, destPDF string) (ConvertResult, error) {
	if destPDF == "" {
		return ConvertResult{}, fmt.Errorf("empty PDF destination")
	}
	if err := os.MkdirAll(filepath.Dir(destPDF), 0700); err != nil {
		return ConvertResult{}, err
	}

	if texPath != "" {
		if _, err := os.Stat(texPath); err == nil {
			if path, note, err := compileLaTeX(texPath, destPDF); err == nil {
				return ConvertResult{PDFPath: path, Method: "latex", Note: note}, nil
			}
		}
	}

	if mdPath != "" {
		if _, err := os.Stat(mdPath); err == nil {
			if path, note, err := pandocToPDF(mdPath, destPDF); err == nil {
				return ConvertResult{PDFPath: path, Method: "pandoc", Note: note}, nil
			}
		}
	}

	if err := RenderNativePDFFor(doc, tpl, destPDF); err != nil {
		return ConvertResult{}, fmt.Errorf("pdf convert failed: %w", err)
	}
	return ConvertResult{
		PDFPath: destPDF,
		Method:  "native",
		Note:    "PDF built by JobPilot (install tectonic/pdflatex for LaTeX typesetting)",
	}, nil
}

// EnsureCoverPDF writes destPDF for a cover letter, mirroring EnsurePDF:
// prefer the LaTeX engine when a .tex file exists, else the native renderer.
func EnsureCoverPDF(cl CoverLetter, texPath, destPDF string) (ConvertResult, error) {
	if destPDF == "" {
		return ConvertResult{}, fmt.Errorf("empty PDF destination")
	}
	if err := os.MkdirAll(filepath.Dir(destPDF), 0700); err != nil {
		return ConvertResult{}, err
	}

	if texPath != "" {
		if _, err := os.Stat(texPath); err == nil {
			if path, note, err := compileLaTeX(texPath, destPDF); err == nil {
				return ConvertResult{PDFPath: path, Method: "latex", Note: note}, nil
			}
		}
	}

	if err := RenderNativeCoverPDF(cl, destPDF); err != nil {
		return ConvertResult{}, fmt.Errorf("cover letter pdf convert failed: %w", err)
	}
	return ConvertResult{
		PDFPath: destPDF,
		Method:  "native",
		Note:    "PDF built by JobPilot (install tectonic/pdflatex for LaTeX typesetting)",
	}, nil
}

// ConvertFileToPDF converts an on-disk .md / .tex / .pdf into destPDF.
func ConvertFileToPDF(src, destPDF string) (ConvertResult, error) {
	src = filepath.Clean(src)
	ext := strings.ToLower(filepath.Ext(src))
	switch ext {
	case ".pdf":
		if err := copyFile(src, destPDF); err != nil {
			return ConvertResult{}, err
		}
		return ConvertResult{PDFPath: destPDF, Method: "copy"}, nil
	case ".tex":
		path, note, err := compileLaTeX(src, destPDF)
		if err != nil {
			return ConvertResult{}, fmt.Errorf("LaTeX→PDF failed: %w", err)
		}
		return ConvertResult{PDFPath: path, Method: "latex", Note: note}, nil
	case ".md", ".markdown":
		if path, note, err := pandocToPDF(src, destPDF); err == nil {
			return ConvertResult{PDFPath: path, Method: "pandoc", Note: note}, nil
		}
		jsonPath := strings.TrimSuffix(src, ext) + ".json"
		if data, err := os.ReadFile(jsonPath); err == nil {
			var doc ImprovedDoc
			if json.Unmarshal(data, &doc) == nil {
				tex := strings.TrimSuffix(src, ext) + ".tex"
				return EnsurePDF(doc, src, tex, destPDF)
			}
		}
		return ConvertResult{}, fmt.Errorf("Markdown→PDF needs pandoc+PDF engine, or a JobPilot .json sidecar")
	default:
		return ConvertResult{}, fmt.Errorf("unsupported source %s — use .md, .tex, or .pdf", ext)
	}
}

func compileLaTeX(texPath, pdfPath string) (string, string, error) {
	dir := filepath.Dir(texPath)
	base := strings.TrimSuffix(filepath.Base(texPath), filepath.Ext(texPath))

	if bin, err := exec.LookPath("tectonic"); err == nil {
		cmd := exec.Command(bin, "-o", dir, texPath)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", truncate(string(out), 160), err
		}
		built := filepath.Join(dir, base+".pdf")
		if built != pdfPath {
			if err := copyFile(built, pdfPath); err != nil {
				return built, "", nil
			}
		}
		return pdfPath, "", nil
	}
	if bin, err := exec.LookPath("pdflatex"); err == nil {
		cmd := exec.Command(bin, "-interaction=nonstopmode", "-output-directory", dir, texPath)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", truncate(string(out), 120), err
		}
		built := filepath.Join(dir, base+".pdf")
		if built != pdfPath {
			_ = copyFile(built, pdfPath)
		}
		return pdfPath, "", nil
	}
	return "", "", fmt.Errorf("no LaTeX PDF engine (tectonic/pdflatex)")
}

func pandocToPDF(mdPath, pdfPath string) (string, string, error) {
	bin, err := exec.LookPath("pandoc")
	if err != nil {
		return "", "", fmt.Errorf("pandoc not found")
	}
	engines := []string{"pdflatex", "xelatex", "lualatex", "tectonic", "wkhtmltopdf", "weasyprint", "context"}
	var lastErr error
	for _, eng := range engines {
		if _, err := exec.LookPath(eng); err != nil {
			continue
		}
		cmd := exec.Command(bin, mdPath, "-o", pdfPath, "--pdf-engine="+eng)
		if out, err := cmd.CombinedOutput(); err != nil {
			lastErr = fmt.Errorf("%s: %s", eng, truncate(string(out), 100))
			continue
		}
		return pdfPath, "via " + eng, nil
	}
	// Try default pandoc pdf (often pdflatex)
	cmd := exec.Command(bin, mdPath, "-o", pdfPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		if lastErr != nil {
			return "", "", lastErr
		}
		return "", "", fmt.Errorf("pandoc: %s", truncate(string(out), 120))
	}
	return pdfPath, "", nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
