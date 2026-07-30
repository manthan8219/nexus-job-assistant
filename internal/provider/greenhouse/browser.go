package greenhouse

import (
	"fmt"
	"strings"

	"github.com/mxschmitt/playwright-go"
)

// ApplicantInfo holds the standard (non-custom-question) fields Greenhouse's
// apply form always has.
type ApplicantInfo struct {
	FirstName       string
	LastName        string
	Email           string
	Phone           string
	City            string
	ResumePath      string
	CoverLetterPath string
}

// FillApplyForm fills a Greenhouse apply page's standard fields and every
// answered custom question. It never clicks the submit button — reviewing
// and submitting is left to whoever is looking at the browser (or to
// SubmitApplication, when the user explicitly asked for auto-submit).
//
// Works against the hosted embed form (EmbedFormURL), where every field is
// addressable by id/name: plain inputs are #first_name/#question_NNN, file
// inputs are #resume/#cover_letter, dropdowns are react-select widgets whose
// inner input takes a typed label + Enter, and multi-selects are checkboxes
// named question_NNN[].
func FillApplyForm(page playwright.Page, info ApplicantInfo, answers []Answer) error {
	fill := func(selector, value, label string) error {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		if err := page.Locator(selector).First().Fill(value); err != nil {
			return fmt.Errorf("fill %s: %w", label, err)
		}
		return nil
	}

	if err := fill(`#first_name`, info.FirstName, "first name"); err != nil {
		return err
	}
	if err := fill(`#last_name`, info.LastName, "last name"); err != nil {
		return err
	}
	if err := fill(`#email`, info.Email, "email"); err != nil {
		return err
	}
	if err := fill(`#phone`, info.Phone, "phone"); err != nil {
		return err
	}

	// Candidate location is a remote-autocomplete react-select; typing the
	// city and accepting the first suggestion populates location+lat+long.
	// Best-effort: the field is optional on most boards.
	if strings.TrimSpace(info.City) != "" {
		if err := selectReactOption(page, `#candidate-location`, info.City); err != nil {
			_ = err // non-fatal
		}
	}

	if info.ResumePath != "" {
		if err := page.Locator(`#resume`).SetInputFiles(info.ResumePath); err != nil {
			return fmt.Errorf("upload resume: %w", err)
		}
	}
	if info.CoverLetterPath != "" {
		if err := page.Locator(`#cover_letter`).SetInputFiles(info.CoverLetterPath); err != nil {
			return fmt.Errorf("upload cover letter: %w", err)
		}
	}

	for _, a := range answers {
		if a.Err != nil || strings.TrimSpace(a.Value) == "" {
			continue // left for the human to fill in
		}
		if len(a.Question.Fields) == 0 {
			continue
		}
		field := a.Question.Fields[0]
		switch field.Type {
		case "input_text", "textarea":
			if err := page.Locator(`#` + field.Name).Fill(a.Value); err != nil {
				return fmt.Errorf("fill %q: %w", a.Question.Label, err)
			}
		case "multi_value_single_select":
			if err := selectReactOption(page, `#`+field.Name, a.Value); err != nil {
				return fmt.Errorf("select %q for %q: %w", a.Value, a.Question.Label, err)
			}
		case "multi_value_multi_select":
			for _, chosen := range strings.Split(a.Value, ";") {
				chosen = strings.TrimSpace(chosen)
				if chosen == "" {
					continue
				}
				v, ok := matchOption(chosen, field.Values)
				if !ok {
					continue
				}
				sel := fmt.Sprintf(`[name="%s"][value="%s"]`, field.Name, strings.TrimSpace(v.ValueStr()))
				if err := page.Locator(sel).First().Check(); err != nil {
					return fmt.Errorf("check %q for %q: %w", chosen, a.Question.Label, err)
				}
			}
		}
	}
	return nil
}

// selectReactOption picks an option from a react-select widget by typing the
// option label into its input and accepting the highlighted suggestion.
func selectReactOption(page playwright.Page, inputSelector, label string) error {
	input := page.Locator(inputSelector).First()
	if err := input.Click(); err != nil {
		return err
	}
	if err := input.Type(label, playwright.LocatorTypeOptions{Delay: playwright.Float(40)}); err != nil {
		return err
	}
	// Let the filtered option list render, then accept the first match.
	page.WaitForTimeout(600)
	return input.Press("Enter")
}

// SubmitApplication clicks the form's Submit button. Only invoked when the
// user explicitly asked for auto-submit; Greenhouse's invisible reCAPTCHA
// runs automatically as part of the real page's submit handler.
func SubmitApplication(page playwright.Page) error {
	btn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Submit application"})
	if err := btn.First().Click(); err != nil {
		// Fallback for boards with a differently-labelled submit control.
		if err2 := page.Locator(`button[type="submit"]`).First().Click(); err2 != nil {
			return fmt.Errorf("click submit: %w (fallback: %v)", err, err2)
		}
	}
	return nil
}
