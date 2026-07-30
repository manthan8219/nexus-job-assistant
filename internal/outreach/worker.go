package outreach

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// Worker is the always-on outreach pipeline. Whenever a job application is
// recorded (engine apply) or the user builds the email queue, Enqueue pushes
// the job here and a single background goroutine walks it through:
//
//	finding  → resolve HR/careers contact (Hunter/Apollo/GitHub/OSINT/patterns)
//	drafting → generator LLM writes the email, reviewer LLM scores it (loop)
//	ready    → user is notified and asked to approve (confirm/queue modes)
//	sent     → in auto mode ("free will") the email is sent immediately
//
// One goroutine keeps JSON-store writes race-free and local LLM calls serial.
type Worker struct {
	Store *store.Store
	// CfgFn must return a fresh config for every item (config.Load works well —
	// edits in the TUI persist to disk, so the worker always sees latest values).
	CfgFn func() (*config.Config, error)
	// Events receives human-readable progress lines (non-blocking, may drop).
	Events chan string

	q         chan store.Application
	startOnce sync.Once
	closeOnce sync.Once
	done      chan struct{}
	wg        sync.WaitGroup

	resumePath string
	resumeText string
}

// NewWorker builds a pipeline worker. Call Start to begin processing.
func NewWorker(st *store.Store, cfgFn func() (*config.Config, error)) *Worker {
	if cfgFn == nil {
		cfgFn = config.Load
	}
	return &Worker{
		Store:  st,
		CfgFn:  cfgFn,
		Events: make(chan string, 256),
		q:      make(chan store.Application, 512),
		done:   make(chan struct{}),
	}
}

// Start launches the background loop (idempotent).
func (w *Worker) Start(ctx context.Context) {
	w.startOnce.Do(func() {
		go func() {
			for {
				select {
				case <-w.done:
					return
				case app := <-w.q:
					w.process(app)
				case <-ctx.Done():
					return
				}
			}
		}()
	})
}

// Finish stops accepting new work and waits for in-flight items to drain.
// Used by the CLI so emails finish drafting before the process exits.
func (w *Worker) Finish() {
	w.closeOnce.Do(func() { close(w.done) })
	w.wg.Wait()
}

// EnqueueAuto queues an application recorded by the engine — gated on the
// user having opted in to automatic outreach (consent + auto-queue toggle).
func (w *Worker) EnqueueAuto(app store.Application) {
	cfg, err := w.CfgFn()
	if err != nil || cfg == nil {
		return
	}
	if !cfg.OutreachConsent || !cfg.OutreachAutoQueue {
		return
	}
	w.enqueue(app)
}

// EnqueueManual queues a job because the user explicitly asked (Outreach →
// Email → g). Consent is still required before anything is actually sent.
func (w *Worker) EnqueueManual(app store.Application) {
	w.enqueue(app)
}

func (w *Worker) enqueue(app store.Application) {
	if strings.TrimSpace(app.URL) == "" {
		return
	}
	select {
	case <-w.done:
		return
	default:
	}
	w.wg.Add(1)
	select {
	case w.q <- app:
	case <-w.done:
		w.wg.Done()
	}
}

func (w *Worker) eventf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	select {
	case w.Events <- msg:
	default: // UI lagging — drop rather than stall the pipeline
	}
}

// process runs one application through the whole pipeline.
func (w *Worker) process(app store.Application) {
	defer w.wg.Done()
	defer func() { _ = recover() }()

	cfg, err := w.CfgFn()
	if err != nil || cfg == nil {
		w.eventf("outreach: config unavailable — skipping %s", app.Company)
		return
	}
	if !cfg.OutreachConsent {
		w.eventf("outreach: consent off — enable it in Outreach → Setup to reach out for %s", app.Company)
		return
	}

	// Dedupe: one email item per job URL.
	items, err := Load()
	if err != nil {
		w.eventf("outreach: load queue: %v", err)
		return
	}
	if ExistsForJob(items, ChannelEmail, app.URL) {
		return
	}

	job := JobRef{URL: app.URL, Company: app.Company, Role: app.Role, Provider: app.Provider}
	item := NewEmailStub(job, true)
	if err := Upsert(item); err != nil {
		w.eventf("outreach: save stub: %v", err)
		return
	}
	w.eventf("finding HR/careers contact: %s (%s)", app.Company, app.Role)

	// ── Step 1: find the best contact ────────────────────────────────────
	findCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	contact, _, findErr := FindBestContact(findCtx, cfg, w.Store, app.Company, app.URL)
	cancel()
	if findErr == nil && contact.Email != "" {
		item.ContactName = contact.Name
		item.ContactEmail = contact.Email
		item.ContactTitle = contact.Title
		item.ContactSource = "osint"
		w.eventf("contact for %s → %s", app.Company, contact.Email)
	} else {
		item.Error = "no contact found"
		if findErr != nil {
			item.Error = findErr.Error()
		}
		w.eventf("no contact found for %s — draft kept, set To: manually in Outreach → Email", app.Company)
	}

	// ── Step 2: draft (AI writer → AI reviewer loop, or template) ────────
	item.Status = StatusDrafting
	_ = Upsert(item)
	in := w.composeInput(cfg, item, app.Description, contact)

	if cfg.OutreachAICompose && cfg.AIAssist {
		reviewOn := cfg.OutreachAIReview
		genAI := AIOptionsFromConfig(cfg, cfg.OutreachGenModel)
		checkAI := AIOptionsFromConfig(cfg, cfg.OutreachCheckModel)
		minScore := MinScoreOrDefault(cfg)
		maxAttempts := MaxAttemptsOrDefault(cfg)
		w.eventf("drafting email for %s (AI, review=%v)…", app.Company, reviewOn)
		aiCtx, aiCancel := context.WithTimeout(context.Background(), 6*time.Minute)
		draft, review, attempts, genErr := GenerateWithReview(aiCtx, genAI, checkAI, in, maxAttempts, minScore, reviewOn)
		aiCancel()
		if genErr == nil {
			item.Subject = draft.Subject
			item.Body = draft.Body
			item.Attempts = attempts
			if reviewOn {
				item.ReviewScore = review.Score
				item.ReviewNotes = review.Summary
				w.eventf("review %s: %d/100 after %d attempt(s) — %s", app.Company, review.Score, attempts, review.Summary)
			}
		} else {
			w.eventf("AI drafting failed for %s (%v) — falling back to template", app.Company, genErr)
			applyTemplate(cfg, &item, in)
		}
	} else {
		applyTemplate(cfg, &item, in)
	}

	// ── Step 3: ready ────────────────────────────────────────────────────
	if strings.TrimSpace(item.ContactEmail) != "" {
		item.Status = StatusReady
		item.Error = ""
	} else {
		item.Status = StatusDraft
	}
	if err := Upsert(item); err != nil {
		w.eventf("outreach: save ready item: %v", err)
		return
	}
	if item.Status != StatusReady {
		return // user must fix To: manually before anything can be sent
	}

	// ── Step 4: notify, then wait for approval or auto-send ─────────────
	if EffectiveMode(cfg) == ModeAuto {
		// Free will: send immediately (SendEmail enforces consent + daily cap).
		if err := SendEmail(cfg, item); err != nil {
			w.eventf("auto-send failed for %s: %v", app.Company, err)
			w.notify(cfg, "⚠️ Outreach auto-send failed", item, "Open Nexus → Outreach → Email to retry manually.")
			return
		}
		w.eventf("sent → %s (%s @ %s)", item.ContactEmail, item.Role, item.Company)
		w.notify(cfg, "✅ Outreach email sent", item, "Sent automatically (auto mode).")
		return
	}
	w.eventf("ready to send: %s @ %s → %s — approve in Outreach → Email", item.Role, item.Company, item.ContactEmail)
	w.notify(cfg, "📧 Outreach email ready — waiting for you", item,
		"Open Nexus → Outreach → Email and press Enter to send, or set Setup → Automation mode to Auto for free-will sending.")
}

// composeInput assembles everything the generator/reviewer LLMs need.
func (w *Worker) composeInput(cfg *config.Config, it Item, description string, contact Contact) ComposeInput {
	full := strings.TrimSpace(cfg.FirstName + " " + cfg.LastName)
	headline := "software engineer"
	if cfg.YearsOfExperience != "" {
		headline = "engineer with " + cfg.YearsOfExperience + "+ years experience"
	}
	li := ""
	if cfg.LinkedInID != "" {
		li = "https://linkedin.com/in/" + cfg.LinkedInID
	}
	return ComposeInput{
		Company:      it.Company,
		Role:         it.Role,
		Provider:     it.Provider,
		JobURL:       it.JobURL,
		Description:  description,
		ContactName:  contact.Name,
		ContactEmail: contact.Email,
		ContactTitle: contact.Title,
		FullName:     full,
		Headline:     headline,
		Email:        cfg.Email,
		Phone:        cfg.Phone,
		City:         cfg.City,
		LinkedIn:     li,
		ResumeText:   w.loadResume(cfg),
	}
}

// loadResume extracts + caches the resume text for AI personalization.
func (w *Worker) loadResume(cfg *config.Config) string {
	path := strings.TrimSpace(cfg.ResumePath)
	if path == "" {
		return ""
	}
	if path == w.resumePath && w.resumeText != "" {
		return w.resumeText
	}
	text, err := resume.ExtractText(path)
	if err != nil {
		return ""
	}
	w.resumePath = path
	w.resumeText = text
	return text
}

// applyTemplate fills subject/body from the configured (or default) templates.
func applyTemplate(cfg *config.Config, it *Item, in ComposeInput) {
	subjTpl := DefaultEmailSubject()
	bodyTpl := DefaultEmailBody()
	if cfg != nil {
		if strings.TrimSpace(cfg.EmailSubjectTpl) != "" {
			subjTpl = cfg.EmailSubjectTpl
		}
		if strings.TrimSpace(cfg.EmailBodyTpl) != "" {
			bodyTpl = cfg.EmailBodyTpl
		}
	}
	vars := map[string]string{
		"contact_name":  orThere(in.ContactName),
		"contact_email": in.ContactEmail,
		"company":       in.Company,
		"role":          in.Role,
		"provider":      in.Provider,
		"full_name":     in.FullName,
		"headline":      in.Headline,
		"linkedin":      in.LinkedIn,
	}
	it.Subject = RenderTemplate(subjTpl, vars)
	it.Body = RenderTemplate(bodyTpl, vars)
}

func orThere(name string) string {
	if strings.TrimSpace(name) == "" {
		return "there"
	}
	return name
}

// notify fans out an outreach event to the user's notification channels.
func (w *Worker) notify(cfg *config.Config, title string, it Item, message string) {
	discordURL, tgToken, tgChatID, channels := cfg.NotifyFields()
	mn := notifier.FromConfig(&notifier.NotifyConfig{
		DiscordWebhookURL: discordURL,
		TelegramBotToken:  tgToken,
		TelegramChatID:    tgChatID,
		EnabledChannels:   channels,
	})
	if len(mn) == 0 {
		return
	}
	fields := map[string]string{
		"Company": it.Company,
		"Role":    it.Role,
	}
	if it.ContactEmail != "" {
		fields["To"] = it.ContactEmail
	}
	if it.ReviewScore > 0 {
		fields["AI score"] = fmt.Sprintf("%d/100", it.ReviewScore)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	mn.Send(ctx, notifier.Event{
		Kind:      notifier.EventCustom,
		Timestamp: time.Now(),
		Title:     title,
		Message:   message,
		Fields:    fields,
	})
}
