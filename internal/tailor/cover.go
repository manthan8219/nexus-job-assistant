package tailor

import (
	"fmt"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/agentx"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

// parseCoverLetter tolerantly parses the cover-writer agent's JSON output,
// requiring at least one real paragraph.
func parseCoverLetter(raw string) (resume.CoverLetter, error) {
	cl, err := agentx.ParseJSON[resume.CoverLetter](raw)
	if err != nil {
		return resume.CoverLetter{}, err
	}
	cl.Subject = strings.TrimSpace(cl.Subject)
	cl.Greeting = strings.TrimSpace(cl.Greeting)
	cl.Closing = strings.TrimSpace(cl.Closing)
	cl.Signature = strings.TrimSpace(cl.Signature)
	paragraphs := make([]string, 0, len(cl.Paragraphs))
	for _, p := range cl.Paragraphs {
		if p = strings.TrimSpace(p); p != "" {
			paragraphs = append(paragraphs, p)
		}
	}
	cl.Paragraphs = paragraphs
	if len(cl.Paragraphs) == 0 {
		return resume.CoverLetter{}, fmt.Errorf("empty cover letter from model")
	}
	return cl, nil
}
