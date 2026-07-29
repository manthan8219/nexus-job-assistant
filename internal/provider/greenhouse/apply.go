package greenhouse

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/manthanmanthan/nexus/internal/provider"
)

// applyURL returns the POST endpoint for a job application.
func applyURL(board string, jobID string) string {
	return fmt.Sprintf("%s/%s/jobs/%s", baseURL, board, jobID)
}

// knownFieldMap maps lowercase substrings of Greenhouse question labels
// to a function that returns the value from Profile.
type fieldResolver func(p provider.Profile) string

var labelResolvers = map[string]fieldResolver{
	"first name":  func(p provider.Profile) string { return p.FirstName },
	"last name":   func(p provider.Profile) string { return p.LastName },
	"email":       func(p provider.Profile) string { return p.Email },
	"phone":       func(p provider.Profile) string { return p.Phone },
	"city":        func(p provider.Profile) string { return p.City },
	"location":    func(p provider.Profile) string { return p.City },
	"linkedin":    func(p provider.Profile) string { return "https://linkedin.com/in/" + p.LinkedInID },
	"website":     func(p provider.Profile) string { return p.Website },
	"portfolio":   func(p provider.Profile) string { return p.Website },
	"salary":      func(p provider.Profile) string { return p.MinSalary },
	"compensation": func(p provider.Profile) string { return p.MinSalary },
	"experience":  func(p provider.Profile) string { return p.YearsExp },
	"years":       func(p provider.Profile) string { return p.YearsExp },
}

// resolveValue tries to find a value for a question label from the profile.
func resolveValue(label string, p provider.Profile) (string, bool) {
	lower := strings.ToLower(label)
	for substring, resolver := range labelResolvers {
		if strings.Contains(lower, substring) {
			val := resolver(p)
			return val, val != ""
		}
	}
	return "", false
}

// submitApplication builds and POSTs a multipart form to Greenhouse.
func submitApplication(
	ctx context.Context,
	client *http.Client,
	job provider.Job,
	questions []ghQuestion,
	profile provider.Profile,
) (provider.ApplyResult, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	missingRequired := []string{}

	for _, q := range questions {
		for _, field := range q.Fields {
			switch field.Type {

			case "input_file":
				// Resume upload
				fmt.Printf("[DEBUG] input_file label=%q resumePath=%q\n", q.Label, profile.ResumePath)
				if strings.Contains(strings.ToLower(q.Label), "resume") ||
					strings.Contains(strings.ToLower(q.Label), "cv") {
					if profile.ResumePath == "" {
						if q.Required {
							missingRequired = append(missingRequired, q.Label)
						}
						continue
					}
					if err := attachFile(w, field.Name, profile.ResumePath); err != nil {
						return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
					}
				}
				// Cover letter — skip, we don't generate one yet

			case "input_text", "input_hidden", "textarea":
				// Skip resume_text — we already uploaded the file for this question.
				if field.Name == "resume_text" || field.Name == "cover_letter_text" {
					continue
				}
				val, ok := resolveValue(q.Label, profile)
				if !ok {
					// Try generic defaults for common custom questions
					val = autoAnswerText(q.Label)
					ok = val != ""
				}
				if !ok {
					if q.Required {
						missingRequired = append(missingRequired, q.Label)
					}
					continue
				}
				if err := w.WriteField(field.Name, val); err != nil {
					return provider.ApplyResult{}, err
				}

			case "multi_value_single_select":
				// Work authorization, yes/no questions — try to auto-answer
				val := autoAnswerSelect(q.Label, field.Values)
				if val != "" {
					if err := w.WriteField(field.Name, val); err != nil {
						return provider.ApplyResult{}, err
					}
				} else if q.Required {
					missingRequired = append(missingRequired, q.Label)
				}

			case "multi_value_multi_select":
				// Skip complex multi-selects for now
				if q.Required {
					missingRequired = append(missingRequired, q.Label)
				}
			}
		}
	}

	if len(missingRequired) > 0 {
		return provider.ApplyResult{
			Status: "skipped",
			Reason: fmt.Sprintf("required fields missing: %s", strings.Join(missingRequired, ", ")),
		}, nil
	}

	if err := w.Close(); err != nil {
		return provider.ApplyResult{}, err
	}

	url := applyURL(job.Board, job.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return provider.ApplyResult{}, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return provider.ApplyResult{Status: "applied"}, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return provider.ApplyResult{
		Status: "failed",
		Reason: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody)),
	}, nil
}

// attachFile writes a file field to the multipart writer.
func attachFile(w *multipart.Writer, fieldName, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open resume %q: %w", path, err)
	}
	defer f.Close()

	part, err := w.CreateFormFile(fieldName, filepath.Base(path))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, f)
	return err
}

// autoAnswerSelect tries to answer simple yes/no / authorization / demographic questions.
func autoAnswerSelect(label string, values []ghValue) string {
	lower := strings.ToLower(label)

	// Helper: find a value whose label matches any of the candidates (case-insensitive).
	findLabel := func(candidates ...string) string {
		for _, v := range values {
			vl := strings.ToLower(v.Label)
			for _, c := range candidates {
				if strings.Contains(vl, c) {
					return v.ValueStr()
				}
			}
		}
		return ""
	}

	// Work authorization → yes
	if strings.Contains(lower, "authorized") || strings.Contains(lower, "authorised") ||
		strings.Contains(lower, "legally") || strings.Contains(lower, "eligible") {
		if v := findLabel("yes"); v != "" {
			return v
		}
	}

	// Sponsorship required → no
	if strings.Contains(lower, "sponsor") {
		if v := findLabel("no"); v != "" {
			return v
		}
	}

	// Previously employed → no
	if strings.Contains(lower, "employed") || strings.Contains(lower, "worked") {
		if v := findLabel("no"); v != "" {
			return v
		}
	}

	// Relatives at company → no
	if strings.Contains(lower, "relative") || strings.Contains(lower, "blood") || strings.Contains(lower, "family") {
		if v := findLabel("no"); v != "" {
			return v
		}
	}

	// How did you hear about → LinkedIn / Job Board
	if strings.Contains(lower, "hear about") || strings.Contains(lower, "how did you") || strings.Contains(lower, "source") {
		if v := findLabel("linkedin"); v != "" {
			return v
		}
		if v := findLabel("job board", "indeed", "glassdoor", "internet", "online"); v != "" {
			return v
		}
		// last resort: first option
		if len(values) > 0 {
			return values[0].ValueStr()
		}
	}

	// Privacy notice / consent → agree / yes
	if strings.Contains(lower, "consent") || strings.Contains(lower, "privacy") || strings.Contains(lower, "gdpr") {
		if v := findLabel("agree", "yes", "accept", "i agree", "acknowledge"); v != "" {
			return v
		}
		if len(values) > 0 {
			return values[0].ValueStr()
		}
	}

	// Gender → prefer not to say
	if strings.Contains(lower, "gender") {
		if v := findLabel("prefer not", "decline", "not to say", "rather not"); v != "" {
			return v
		}
	}

	// Race / ethnicity → prefer not to say
	if strings.Contains(lower, "race") || strings.Contains(lower, "ethnic") {
		if v := findLabel("prefer not", "decline", "not to say", "rather not"); v != "" {
			return v
		}
	}

	// Veteran status → not a veteran
	if strings.Contains(lower, "veteran") || strings.Contains(lower, "military") {
		if v := findLabel("not a veteran", "no", "i am not"); v != "" {
			return v
		}
	}

	// Disability → no disability
	if strings.Contains(lower, "disabilit") {
		if v := findLabel("no", "i don", "do not"); v != "" {
			return v
		}
	}

	return ""
}

// autoAnswerText returns a default answer for common free-text questions.
func autoAnswerText(label string) string {
	lower := strings.ToLower(label)
	// Numeric scale questions (1-5, 1-10)
	if strings.Contains(lower, "scale of 1") || strings.Contains(lower, "rate yourself") ||
		strings.Contains(lower, "out of 5") || strings.Contains(lower, "out of 10") {
		if strings.Contains(lower, "out of 10") || strings.Contains(lower, "scale of 1-10") {
			return "8"
		}
		return "4"
	}
	// Salary / CTC questions
	if strings.Contains(lower, "salary") || strings.Contains(lower, "ctc") ||
		strings.Contains(lower, "compensation") || strings.Contains(lower, "expect") {
		return "open to discussion"
	}
	// Notice period
	if strings.Contains(lower, "notice period") || strings.Contains(lower, "notice") {
		return "30 days"
	}
	// Years of experience
	if strings.Contains(lower, "years of experience") || strings.Contains(lower, "how many years") {
		return "4"
	}
	return ""
}
