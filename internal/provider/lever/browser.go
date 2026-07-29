package lever

import (
	"fmt"
	"strings"

	"github.com/mxschmitt/playwright-go"
)

// ApplicantInfo holds the standard (non-custom-question) fields Lever's
// apply form always has.
type ApplicantInfo struct {
	FullName   string
	Email      string
	Phone      string
	City       string
	LinkedInID string
	ResumePath string
}

// FillApplyForm fills a Lever apply page's standard fields and every
// answered custom question. It never interacts with the captcha or the
// submit button — solving the captcha and deciding whether to submit is
// left entirely to whoever is looking at the browser.
func FillApplyForm(page playwright.Page, info ApplicantInfo, answers []Answer) error {
	fill := func(selector, value string, label string) error {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		loc := page.Locator(selector)
		if err := loc.Fill(value); err != nil {
			return fmt.Errorf("fill %s: %w", label, err)
		}
		return nil
	}

	if err := fill(`[data-qa="name-input"]`, info.FullName, "name"); err != nil {
		return err
	}
	if err := fill(`[data-qa="email-input"]`, info.Email, "email"); err != nil {
		return err
	}
	if err := fill(`[data-qa="phone-input"]`, info.Phone, "phone"); err != nil {
		return err
	}
	if err := fill(`[data-qa="org-input"]`, info.City, "org/city"); err != nil {
		return err
	}
	if info.LinkedInID != "" {
		if err := fill(`[name="urls[LinkedIn]"]`, "https://linkedin.com/in/"+info.LinkedInID, "LinkedIn"); err != nil {
			return err
		}
	}
	if info.ResumePath != "" {
		if err := page.Locator(`#resume-upload-input`).SetInputFiles(info.ResumePath); err != nil {
			return fmt.Errorf("upload resume: %w", err)
		}
	}

	for _, a := range answers {
		if a.Value == "" {
			continue // left for the human to fill in
		}
		selector := fmt.Sprintf(`[name="%s"]`, a.Question.FieldName)
		switch a.Question.Type {
		case "dropdown":
			label := a.Value
			if _, err := page.Locator(selector).SelectOption(playwright.SelectOptionValues{
				Labels: &[]string{label},
			}); err != nil {
				return fmt.Errorf("select %q for %q: %w", a.Value, a.Question.Text, err)
			}
		case "multiple-select":
			for _, chosen := range strings.Split(a.Value, ";") {
				chosen = strings.TrimSpace(chosen)
				if chosen == "" {
					continue
				}
				cbSelector := fmt.Sprintf(`[name="%s"][value="%s"]`, a.Question.FieldName, chosen)
				if err := page.Locator(cbSelector).Check(); err != nil {
					return fmt.Errorf("check %q for %q: %w", chosen, a.Question.Text, err)
				}
			}
		default: // "text", "textarea"
			if err := page.Locator(selector).Fill(a.Value); err != nil {
				return fmt.Errorf("fill %q: %w", a.Question.Text, err)
			}
		}
	}
	return nil
}
