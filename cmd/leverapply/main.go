// Command leverapply is a review-only diagnostic tool: given a Lever job
// URL, it extracts the apply form's custom questions, asks the configured
// AI to answer them from the user's resume + profile, and prints every
// question + answer for human review. It does NOT submit anything.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/config"
	"github.com/manthanmanthan/nexus/internal/provider/lever"
	"github.com/manthanmanthan/nexus/internal/resume"
	"github.com/mxschmitt/playwright-go"
)

func main() {
	jobURL := flag.String("url", "", "Lever job apply URL, e.g. https://jobs.lever.co/coupa/<id>/apply")
	openBrowser := flag.Bool("browser", false, "open a real browser window, fill the form, and wait — does NOT submit")
	flag.Parse()

	if *jobURL == "" {
		fmt.Fprintln(os.Stderr, "usage: leverapply -url https://jobs.lever.co/<board>/<id>/apply")
		os.Exit(1)
	}

	board, id, err := parseLeverURL(*jobURL)
	if err != nil {
		fail(err)
	}

	cfg, err := config.Load()
	if err != nil {
		fail(fmt.Errorf("load config: %w", err))
	}
	if cfg.ResumePath == "" {
		fail(fmt.Errorf("no resume_path set in config"))
	}
	resumeText, err := resume.ExtractText(cfg.ResumePath)
	if err != nil {
		fail(fmt.Errorf("extract resume text: %w", err))
	}

	fmt.Printf("Fetching apply form for %s/%s …\n", board, id)
	info, err := lever.FetchFormInfo(board, id)
	if err != nil {
		fail(fmt.Errorf("fetch form: %w", err))
	}

	fmt.Printf("\nRequires hCaptcha: %v\n", info.RequiresCaptcha)
	if info.RequiresCaptcha {
		fmt.Println("  → cannot be auto-submitted via plain HTTP POST; a human must solve the captcha.")
	}
	fmt.Printf("Custom questions found: %d\n\n", len(info.Questions))

	if len(info.Questions) == 0 {
		fmt.Println("No custom questions on this posting — standard fields only (name/email/phone/resume/LinkedIn).")
		return
	}

	ai := resume.AIOptions{
		Enabled:      cfg.AIAssist,
		Provider:     cfg.AIProvider,
		LocalURL:     cfg.LocalLLMURL,
		LocalModel:   cfg.LocalLLMModel,
		OpenAIKey:    cfg.OpenAIKey,
		AnthropicKey: cfg.AnthropicKey,
	}
	if !ai.Enabled {
		fail(fmt.Errorf("AI Assist is off in config — enable it to answer custom questions"))
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
		// Company/job title aren't fetched from the Lever postings API here —
		// board slug is a reasonable stand-in for review purposes.
		CompanyName: board,
		JobTitle:    "(see job posting)",
	}

	fmt.Println("Asking AI to answer questions — this can take a bit on a local model…")
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	answers, err := lever.AnswerQuestions(ctx, ai, info.Questions, actx)
	if err != nil {
		fail(fmt.Errorf("answer questions: %w", err))
	}
	fmt.Printf("Done in %s.\n\n", time.Since(start).Round(time.Second))

	fmt.Println("═══ REVIEW — nothing has been submitted ═══")
	missing := 0
	for i, a := range answers {
		req := ""
		if a.Question.Required {
			req = " (required)"
		}
		fmt.Printf("\n%d. %s%s\n", i+1, a.Question.Text, req)
		if len(a.Question.Options) > 0 {
			fmt.Printf("   options: %s\n", strings.Join(a.Question.Options, " | "))
		}
		switch {
		case a.Err != nil:
			missing++
			fmt.Printf("   → ⚠ ERROR: %v\n", a.Err)
		case a.Value == "":
			missing++
			fmt.Printf("   → ⚠ NO ANSWER (needs manual input)\n")
		default:
			fmt.Printf("   → %s\n", a.Value)
		}
	}
	fmt.Println()
	if missing > 0 {
		fmt.Printf("%d of %d questions still need a manual answer before this could be submitted.\n", missing, len(answers))
	} else {
		fmt.Println("All questions have an AI-proposed answer. Review above before deciding whether to submit.")
	}

	if !*openBrowser {
		return
	}
	if err := openAndFillBrowser(*jobURL, cfg, answers); err != nil {
		fail(fmt.Errorf("browser: %w", err))
	}
}

// openAndFillBrowser launches a real, visible Chromium window, fills the
// apply form (standard fields + every answered question), and then waits
// for the user — it never touches the captcha or the submit button.
func openAndFillBrowser(jobURL string, cfg *config.Config, answers []lever.Answer) error {
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("start playwright: %w", err)
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
	if _, err := page.Goto(jobURL); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}

	info := lever.ApplicantInfo{
		FullName:   strings.TrimSpace(cfg.FirstName + " " + cfg.LastName),
		Email:      cfg.Email,
		Phone:      cfg.Phone,
		City:       cfg.City,
		LinkedInID: cfg.LinkedInID,
		ResumePath: cfg.ResumePath,
	}
	if err := lever.FillApplyForm(page, info, answers); err != nil {
		fmt.Fprintf(os.Stderr, "\n⚠ some fields failed to fill: %v\n", err)
		fmt.Fprintln(os.Stderr, "  (the browser stays open — fill anything missing by hand)")
	}

	fmt.Println("\n═══ Browser filled — nothing has been submitted ═══")
	fmt.Println("Review every field, solve the captcha, and click Submit yourself if it looks right.")
	fmt.Println("Press Enter here when you're done (this just closes the browser, it does not submit).")
	bufio.NewReader(os.Stdin).ReadString('\n')
	return nil
}

func parseLeverURL(raw string) (board, id string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("expected URL like https://jobs.lever.co/<board>/<id>[/apply], got %q", raw)
	}
	return parts[0], parts[1], nil
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "leverapply: %v\n", err)
	os.Exit(1)
}
