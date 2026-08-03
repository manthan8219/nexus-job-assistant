package outreach

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
	"github.com/manthan8219/nexus-job-assistant/internal/textutil"
)

// ComposeInput carries everything the generator/reviewer LLMs need to write
// and judge one outreach email.
type ComposeInput struct {
	Company     string
	Role        string
	Provider    string
	JobURL      string
	Description string // plain-text job description (may be empty)

	ContactName  string
	ContactEmail string
	ContactTitle string

	// Sender profile
	FullName   string
	Headline   string // e.g. "engineer with 5+ years experience"
	Email      string
	Phone      string
	City       string
	LinkedIn   string
	ResumeText string // trimmed resume text (may be empty)

	// Feedback is the reviewer's critique from the previous attempt — the
	// generator must fix these points on regeneration.
	Feedback string
}

// DraftEmail is what the generator LLM returns.
type DraftEmail struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// ReviewResult is what the reviewer LLM returns.
type ReviewResult struct {
	Pass    bool     `json:"pass"`
	Score   int      `json:"score"` // 0-100
	Issues  []string `json:"issues"`
	Summary string   `json:"summary"`
}

const (
	defaultMinScore    = 70
	defaultMaxAttempts = 3
	maxBodyWords       = 140
)

// MinScoreOrDefault resolves the configured pass threshold.
func MinScoreOrDefault(cfg *config.Config) int {
	if cfg != nil && cfg.OutreachMinScore > 0 && cfg.OutreachMinScore <= 100 {
		return cfg.OutreachMinScore
	}
	return defaultMinScore
}

// MaxAttemptsOrDefault resolves the configured regenerate→review loop cap.
func MaxAttemptsOrDefault(cfg *config.Config) int {
	if cfg != nil && cfg.OutreachMaxRetries > 0 {
		return cfg.OutreachMaxRetries
	}
	return defaultMaxAttempts
}

// AIOptionsFromConfig builds resume.AIOptions for outreach LLM calls.
// modelOverride replaces the local model (generator vs checker models);
// it is ignored for API providers (model is fixed by the provider).
func AIOptionsFromConfig(cfg *config.Config, modelOverride string) resume.AIOptions {
	if cfg == nil {
		return resume.AIOptions{}
	}
	ai := resume.AIOptionsFromConfig(cfg)
	if strings.TrimSpace(modelOverride) != "" {
		ai.LocalModel = strings.TrimSpace(modelOverride)
	}
	return ai
}

// ComposeEmail asks the generator LLM for one personalized cold email.
func ComposeEmail(ctx context.Context, ai resume.AIOptions, in ComposeInput) (DraftEmail, error) {
	raw, err := resume.Complete(ctx, ai, composePrompt(in))
	if err != nil {
		return DraftEmail{}, err
	}
	return parseDraft(raw)
}

// ReviewEmail asks the reviewer LLM to judge a drafted email.
func ReviewEmail(ctx context.Context, ai resume.AIOptions, in ComposeInput, d DraftEmail, minScore int) (ReviewResult, error) {
	raw, err := resume.Complete(ctx, ai, reviewPrompt(in, d, minScore))
	if err != nil {
		return ReviewResult{}, err
	}
	return parseReview(raw)
}

// GenerateWithReview runs the writer→reviewer loop: the generator drafts an
// email, the reviewer scores it; when it fails, the review feedback is fed
// back into the next generation attempt. Returns the best draft seen.
func GenerateWithReview(ctx context.Context, genAI, checkAI resume.AIOptions, in ComposeInput, maxAttempts, minScore int, reviewEnabled bool) (DraftEmail, ReviewResult, int, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var best DraftEmail
	var bestReview ReviewResult
	bestScore := -1
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		draft, err := ComposeEmail(ctx, genAI, in)
		if err != nil {
			lastErr = err
			break // generator broken (e.g. Ollama down) — retrying won't help
		}
		draft = sanitizeDraft(draft)

		if !reviewEnabled {
			return draft, ReviewResult{Pass: true}, attempt, nil
		}

		review, err := ReviewEmail(ctx, checkAI, in, draft, minScore)
		if err != nil {
			// Reviewer unavailable — accept the draft rather than block outreach.
			return draft, ReviewResult{Pass: true, Summary: "reviewer unavailable: " + err.Error()}, attempt, nil
		}
		if review.Score > bestScore {
			bestScore = review.Score
			best = draft
			bestReview = review
		}
		if review.Pass && review.Score >= minScore {
			return draft, review, attempt, nil
		}
		// Feed the critique back into the next attempt.
		in.Feedback = strings.Join(review.Issues, "; ")
		if in.Feedback == "" {
			in.Feedback = fmt.Sprintf("quality score %d/100 is below the %d threshold — make it more specific and human", review.Score, minScore)
		}
	}
	if best.Subject != "" {
		return best, bestReview, maxAttempts, nil
	}
	if lastErr != nil {
		return DraftEmail{}, bestReview, maxAttempts, lastErr
	}
	return DraftEmail{}, bestReview, maxAttempts, fmt.Errorf("no acceptable draft after %d attempts", maxAttempts)
}

// ── prompts ──────────────────────────────────────────────────────────────────

func composePrompt(in ComposeInput) string {
	contact := "the hiring team"
	if strings.TrimSpace(in.ContactName) != "" {
		contact = in.ContactName
		if in.ContactTitle != "" {
			contact += " (" + in.ContactTitle + ")"
		}
	}
	desc := strings.TrimSpace(textutil.HTMLToPlain(in.Description))
	if len(desc) > 6000 {
		desc = desc[:6000]
	}
	resumeText := strings.TrimSpace(in.ResumeText)
	if len(resumeText) > 6000 {
		resumeText = resumeText[:6000]
	}
	if resumeText == "" {
		resumeText = "(no resume text available — do not invent specifics; keep claims general and honest)"
	}

	var extra strings.Builder
	if in.LinkedIn != "" {
		extra.WriteString("\n- LinkedIn: " + in.LinkedIn)
	}
	if in.City != "" {
		extra.WriteString("\n- Based in: " + in.City)
	}

	feedbackSection := ""
	if strings.TrimSpace(in.Feedback) != "" {
		feedbackSection = "\nA previous draft was REJECTED by a quality reviewer. Fix every one of these problems in your new draft:\n\"\"\"\n" + in.Feedback + "\n\"\"\"\n"
	}

	return fmt.Sprintf(`You are an expert job-seeker writing a short cold outreach email after applying for a job.

GOAL: one brief, human, specific email to %s at %s about the "%s" role.
The sender has ALREADY APPLIED — this email is a follow-up to get noticed, not a cover letter.

HARD RULES:
- Body must be 60-%d words (count carefully). Short emails get read.
- Reference ONE concrete detail from the job description and ONE concrete strength from the resume that matches it. No generic "I am passionate".
- Exactly one clear ask (e.g. a short chat, or pointing them to the application). No begging.
- Never invent employers, degrees, years of experience, metrics, or skills that are not in the resume below.
- No buzzwords ("synergy", "rockstar"), no spam-trigger words ("free", "guarantee", "urgent"), no exclamation marks everywhere.
- Plain text only. No markdown, no bullet lists, no placeholders like [Name].
- Greet by first name if known (otherwise "Hi %s,"), sign off with the sender's full name.
- Subject line: 4-9 words, specific to the role, no clickbait.

SENDER:
- Name: %s
- Headline: %s
- Email: %s%s

JOB:
- Role: %s
- Company: %s
- Job description:
"""
%s
"""

SENDER RESUME:
"""
%s
"""
%s
Return ONLY compact JSON (no markdown fences):
{"subject":"...","body":"..."}
`, contact, in.Company, in.Role,
		maxBodyWords,
		greetingName(in),
		in.FullName, in.Headline, in.Email, extra.String(),
		in.Role, in.Company, desc,
		resumeText,
		feedbackSection)
}

func reviewPrompt(in ComposeInput, d DraftEmail, minScore int) string {
	return fmt.Sprintf(`You are a strict reviewer for job-outreach cold emails. A different AI wrote the draft below; you are the quality gate before a real recruiter sees it.

Judge the draft on these criteria:
1. Personalization — mentions the specific role/company and a real detail (not mail-merge generic).
2. Truthfulness — every claim must be supported by the candidate info below; flag ANY invented experience, skill, employer, or metric as a critical issue.
3. Length — body between 60 and %d words.
4. One clear, polite ask. Professional, human tone. No buzzwords or spam-trigger words.
5. Mechanics — correct greeting name, sign-off matches the sender name, no placeholders, no markdown.
6. Subject — specific, 4-9 words, no clickbait.

CONTEXT:
- Role: %s at %s
- Recipient: %s
- Sender: %s (%s)
- Sender resume excerpt (truth source):
"""
%s
"""

DRAFT TO REVIEW:
Subject: %s
Body:
"""
%s
"""

Score 0-100 (90+ = excellent, 70-89 = good enough to send, below 70 = needs rewrite).
Set "pass" true only when the score is >= %d AND there are no critical (truthfulness/mechanics) issues.

Return ONLY compact JSON (no markdown fences):
{"pass":true|false,"score":0-100,"issues":["specific, actionable problem", "..."],"summary":"one sentence verdict"}
`, maxBodyWords, in.Role, in.Company, recipientLabel(in), in.FullName, in.Headline,
		trimForReview(in.ResumeText), d.Subject, d.Body, minScore)
}

// ── parsing / sanitizing ─────────────────────────────────────────────────────

func parseDraft(raw string) (DraftEmail, error) {
	raw = jsonSlice(raw)
	var d DraftEmail
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return DraftEmail{}, fmt.Errorf("invalid draft JSON: %w", err)
	}
	d.Subject = strings.TrimSpace(d.Subject)
	d.Body = strings.TrimSpace(d.Body)
	if d.Subject == "" || d.Body == "" {
		return DraftEmail{}, fmt.Errorf("draft missing subject or body")
	}
	return d, nil
}

func parseReview(raw string) (ReviewResult, error) {
	raw = jsonSlice(raw)
	var r ReviewResult
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return ReviewResult{}, fmt.Errorf("invalid review JSON: %w", err)
	}
	if r.Score < 0 {
		r.Score = 0
	}
	if r.Score > 100 {
		r.Score = 100
	}
	return r, nil
}

func jsonSlice(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			return raw[i : j+1]
		}
	}
	return raw
}

// sanitizeDraft repairs common model mistakes deterministically before review:
// leftover markdown fences and quote-wrapped subjects.
func sanitizeDraft(d DraftEmail) DraftEmail {
	d.Subject = strings.Trim(strings.TrimSpace(d.Subject), "\"")
	body := strings.TrimSpace(d.Body)
	body = strings.TrimPrefix(body, "```plaintext")
	body = strings.TrimPrefix(body, "```text")
	body = strings.TrimPrefix(body, "```")
	body = strings.TrimSuffix(body, "```")
	d.Body = strings.TrimSpace(body)
	return d
}

func greetingName(in ComposeInput) string {
	name := strings.TrimSpace(in.ContactName)
	if name == "" {
		return "there"
	}
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	return "there"
}

func recipientLabel(in ComposeInput) string {
	if strings.TrimSpace(in.ContactName) == "" {
		return "hiring team <" + in.ContactEmail + ">"
	}
	if in.ContactTitle != "" {
		return fmt.Sprintf("%s (%s) <%s>", in.ContactName, in.ContactTitle, in.ContactEmail)
	}
	return fmt.Sprintf("%s <%s>", in.ContactName, in.ContactEmail)
}

func trimForReview(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 3000 {
		return s[:3000]
	}
	return s
}
