package resume

import (
	"strings"
	"testing"
)

func TestSampleResumeIsComplete(t *testing.T) {
	s := SampleResume()
	if s.FullName == "" || s.Headline == "" || s.Summary == "" {
		t.Fatalf("sample resume missing basics: %+v", s)
	}
	if len(s.Skills) == 0 || len(s.Experience) == 0 || len(s.Education) == 0 {
		t.Fatalf("sample resume missing sections: %+v", s)
	}
	for _, role := range s.Experience {
		if len(role.Bullets) == 0 {
			t.Errorf("sample role %q has no bullets", role.Title)
		}
	}
}

func TestRenderTemplatePreviewPDFForAllTemplates(t *testing.T) {
	for _, id := range TemplateIDs() {
		data, err := RenderTemplatePreviewPDF(id)
		if err != nil {
			t.Fatalf("preview %s: %v", id, err)
		}
		if len(data) < 8 {
			t.Errorf("preview %s: too small (%d bytes)", id, len(data))
		}
		if !strings.HasPrefix(string(data), "%PDF") {
			t.Errorf("preview %s: not a PDF", id)
		}
	}
}

func TestRenderTemplatePreviewPDFRejectsUnknown(t *testing.T) {
	if _, err := RenderTemplatePreviewPDF("nope"); err == nil {
		t.Fatal("RenderTemplatePreviewPDF(\"nope\") should error")
	}
}
