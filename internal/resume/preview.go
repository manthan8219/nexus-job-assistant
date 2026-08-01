package resume

import (
	"fmt"
	"os"
)

// SampleResume returns the realistic sample persona used for template previews.
// It deliberately mirrors what the web gallery's miniature renders, so the
// thumbnail card and the real backend-rendered PDF show the same document and
// the user can judge a template on the content, not on abstract bars.
func SampleResume() ImprovedDoc {
	return ImprovedDoc{
		FullName: "Maya Okonkwo",
		Headline: "Senior Product Engineer",
		Summary: "Product engineer with 8+ years building high-scale web platforms. " +
			"Led a five-person team shipping a payments platform used by 2M customers. " +
			"Strong in Go, React, distributed systems, and product thinking.",
		Skills: []string{
			"Go", "TypeScript", "React", "PostgreSQL", "Kubernetes", "gRPC", "CI/CD", "System Design",
		},
		Experience: []ImprovedRole{
			{
				Title: "Senior Product Engineer", Org: "Northwind Labs", Period: "2021 — Present",
				Bullets: []string{
					"Led a 5-person team rebuilding the payments platform, cutting checkout latency 40%.",
					"Designed gRPC APIs and an event pipeline processing 12M events/day.",
					"Introduced CI/CD and a testing culture that halved regression bugs.",
				},
			},
			{
				Title: "Software Engineer", Org: "Acme Cloud", Period: "2018 — 2021",
				Bullets: []string{
					"Built a real-time analytics dashboard used by 500+ customers.",
					"Migrated a legacy monolith to microservices with zero downtime.",
				},
			},
		},
		Education: []string{"B.Sc. Computer Science, University of Lagos"},
	}
}

// RenderTemplatePreviewPDF renders the sample persona into the named template
// and returns the raw PDF bytes. Previews go through the exact same renderer
// that produces real resumes (RenderNativePDFFor), so the gallery's "view the
// actual document" action is always an honest representation of the template.
func RenderTemplatePreviewPDF(templateID string) ([]byte, error) {
	tpl, err := GetTemplate(templateID)
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "nexus-resume-preview-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("preview temp file: %w", err)
	}
	path := tmp.Name()
	// The renderer reopens the path for writing; drop our handle up front.
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("preview temp close: %w", err)
	}
	defer os.Remove(path)

	if err := RenderNativePDFFor(SampleResume(), tpl, path); err != nil {
		return nil, fmt.Errorf("render preview: %w", err)
	}
	return os.ReadFile(path)
}
