// Command greenhouseapply automates Greenhouse job applications.
//
// Given a Greenhouse job URL it fetches the live apply form (questions,
// anti-replay fingerprint, submit endpoint), answers custom questions with
// the configured AI from the user's resume + profile, prints everything for
// review, and then — depending on the flags — does nothing more, submits
// directly over HTTP, or drives a real browser.
//
// Usage:
//
//	greenhouseapply -url <job-url>                      review questions + AI answers only
//	greenhouseapply -url <job-url> -submit              submit over HTTP (works on boards
//	                                                    without captcha; tells you otherwise)
//	greenhouseapply -url <job-url> -browser             open a filled browser, review, submit yourself
//	greenhouseapply -url <job-url> -browser -submit     fully automatic: fill + submit in browser
//	greenhouseapply -url <job-url> -submit -security-code 123456
//	                                                    retry after Greenhouse emails you a
//	                                                    verification code (captcha fallback)
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/greenhouse"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/lever"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
	"github.com/mxschmitt/playwright-go"
)

func main() {
	jobURL := flag.String("url", "", "Greenhouse job URL (boards.greenhouse.io/<board>/jobs/<id>, job-boards.greenhouse.io/…, boards-api.greenhouse.io/…, or an embed job_app link)")
	openBrowser := flag.Bool("browser", false, "open a real browser window and fill the form")
	doSubmit := flag.Bool("submit", false, "actually submit: over HTTP, or click submit in the -browser window")
	securityCode := flag.String("security-code", "", "email verification code Greenhouse sends when its captcha check fails — retries the HTTP submit with it")
	flag.Parse()

	if *jobURL == "" {
		fmt.Fprintln(os.Stderr, "usage: greenhouseapply -url <greenhouse-job-url> [-browser] [-submit] [-security-code N]")
		os.Exit(1)
	}

	board, jobID, err := parseGreenhouseURL(*jobURL)
	if err != nil {
		fail(err)
	}

	cfg, err := config.Load()
	if err != nil {
		fail(fmt.Errorf("load config: %w", err))
	}
	if cfg.Email == "" || cfg.FirstName == "" {
		fail(fmt.Errorf("profile incomplete — fill name/email in the Config tab first"))
	}
	if cfg.ResumePath == "" {
		fail(fmt.Errorf("no resume_path set in config"))
	}

	fmt.Printf("Fetching apply form for %s/%s …\n", board, jobID)
	httpClient := &http.Client{Timeout: 30 * time.Second}
	ctx := context.Background()

	form, err := greenhouse.FetchForm(ctx, httpClient, board, jobID)
	if err != nil {
		fail(fmt.Errorf("fetch form: %w", err))
	}
	fmt.Printf("Job:      %s @ %s (%s)\n", form.Title, form.Company, form.Location)
	custom := 0
	for _, q := range form.Questions {
		if len(q.Fields) > 0 && strings.HasPrefix(q.Fields[0].Name, "question_") {
			custom++
		}
	}
	fmt.Printf("Questions: %d total, %d custom\n\n", len(form.Questions), custom)

	profile := provider.Profile{
		FirstName:  cfg.FirstName,
		LastName:   cfg.LastName,
		Email:      cfg.Email,
		Phone:      cfg.Phone,
		ResumePath: cfg.ResumePath,
		LinkedInID: cfg.LinkedInID,
		City:       cfg.City,
		YearsExp:   cfg.YearsOfExperience,
		MinSalary:  cfg.MinSalary,
	}

	answers := answerQuestions(ctx, cfg, form)
	printReview(answers)

	opts := greenhouse.SubmitOptions{SecurityCode: *securityCode}
	if needsCoverLetter(form) {
		opts.CoverLetterText = generateCoverLetter(ctx, cfg, form)
		if opts.CoverLetterText == "" {
			fmt.Println("⚠ This posting REQUIRES a cover letter and none could be generated —")
			fmt.Println("  the submission will be skipped unless you add one (browser flow lets you type it in).")
		} else {
			fmt.Println("═══ AI cover letter (sent as cover_letter_text) ═══")
			fmt.Println(opts.CoverLetterText)
			fmt.Println()
		}
	}

	switch {
	case *openBrowser:
		if err := runBrowser(form, cfg, answers, *doSubmit); err != nil {
			fail(fmt.Errorf("browser: %w", err))
		}
	case *doSubmit:
		fmt.Println("Submitting over HTTP …")
		res, err := greenhouse.SubmitForm(ctx, httpClient, form, profile, answers, opts)
		if err != nil {
			fail(fmt.Errorf("submit: %w", err))
		}
		fmt.Printf("Result: %s\n", res.Status)
		if res.Reason != "" {
			fmt.Printf("Reason: %s\n", res.Reason)
		}
		if res.Status == "applied" {
			fmt.Println("✓ Application submitted to Greenhouse.")
		}
	default:
		fmt.Println("Review complete — nothing submitted. Re-run with -submit or -browser to apply.")
	}
}

// answerQuestions answers the form's custom questions with the configured AI
// when available, falling back to profile/heuristic auto-answers otherwise.
func answerQuestions(ctx context.Context, cfg *config.Config, form *greenhouse.FormInfo) []greenhouse.Answer {
	ai := resume.AIOptions{
		Enabled:      cfg.AIAssist,
		Provider:     cfg.AIProvider,
		LocalURL:     cfg.LocalLLMURL,
		LocalModel:   cfg.LocalLLMModel,
		OpenAIKey:    cfg.OpenAIKey,
		AnthropicKey: cfg.AnthropicKey,
	}
	if !ai.Enabled {
		fmt.Println("AI Assist is off — using profile/heuristic answers (enable AI Assist for full answers).")
		profile := provider.Profile{
			FirstName: cfg.FirstName, LastName: cfg.LastName, Email: cfg.Email,
			Phone: cfg.Phone, LinkedInID: cfg.LinkedInID, City: cfg.City,
			YearsExp: cfg.YearsOfExperience, MinSalary: cfg.MinSalary,
		}
		return greenhouse.AutoAnswers(form.Questions, profile)
	}

	resumeText, err := resume.ExtractText(cfg.ResumePath)
	if err != nil {
		fail(fmt.Errorf("extract resume text: %w", err))
	}
	actx := lever.AnswerContext{
		ResumeText:   resumeText,
		FirstName:    cfg.FirstName,
		LastName:     cfg.LastName,
		Email:        cfg.Email,
		Phone:        cfg.Phone,
		City:         cfg.City,
		YearsExp:     cfg.YearsOfExperience,
		Currency:     cfg.Currency,
		MinSalary:    cfg.MinSalary,
		WorkAuth:     cfg.WorkAuth,
		WorkType:     cfg.WorkType,
		NoticePeriod: cfg.NoticePeriodDays,
		OfficeDays:   cfg.OfficeDaysPerWeek,
		CompanyName:  form.Company,
		JobTitle:     form.Title,
	}

	fmt.Println("Asking AI to answer custom questions — this can take a bit on a local model…")
	start := time.Now()
	aiCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	answers, err := greenhouse.AnswerQuestions(aiCtx, ai, form.Questions, actx)
	if err != nil {
		fail(fmt.Errorf("answer questions: %w", err))
	}
	fmt.Printf("Done in %s.\n\n", time.Since(start).Round(time.Second))
	return answers
}

// printReview shows every question + proposed answer for human review.
func printReview(answers []greenhouse.Answer) {
	fmt.Println("═══ REVIEW ═══")
	missing := 0
	for i, a := range answers {
		req := ""
		if a.Question.Required {
			req = " (required)"
		}
		fmt.Printf("\n%d. %s%s\n", i+1, a.Question.Label, req)
		if len(a.Question.Fields) > 0 && len(a.Question.Fields[0].Values) > 0 {
			var opts []string
			for _, v := range a.Question.Fields[0].Values {
				opts = append(opts, v.Label)
			}
			fmt.Printf("   options: %s\n", strings.Join(opts, " | "))
		}
		switch {
		case a.Err != nil:
			missing++
			fmt.Printf("   → ⚠ ERROR: %v\n", a.Err)
		case strings.TrimSpace(a.Value) == "":
			if a.Question.Required {
				missing++
			}
			fmt.Printf("   → ⚠ NO ANSWER (needs manual input)\n")
		default:
			fmt.Printf("   → %s\n", a.Value)
		}
	}
	fmt.Println()
	if missing > 0 {
		fmt.Printf("%d question(s) still need a manual answer.\n", missing)
	} else {
		fmt.Println("All questions have a proposed answer.")
	}
	fmt.Println()
}

// needsCoverLetter reports whether the form has a required cover-letter question.
func needsCoverLetter(form *greenhouse.FormInfo) bool {
	for _, q := range form.Questions {
		for _, f := range q.Fields {
			if f.Name == "cover_letter" && q.Required {
				return true
			}
		}
	}
	return false
}

// generateCoverLetter drafts a short plain-text cover letter with the
// configured AI (sent as cover_letter_text, the form's "Enter manually" path).
// Returns "" when AI is unavailable or extraction fails.
func generateCoverLetter(ctx context.Context, cfg *config.Config, form *greenhouse.FormInfo) string {
	ai := resume.AIOptions{
		Enabled:      cfg.AIAssist,
		Provider:     cfg.AIProvider,
		LocalURL:     cfg.LocalLLMURL,
		LocalModel:   cfg.LocalLLMModel,
		OpenAIKey:    cfg.OpenAIKey,
		AnthropicKey: cfg.AnthropicKey,
	}
	if !ai.Enabled {
		return ""
	}
	resumeText, err := resume.ExtractText(cfg.ResumePath)
	if err != nil {
		return ""
	}
	prompt := fmt.Sprintf(`Write a short cover letter (under 150 words, plain text, no placeholder
brackets — use the real names) for %s %s applying to the role %q at %q.
Base it ONLY on the facts in this resume; do not invent employers, numbers,
or achievements. First person, direct, professional.

RESUME TEXT:
"""
%s
"""

Respond with ONLY the cover letter text — no JSON, no markdown, no commentary.`,
		cfg.FirstName, cfg.LastName, form.Title, form.Company, resumeText)

	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	out, err := resume.Complete(cctx, ai, prompt)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// runBrowser opens a visible Chromium window on the real apply form, fills
// every answered field, and either clicks submit (-browser -submit) or waits
// for the user to review + submit themselves.
func runBrowser(form *greenhouse.FormInfo, cfg *config.Config, answers []greenhouse.Answer, autoSubmit bool) error {
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("start playwright: %w (run: go run ./cmd/pwinstall)", err)
	}
	defer pw.Stop()

	headless := false
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: &headless})
	if err != nil {
		return fmt.Errorf("launch browser: %w", err)
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		return fmt.Errorf("new page: %w", err)
	}
	formURL := greenhouse.EmbedFormURL(form.Board, form.JobID)
	if _, err := page.Goto(formURL); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}
	// Let the React form hydrate before filling.
	page.WaitForTimeout(2500)

	info := greenhouse.ApplicantInfo{
		FirstName:  cfg.FirstName,
		LastName:   cfg.LastName,
		Email:      cfg.Email,
		Phone:      cfg.Phone,
		City:       cfg.City,
		ResumePath: cfg.ResumePath,
	}
	if err := greenhouse.FillApplyForm(page, info, answers); err != nil {
		fmt.Fprintf(os.Stderr, "\n⚠ some fields failed to fill: %v\n", err)
		fmt.Fprintln(os.Stderr, "  (the browser stays open — fill anything missing by hand)")
	}

	if autoSubmit {
		fmt.Println("\n═══ Auto-submitting in browser (captcha runs automatically) ═══")
		if err := greenhouse.SubmitApplication(page); err != nil {
			return err
		}
		page.WaitForTimeout(4000)
		fmt.Println("Submitted — check the browser window for Greenhouse's confirmation.")
		fmt.Println("Press Enter here to close the browser.")
		bufio.NewReader(os.Stdin).ReadString('\n')
		return nil
	}

	fmt.Println("\n═══ Browser filled — nothing has been submitted ═══")
	fmt.Println("Review every field, then click Submit yourself if it looks right.")
	fmt.Println("Press Enter here when you're done (this just closes the browser, it does not submit).")
	bufio.NewReader(os.Stdin).ReadString('\n')
	return nil
}

// parseGreenhouseURL extracts board token + job ID from any public Greenhouse
// job URL shape.
func parseGreenhouseURL(raw string) (board, jobID string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", err
	}
	host := strings.ToLower(u.Host)
	if !strings.Contains(host, "greenhouse.io") {
		return "", "", fmt.Errorf("not a Greenhouse URL: %q", raw)
	}

	// Embed form: /embed/job_app?for=<board>&token=<jobID>
	if strings.HasPrefix(u.Path, "/embed/job_app") {
		q := u.Query()
		if q.Get("for") != "" && q.Get("token") != "" {
			return q.Get("for"), q.Get("token"), nil
		}
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, p := range parts {
		if p == "boards" && i+1 < len(parts) { // boards-api: /v1/boards/<board>/jobs/<id>
			board = parts[i+1]
		}
		if p == "jobs" && i+1 < len(parts) {
			jobID = parts[i+1]
		}
	}
	if board == "" && len(parts) >= 3 && parts[1] == "jobs" { // /<board>/jobs/<id>
		board = parts[0]
	}
	if board == "" || jobID == "" {
		return "", "", fmt.Errorf("could not parse board/job ID from %q", raw)
	}
	return board, jobID, nil
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "greenhouseapply: %v\n", err)
	os.Exit(1)
}
