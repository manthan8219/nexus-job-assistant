// Package ui is the Bubble Tea TUI for Nexus. The root AppModel owns every
// tab (Dashboard, Config, Resume, Jobs/History, Companies, Outreach,
// Contacts, Logs), the chrome header/footer/body renderers, and tab
// switching. The Update dispatcher lives in app_update.go and async commands
// in app_commands.go.
//
// # Why one flat package
//
// Every tab model (FormModel, OutreachHubModel, HistoryModel, …) shares
// unexported symbols across files: the styles palette (styles.go), field
// index constants (form_fields.go), helper renderers (divider,
// placeholderView), and the message types that route between AppModel.Update
// and each sub-model (ScraperScanMsg, ResumeAnalysisDoneMsg, RefreshStatsMsg,
// …). Splitting these into sub-packages would force exporting all of that and
// break the message routing, so the filename-prefix convention below is the
// in-package grouping instead — each concern keeps a stable file prefix.
//
// # File manifest (grouped by concern)
//
// Root app chrome:
//
//	app.go               AppModel state, tab constants, chrome View render
//	app_update.go        AppModel.Update dispatcher + per-message handlers
//	app_commands.go      async commands (engine, scraper scan, stats, replies, usage, test notify)
//	styles.go            shared color palette, lipgloss styles, divider/placeholder helpers
//	dashboard.go         Dashboard tab model + view
//	browser.go           openURL helper: launch a URL in the default browser
//
// Config form (form_*.go — the FormModel):
//
//	form.go              FormModel struct, constructor, lifecycle (Init/InitCmd/ConsumesEscape/CustomFieldActive)
//	form_fields.go       field index constants, labels, placeholders, section grouping
//	form_keys.go         Config form key dispatch
//	form_update.go       FormModel.Update dispatch
//	form_view.go         top-level render loop walking visible fields
//	form_render.go       per-field rendering helpers
//	form_widgets.go      custom widgets (tag pills, work-type checkboxes, channel checkboxes, pickers)
//	form_scroll.go       body scrolling when content overflows
//	form_config.go       form ↔ config.Config serialization + auto-save
//	form_complete.go     profile-completion checks + focus helpers
//	form_llm.go          local LLM (Ollama) picker + setup menu
//	form_resume.go       resume path field + async analysis commands
//	form_job_titles.go   job-title tag input + AI suggestions
//	form_locations.go    location tag input + autocomplete
//	form_scraper.go      career-scraper field + setup menu
//	form_outreach.go     outreach fields on the Config form
//	form_apply_safety.go apply-consent, work-auth, and rate-limit fields
//	form_messages.go     async message types (resume analysis, job-title suggestions, test notifications)
//
// Companies tab (companies_*.go):
//
//	companies_tab.go     model + Update + data helpers
//	companies_view.go    rendering (list, badges, scraped-jobs detail)
//	companies_keys.go    modal key handlers (detail view, search, add-company form)
//
// Outreach tab (outreach_*.go — UI only; pipeline logic is in internal/outreach):
//
//	outreach_hub.go      model + Update + helpers (config sync, sub-tab nav)
//	outreach_view.go     rendering (Setup/Email/LinkedIn/Sent sub-tabs)
//	outreach_keys.go     key handlers
//
// Resume tab:
//
//	resume_hub.go        hub model + sub-tab routing
//	resume_tab.go        analysis view (fit score + JD)
//
// Jobs/History tab:
//
//	history.go           model + Update (list nav, search, detail, outcome, enrich)
//	history_view.go      list table view (columns, badges, outcome funnel)
//	history_detail.go    detail pane (full JD + fit rendering)
//
// Other tabs:
//
//	contacts_tab.go      Contacts tab (OSINT contact discovery + clipboard)
//	improve_tab.go       Resume-improvement tab (AI suggestions)
//	work_tab.go          Work-context tab (project/work-history editor)
//	logs.go              Logs/diagnostics tab (usage snapshot)
//
// Tests:
//
//	form_nav_test.go        form navigation
//	form_resume_lock_test.go resume-lock logic
//	subnav_test.go          sub-menu tab navigation
package ui
