package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/api"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/engine"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
	"github.com/manthan8219/nexus-job-assistant/internal/ui"
)

const version = "0.1.0"

const helpText = `
⚡ Nexus — Automated Job Applier

USAGE:
  nexus [flags]

MODES:
  (no flags)        Launch the interactive TUI dashboard
  --run             Run the apply engine once and exit
  --api             Start the HTTP API server (for the web frontend)
  --check-replies   Check Gmail for replies to outreach, update the pipeline
                    (stops follow-ups on reply, records rejections), send any
                    due follow-ups, and notify Discord/Telegram

ENGINE FLAGS:
  --limit N         Max applications per run (default: 10)
  --no-limit        Remove the per-run application cap
  --dry-run         Search and list matching jobs without applying
  --delay N         Min seconds to wait between applications (default: 8)
  --provider NAME   Run only a specific provider (e.g. greenhouse)

CONFIG FLAGS:
  --config PATH          Path to config file (default: ~/.nexus/config.json)
  --companies PATH       Path to companies JSON file (default: data/companies.json)
  --skip-resume-check    Skip resume file validation (accept any path)

API FLAGS:
  --api-port PORT        Port for the API server (default: 8080)

OUTPUT FLAGS:
  --verbose         Print detailed logs including skipped jobs
  --version         Print version and exit

EXAMPLES:
  nexus                          Open the TUI
  nexus --run                    Apply to jobs (max 10)
  nexus --run --limit 5          Apply to max 5 jobs
  nexus --run --no-limit         Apply to all found jobs
  nexus --run --dry-run          See matching jobs without applying
  nexus --run --delay 15         Wait at least 15s between applications
  nexus --run --provider greenhouse --limit 3
  nexus --api                    Start the API server on port 8080
  nexus --api --api-port 3000    Start the API server on port 3000
  nexus --run --verbose

DATA:
  Config:       ~/.nexus/config.json
  Applications: ~/.nexus/applications.db
  Sessions:     ~/.nexus/sessions/

  Edit companies list: data/companies.json
`

func main() {
	// Flags
	runMode := flag.Bool("run", false, "run the apply engine once and exit")
	dryRun := flag.Bool("dry-run", false, "search jobs without applying")
	noLimit := flag.Bool("no-limit", false, "remove per-run application cap")
	verbose := flag.Bool("verbose", false, "print detailed logs")
	showVersion := flag.Bool("version", false, "print version and exit")
	skipResume := flag.Bool("skip-resume-check", false, "skip resume file validation in the TUI")
	testNotify := flag.Bool("test-notify", false, "send a test notification to all configured channels and exit")
	checkReplies := flag.Bool("check-replies", false, "check Gmail for outreach replies, update pipeline, send due follow-ups")
	limit := flag.Int("limit", 10, "max applications per run")
	delay := flag.Int("delay", 8, "min seconds between applications")
	providerName := flag.String("provider", "", "run only this provider (e.g. greenhouse)")
	configPath := flag.String("config", "", "path to config file")
	companiesPath := flag.String("companies", "", "path to companies JSON file")
	apiMode := flag.Bool("api", false, "start the HTTP API server and exit")
	apiPort := flag.Int("api-port", 8080, "port for the API server (default: 8080)")

	flag.Usage = func() { fmt.Print(helpText) }
	flag.Parse()

	if *showVersion {
		fmt.Printf("nexus v%s\n", version)
		return
	}

	if *testNotify {
		cfg, err := loadConfig(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			os.Exit(1)
		}
		runTestNotify(cfg)
		return
	}

	if *checkReplies {
		cfg, err := loadConfig(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			os.Exit(1)
		}
		runReplyCheck(cfg)
		return
	}

	// Load config
	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	if *apiMode {
		// API server mode — create store + engine, then start the HTTP API
		st, err := store.Open()
		if err != nil {
			fmt.Fprintf(os.Stderr, "store: %v\n", err)
			os.Exit(1)
		}
		defer st.Close()

		eng, err := engine.New(cfg, st, *companiesPath)
		if err != nil {
			// Non-fatal: API works without engine (shows error on dashboard)
			fmt.Fprintf(os.Stderr, "warning: engine init failed: %v\n", err)
			eng = nil
		}

		// Platforms like Vercel inject a PORT env var that the server must
		// bind to (the function won't receive traffic otherwise). --api-port
		// still wins for local runs when PORT is absent.
		port := *apiPort
		if p := os.Getenv("PORT"); p != "" {
			if n, err := strconv.Atoi(p); err == nil && n > 0 {
				port = n
			}
		}
		addr := fmt.Sprintf(":%d", port)
		srv := api.New(cfg, st, eng, addr)
		if err := srv.ListenAndServe(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "api server: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if !*runMode && !*dryRun {
		// TUI mode — create store + engine so UI can control the engine directly
		st, err := store.Open()
		if err != nil {
			fmt.Fprintf(os.Stderr, "store: %v\n", err)
			os.Exit(1)
		}
		defer st.Close()

		eng, err := engine.New(cfg, st, *companiesPath)
		if err != nil {
			// Non-fatal: TUI works without engine (shows error on dashboard)
			fmt.Fprintf(os.Stderr, "warning: engine init failed: %v\n", err)
			eng = nil
		}

		app := ui.NewAppModel(cfg, st, eng, ui.AppOptions{SkipResumeCheck: *skipResume})
		p := tea.NewProgram(app, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Engine mode
	opts := engineOpts{
		dryRun:       *dryRun,
		noLimit:      *noLimit,
		limit:        *limit,
		minDelay:     *delay,
		verbose:      *verbose,
		providerName: *providerName,
		companies:    *companiesPath, // empty = use embedded default
	}
	runEngine(cfg, opts)
}

type engineOpts struct {
	dryRun       bool
	noLimit      bool
	limit        int
	minDelay     int
	verbose      bool
	providerName string
	companies    string
}

func runEngine(cfg *config.Config, opts engineOpts) {
	// Validate required config fields
	missing := []string{}
	if cfg.FirstName == "" {
		missing = append(missing, "first_name")
	}
	if cfg.LastName == "" {
		missing = append(missing, "last_name")
	}
	if cfg.Email == "" {
		missing = append(missing, "email")
	}
	if cfg.TargetJobTitles == "" {
		missing = append(missing, "target_job_titles")
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "error: missing required config fields: %v\n", missing)
		fmt.Fprintf(os.Stderr, "run 'nexus' to open the TUI and fill in your profile\n")
		os.Exit(1)
	}

	st, err := store.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	eng, err := engine.New(cfg, st, opts.companies)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engine init: %v\n", err)
		os.Exit(1)
	}

	// Apply options
	if opts.noLimit {
		eng.MaxPerRun = 999999
	} else {
		eng.MaxPerRun = opts.limit
	}
	eng.MinDelay = opts.minDelay
	eng.DryRun = opts.dryRun
	eng.Verbose = opts.verbose
	eng.OnlyProvider = opts.providerName

	if opts.dryRun {
		fmt.Println("⚡ Nexus — DRY RUN (no applications will be submitted)")
		fmt.Println()
	} else {
		fmt.Printf("⚡ Nexus — Running (limit: %d, delay: %ds+)\n\n", eng.MaxPerRun, opts.minDelay)
	}

	// Drain result channel in background
	go func() {
		for range eng.ResultCh {
		}
	}()

	if err := eng.RunOnce(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "engine error: %v\n", err)
		os.Exit(1)
	}
}

func loadConfig(path string) (*config.Config, error) {
	if path != "" {
		return config.LoadFrom(path)
	}
	return config.Load()
}

func runTestNotify(cfg *config.Config) {
	discordURL, tgToken, tgChatID, channels := cfg.NotifyFields()
	mn := notifier.FromConfig(&notifier.NotifyConfig{
		DiscordWebhookURL:  discordURL,
		TelegramBotToken:   tgToken,
		TelegramChatID:     tgChatID,
		EnabledChannels:    channels,
		Email:              cfg.Email,
		GmailAppPassword:   cfg.GmailAppPassword,
		EmailNotifications: cfg.EmailNotifications,
		EmailPerJob:        cfg.EmailPerJob,
	})
	if len(mn) == 0 {
		fmt.Fprintln(os.Stderr, "error: no notification channels configured")
		os.Exit(1)
	}
	for _, n := range mn {
		fmt.Printf("→ %s configured\n", n.Name())
	}
	fmt.Println("Sending test notification...")
	ev := notifier.Event{
		Kind:    notifier.EventCustom,
		Title:   "⚡ Nexus — Test Notification",
		Message: "Your notification integration is working correctly.",
	}
	errs := mn.Send(context.Background(), ev)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "error: %v\n", e)
		}
		os.Exit(1)
	}
	fmt.Println("✓ Test notification sent successfully")
}

// runReplyCheck is the one-shot response-loop pass: scan the inbox for
// replies to outreach, stop answered sequences, record outcomes, alert on
// human replies, then fire any follow-ups whose scheduled time has arrived.
func runReplyCheck(cfg *config.Config) {
	fetcher := outreach.NewGmailIMAPFetcher(cfg)
	if fetcher == nil {
		fmt.Fprintln(os.Stderr, "error: reply check needs your Email + Gmail app password (Config → Outreach)")
		os.Exit(1)
	}
	st, err := store.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	discordURL, tgToken, tgChatID, channels := cfg.NotifyFields()
	mn := notifier.FromConfig(&notifier.NotifyConfig{
		DiscordWebhookURL: discordURL,
		TelegramBotToken:  tgToken,
		TelegramChatID:    tgChatID,
		EnabledChannels:   channels,
	})

	fmt.Println("⚡ Nexus — checking inbox for replies…")
	rep, err := outreach.RunReplyCheck(context.Background(), cfg, st, mn, fetcher,
		func(line string) { fmt.Println("  " + line) })
	if err != nil {
		fmt.Fprintf(os.Stderr, "reply check: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Scanned %d message(s) · %d human repl(ies) · %d rejection(s)\n",
		rep.Scanned, len(rep.HumanReplies), len(rep.Rejections))

	sent, fuErrs := outreach.SendDueFollowUps(cfg, func(line string) { fmt.Println("  " + line) })
	fmt.Printf("Follow-ups sent: %d\n", sent)
	for _, e := range append(rep.Errors, fuErrs...) {
		fmt.Fprintln(os.Stderr, "  ⚠ "+e)
	}
}
