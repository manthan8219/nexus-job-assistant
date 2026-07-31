// Package outreach runs the recruiter outreach pipeline: discover a contact,
// draft a message (template or AI), review it, mark it ready, send it, run a
// follow-up sequence, and detect replies. One Item (models.go) flows through
// these Status stages; the Worker (worker.go) drives the transition per item.
//
// Pipeline stages → files:
//
//	find.go        contact discovery via OSINT + stored application history
//	finder.go      Contact type + email/domain derivation (regex helpers)
//	browser.go     LinkedIn people-search URL builder
//	draft.go       JobRef + draft creation from a job + config
//	compose.go     AI generator/reviewer LLMs (ComposeInput)
//	email_send.go  SMTP send + sentLogger audit hook
//	gmail.go       Gmail API send path
//	imap.go        IMAP inbox fetch for replies
//	followups.go   follow-up cadence (deltas) + sequence logic
//	queue.go       queue build + actionable/pending status gating
//	ready.go       Setup readiness checks (Check) for the UI
//	replies.go     Reply type + reply matching against sent items
//	replycheck.go  reply-check pass → ReplyReport + notifier fan-out
//	worker.go      Worker orchestrating the full pipeline
//
// State & storage:
//
//	models.go   Channel/Status enums + Item struct + StoreFile
//	mode.go     run modes (confirm | queue | auto)
//	store.go    JSON file store (Load/Save under ~/.nexus)
//
// Tests:
//
//	followups_test.go follow-up sequence
//	imap_test.go      IMAP parsing
//	mode_test.go      run-mode resolution
//	replies_test.go   reply matching
package outreach
