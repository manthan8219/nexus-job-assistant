package greenhouse

// Package greenhouse — apply.go
// Greenhouse application submission: building the job_application payload and
// POSTing it to the hosted apply-form endpoint, plus the S3 file uploads. The
// answer-guessing heuristics (AutoAnswers and friends) live in apply_answers.go.

// This file submits a Greenhouse application through the same public
// endpoint the hosted apply form uses:
//
//	POST {form.SubmitPath}   Content-Type: application/json
//	{"job_application": {...}, "fingerprint": "<from form loader>"}
//
// The payload shape mirrors what Greenhouse's own React front-end sends
// (reverse-engineered from the published job-board-renderer bundle):
//   - basic fields (first_name, last_name, email, phone, …) as plain strings
//   - resume / cover letter as resume_url + resume_url_filename, after
//     uploading the file to Greenhouse's S3 stash bucket (see upload.go)
//   - custom answers in answers_attributes, keyed by question id:
//     text questions   → {"question_id": N, "priority": P, "text_value": "…"}
//     yes/no (1/0)     → {"question_id": N, "priority": P, "boolean_value": 1}
//     option selects   → {"question_id": N, "priority": P,
//                         "answer_selected_options_attributes":
//                            {"0": {"question_option_id": M}, …}}
//
// Greenhouse enforces an invisible reCAPTCHA Enterprise check on most boards:
// the POST is answered 400 (token missing) or 428 (token invalid). Those
// boards can only be submitted from a real browser (see browser.go); we map
// both statuses to a clear "skipped" result so the engine can fall back.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// Answer pairs one form question with a proposed value. For select-type
// questions Value holds the option *label* ("; "-joined for multi-select) —
// resolved back to option IDs at submission time.
type Answer struct {
	Question ghQuestion
	Value    string
	Err      error
}

// SubmitOptions carries rarely-needed extras for a submission attempt.
type SubmitOptions struct {
	// SecurityCode is the email-verification code Greenhouse sends when its
	// captcha check fails; supplying it retries the submission with
	// "security_code" at the top level, exactly like the hosted form does.
	SecurityCode string
	// CoverLetterText fills a required cover-letter question as plain text
	// (the form's "Enter manually" alternative to a file upload).
	CoverLetterText string
	// CoverLetterPath uploads a cover-letter file instead.
	CoverLetterPath string
}

// captchaReason is the skip reason used when Greenhouse's bot check blocks
// a plain HTTP submission.
const captchaReason = "greenhouse requires a captcha (reCAPTCHA) check for this board — submit via the browser flow (greenhouseapply -browser) or retry with -security-code after email verification"

// basicFieldNames are question field names that map to top-level
// job_application keys rather than answers_attributes.
var basicFieldNames = map[string]bool{
	"first_name": true, "last_name": true, "preferred_name": true,
	"email": true, "phone": true,
	"resume_text": true, "cover_letter_text": true,
	"location": true, "latitude": true, "longitude": true, "country_short_name": true,
}

// SubmitForm submits an application to an already-fetched form. It is the
// exported entry point used by the greenhouseapply command; the engine path
// goes through Client.Apply.
func SubmitForm(
	ctx context.Context,
	client *http.Client,
	form *FormInfo,
	profile provider.Profile,
	answers []Answer,
	opts SubmitOptions,
) (provider.ApplyResult, error) {
	return submitApplication(ctx, client, form, profile, answers, opts)
}

// submitApplication builds the job_application payload and POSTs it.
func submitApplication(
	ctx context.Context,
	client *http.Client,
	form *FormInfo,
	profile provider.Profile,
	answers []Answer,
	opts SubmitOptions,
) (provider.ApplyResult, error) {
	byField := map[string]string{}
	for _, a := range answers {
		if a.Err != nil {
			continue
		}
		for _, f := range a.Question.Fields {
			byField[f.Name] = a.Value
		}
	}

	app, missing, err := buildApplication(form, profile, byField, opts)
	if err != nil {
		return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
	}
	if len(missing) > 0 {
		return provider.ApplyResult{
			Status: "skipped",
			Reason: fmt.Sprintf("required fields missing: %s", strings.Join(missing, ", ")),
		}, nil
	}

	// Validation passed — only now upload attachments (avoids orphan S3 objects).
	if err := uploadFiles(ctx, client, form, opts, profile, app); err != nil {
		return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
	}

	payload := map[string]any{
		"job_application": app,
		"fingerprint":     form.Fingerprint,
	}
	if opts.SecurityCode != "" {
		payload["security_code"] = opts.SecurityCode
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return provider.ApplyResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, form.SubmitPath, bytes.NewReader(body))
	if err != nil {
		return provider.ApplyResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", rendererUA)

	resp, err := client.Do(req)
	if err != nil {
		return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return provider.ApplyResult{Status: "applied"}, nil

	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == 428:
		// 400 = captcha token absent, 428 = captcha verification failed.
		return provider.ApplyResult{Status: "skipped", Reason: captchaReason}, nil

	case resp.StatusCode == http.StatusUnprocessableEntity:
		return provider.ApplyResult{
			Status: "failed",
			Reason: fmt.Sprintf("greenhouse validation: %s", truncate(string(respBody), 400)),
		}, nil

	default:
		return provider.ApplyResult{
			Status: "failed",
			Reason: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 400)),
		}, nil
	}
}

// buildApplication assembles the job_application object (sans file uploads)
// and reports any required questions it could not fill.
func buildApplication(
	form *FormInfo,
	profile provider.Profile,
	byField map[string]string,
	opts SubmitOptions,
) (app map[string]any, missing []string, err error) {
	app = map[string]any{
		"first_name":              profile.FirstName,
		"last_name":               profile.LastName,
		"email":                   profile.Email,
		"answers_attributes":      map[string]any{},
		"demographic_answers":     []any{},
		"data_compliance":         map[string]any{},
		"attachments":             map[string]any{},
		"from_job_board_renderer": true,
		"employments":             []any{},
	}
	if profile.Phone != "" {
		app["phone"] = profile.Phone
	}

	answers := app["answers_attributes"].(map[string]any)
	priority := 0

	for _, q := range form.Questions {
		for _, field := range q.Fields {
			name := field.Name

			switch {
			// ── basic applicant fields → top-level keys ──
			case basicFieldNames[name]:
				switch name {
				case "resume_text", "cover_letter_text":
					// Text alternative to the file upload. Cover-letter text is
					// optional input; resume_text is never sent (file uploaded).
					if name == "cover_letter_text" && opts.CoverLetterText != "" {
						app[name] = opts.CoverLetterText
					}
					continue
				case "location", "latitude", "longitude", "country_short_name":
					// Sent only as a complete geo set from the places widget —
					// a bare location string is ignored by the server, so skip.
					continue
				}
				val := profileValue(name, profile)
				if val != "" {
					app[name] = val
				} else if q.Required {
					missing = append(missing, q.Label)
				}

			// ── resume / cover letter file questions ──
			case name == "resume" || name == "cover_letter":
				// Presence validated here; the actual S3 upload happens in
				// uploadFiles after all required fields check out.
				if name == "resume" && profile.ResumePath == "" && q.Required {
					missing = append(missing, q.Label)
				}
				if name == "cover_letter" && opts.CoverLetterPath == "" && opts.CoverLetterText == "" && q.Required {
					missing = append(missing, q.Label)
				}

			// ── candidate_location widget — skip (needs lat/long pair) ──
			case name == "candidate_location":
				continue

			// ── custom questions → answers_attributes ──
			case strings.HasPrefix(name, "question_"):
				qid, ok := questionID(name)
				if !ok {
					continue
				}
				if field.Type == "input_file" {
					// Custom attachment questions need per-question S3 uploads
					// (attachments{"<qid>_url": …}) — not supported yet.
					if q.Required {
						missing = append(missing, q.Label+" (file attachment)")
					}
					continue
				}
				attr := map[string]any{"question_id": qid, "priority": priority}
				priority++

				raw := strings.TrimSpace(byField[name])
				switch field.Type {
				case "input_text", "textarea":
					if raw != "" {
						attr["text_value"] = raw
					} else if q.Required {
						missing = append(missing, q.Label)
					}
				case "multi_value_single_select", "multi_value_multi_select":
					chosen := resolveOptions(raw, field.Values)
					switch {
					case len(chosen) == 1 && isBoolOption(chosen[0]):
						b, _ := strconv.Atoi(strings.TrimSpace(chosen[0].ValueStr()))
						attr["boolean_value"] = b
					case len(chosen) > 0:
						sel := map[string]any{}
						for i, v := range chosen {
							id, cerr := strconv.ParseInt(strings.TrimSpace(v.ValueStr()), 10, 64)
							if cerr != nil {
								continue
							}
							sel[strconv.Itoa(i)] = map[string]any{"question_option_id": id}
						}
						attr["answer_selected_options_attributes"] = sel
					default:
						// Unanswered select — the front-end sends an empty set.
						attr["answer_selected_options_attributes"] = map[string]any{}
						if q.Required {
							missing = append(missing, q.Label)
						}
					}
				}
				answers[strconv.FormatInt(qid, 10)] = attr
			}
		}
	}
	return app, missing, nil
}

// uploadFiles pushes the resume (and optional cover letter) to Greenhouse's
// S3 stash bucket and wires the resulting URLs into the payload.
func uploadFiles(ctx context.Context, client *http.Client, form *FormInfo, opts SubmitOptions, profile provider.Profile, app map[string]any) error {
	for _, q := range form.Questions {
		for _, field := range q.Fields {
			if field.Type != "input_file" {
				continue
			}
			switch field.Name {
			case "resume":
				if profile.ResumePath == "" {
					continue
				}
				fileURL, fileName, err := uploadAttachment(ctx, client, "resume", profile.ResumePath)
				if err != nil {
					return fmt.Errorf("upload resume: %w", err)
				}
				app["resume_url"] = fileURL
				app["resume_url_filename"] = fileName
			case "cover_letter":
				if opts.CoverLetterPath == "" {
					continue
				}
				fileURL, fileName, err := uploadAttachment(ctx, client, "cover_letter", opts.CoverLetterPath)
				if err != nil {
					return fmt.Errorf("upload cover letter: %w", err)
				}
				app["cover_letter_url"] = fileURL
				app["cover_letter_url_filename"] = fileName
			}
		}
	}
	return nil
}
