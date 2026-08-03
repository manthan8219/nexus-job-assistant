package ui

// Package ui — app_commands.go
// Async command functions for the AppModel: engine, scraper, history enrichment,
// reply checking, usage monitoring, and test notifications.

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/engine"
	"github.com/manthan8219/nexus-job-assistant/internal/enrich"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
	"github.com/manthan8219/nexus-job-assistant/internal/usage"
)

func runScraperScanCmd(cfg *config.Config, keywords []string, logCh chan string) tea.Msg {
	return scraperScanDoneMsg{}
}
func runEngineCmd(eng *engine.Engine, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		err := eng.RunOnce(ctx)
		return EngineDoneMsg{Err: err}
	}
}
func waitForLog(ch chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return nil
		}
		return AppendLogMsg{Line: line}
	}
}
func waitForResult(ch chan engine.Result) tea.Cmd {
	return func() tea.Msg {
		r, ok := <-ch
		if !ok {
			return nil
		}
		return EngineResultMsg{Result: r}
	}
}
func waitForProgress(ch chan engine.ProviderProgress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}
		return ProviderProgressMsg{P: p}
	}
}

func (m AppModel) loadStats() tea.Cmd {
	st := m.st
	return func() tea.Msg {
		applied, skipped, failed, _ := st.Stats()
		now := time.Now()
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		appliedToday, _ := st.CountAppliedSince(dayStart)
		return RefreshStatsMsg{Applied: applied, Skipped: skipped, Failed: failed, AppliedToday: appliedToday, Recent: nil}
	}
}

func (m AppModel) enrichHistoryCmd(req HistoryEnrichRequestMsg) tea.Cmd {
	st := m.st
	cfg := m.config.toConfig()
	return func() tea.Msg {
		events := make(chan historyEnrichDoneOrProgress, 100)
		go runHistoryEnrich(events, st, cfg, req)
		return listenHistoryEnrich(events)
	}
}

type historyEnrichDoneOrProgress struct {
	done *historyEnrichDoneMsg
	prog *historyEnrichProgressMsg
}

func listenHistoryEnrich(events <-chan historyEnrichDoneOrProgress) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return nil
		}
		if ev.done != nil {
			return *ev.done
		}
		next := listenHistoryEnrich(events)
		return historyEnrichProgressMsg{Line: ev.prog.Line, Next: next}
	}
}

func runHistoryEnrich(events chan<- historyEnrichDoneOrProgress, st *store.Store, cfg *config.Config, req HistoryEnrichRequestMsg) {
	defer close(events)
	emit := func(line string) {
		events <- historyEnrichDoneOrProgress{prog: &historyEnrichProgressMsg{Line: line}}
	}
	finish := func(msg historyEnrichDoneMsg) {
		events <- historyEnrichDoneOrProgress{done: &msg}
	}
	apps, err := st.List()
	if err != nil {
		finish(historyEnrichDoneMsg{Err: err})
		return
	}
	if !req.All {
		apps = []store.Application{req.App}
	}
	ai := resume.AIOptions{}
	resumeText := ""
	if cfg != nil {
		ai = resume.AIOptionsFromConfig(cfg)
		if ai.Enabled && strings.TrimSpace(cfg.ResumePath) != "" {
			if text, err := resume.ExtractText(cfg.ResumePath); err == nil {
				resumeText = text
				emit("[enrich] AI fit scoring enabled (resume loaded)")
			} else {
				emit(fmt.Sprintf("[enrich] AI on but resume unavailable: %v — descriptions only", err))
			}
		} else if ai.Enabled {
			emit("[enrich] AI on but no resume path — descriptions only")
		} else {
			emit("[enrich] AI Assist off — fetching descriptions only")
		}
	}
	updated, failed := 0, 0
	var lastErr error
	for i, app := range apps {
		label := fmt.Sprintf("%s @ %s", app.Role, app.Company)
		emit(fmt.Sprintf("[enrich] (%d/%d) fetch description: %s (%s) …", i+1, len(apps), label, app.Provider))
		timeout := 25 * time.Second
		if app.Provider == "linkedin" || app.Provider == "careerscraper" {
			timeout = 60 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		desc, err := enrich.FetchDescription(ctx, app.Provider, app.URL)
		cancel()
		if err != nil {
			failed++
			lastErr = err
			emit(fmt.Sprintf("[enrich] ✗ fetch failed: %s — %v", label, err))
			continue
		}
		emit(fmt.Sprintf("[enrich] ✓ description (%d chars): %s", len(desc), label))
		fitScore, fitSummary := app.FitScore, app.FitSummary
		if ai.Enabled && strings.TrimSpace(resumeText) != "" {
			emit(fmt.Sprintf("[enrich] scoring fit vs resume: %s …", label))
			job := provider.Job{
				Title: app.Role, Company: app.Company, Location: app.Location,
				Remote: app.Remote, URL: app.URL, Provider: app.Provider,
				Description: desc,
			}
			fitCtx, fitCancel := context.WithTimeout(context.Background(), 55*time.Second)
			res, scoreErr := resume.ScoreJobFit(fitCtx, ai, resumeText, job)
			fitCancel()
			if scoreErr != nil {
				emit(fmt.Sprintf("[enrich] ~ fit skip: %s — %v", label, scoreErr))
			} else {
				fitScore, fitSummary = res.Score, res.Summary
				sum := strings.TrimSpace(fitSummary)
				if len(sum) > 80 {
					sum = sum[:79] + "…"
				}
				emit(fmt.Sprintf("[enrich] ✓ fit %d/100 — %s", fitScore, sum))
			}
		}
		if err := st.UpdateDescriptionFit(app.URL, desc, fitScore, fitSummary); err != nil {
			failed++
			lastErr = err
			emit(fmt.Sprintf("[enrich] ✗ save failed: %s — %v", label, err))
			continue
		}
		updated++
	}
	status := ""
	if !req.All && updated == 0 && failed == 1 && lastErr != nil {
		status = lastErr.Error()
	}
	finish(historyEnrichDoneMsg{Updated: updated, Failed: failed, Status: status})
}

func usageTickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return usageTickMsg{} })
}

func (m AppModel) loadUsage() tea.Cmd {
	st := m.st
	cfg := m.config.toConfig()
	aiMode := ""
	if cfg != nil {
		aiMode = cfg.AIProvider
	}
	return func() tea.Msg {
		dir, err := config.Dir()
		if err != nil {
			dir = ""
		}
		jobs := 0
		if st != nil {
			if apps, e := st.List(); e == nil {
				jobs = len(apps)
			}
		}
		snap := usage.Collect(dir, jobs, aiMode)
		return RefreshUsageMsg{Snap: snap}
	}
}

const replyCheckInterval = 10 * time.Minute

func (m AppModel) replyCheckCmd(background bool) tea.Cmd {
	cfg := m.config.toConfig()
	st := m.st
	mn := m.eng.Notifier
	return func() tea.Msg {
		report, err := outreach.RunReplyCheck(context.Background(), cfg, st, mn, nil, func(string) {})
		if err != nil {
			return replyCheckDoneMsg{Err: err, Background: background}
		}
		text := fmt.Sprintf("Inbox check: %d replies, %d rejections", len(report.HumanReplies), len(report.Rejections))
		return replyCheckDoneMsg{Text: text, Replies: len(report.HumanReplies), Rejections: len(report.Rejections), Background: background}
	}
}

func scheduleReplyCheckTick() tea.Cmd {
	return tea.Tick(replyCheckInterval, func(time.Time) tea.Msg { return replyCheckTickMsg{} })
}

func (m AppModel) loadHistory() tea.Cmd {
	st := m.st
	return func() tea.Msg {
		apps, err := st.List()
		if err != nil {
			apps = nil
		}
		outcomes, _ := st.OutcomeStats()
		return RefreshHistoryMsg{Apps: apps, Outcomes: outcomes}
	}
}

type testNotifyResultMsg struct{ err error }

func sendTestNotifyCmd(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		ncfg := &notifier.NotifyConfig{
			DiscordWebhookURL: cfg.DiscordWebhookURL,
			TelegramBotToken:  cfg.TelegramBotToken,
			TelegramChatID:    cfg.TelegramChatID,
			EnabledChannels:   cfg.NotifyChannels,
		}
		mn := notifier.FromConfig(ncfg)
		if len(mn) == 0 {
			return testNotifyResultMsg{err: fmt.Errorf("no channels ready — fill credentials and enable them under Notify on Apply")}
		}
		var parts []string
		for _, n := range mn {
			if err := n.Send(context.Background(), notifier.Event{Kind: notifier.EventCustom, Message: "Test notification from Nexus"}); err != nil {
				parts = append(parts, err.Error())
			}
		}
		if len(parts) > 0 {
			return testNotifyResultMsg{err: fmt.Errorf("%s", strings.Join(parts, "; "))}
		}
		return testNotifyResultMsg{}
	}
}
