# Adding a new job-board provider

Every provider is a plugin: a package under `internal/provider/<name>/` implementing
the `provider.Provider` interface (`internal/provider/provider.go`). The engine never
special-cases a provider by name — new boards are added, not switched on.

Read `internal/provider/workable/` first (small, ~340 lines total) as a working
reference; `internal/provider/lever/` and `internal/provider/greenhouse/` are larger
references that also do AI-assisted question answering (see §14 of AGENTS.md for the
grounding/CAPTCHA rules that applies to that).

## 1. Interface you must implement

```go
type Provider interface {
    Name() string
    Search(ctx context.Context, c SearchCriteria) ([]Job, error)
    Apply(ctx context.Context, j Job, p Profile) (ApplyResult, error)
}
```

If your board is a per-company ATS scanned from a seed list (like Workable,
Greenhouse, Lever), also implement `provider.BoardMerger` so the engine can
extend the embedded company list at runtime:

```go
type BoardMerger interface {
    MergeBoards(extra []NamedBoard)
}
```

## 2. Minimal skeleton

```go
// Package <name> implements provider.Provider for the <Name> job board.
package <name>

import (
    "context"
    "net/http"
    "time"

    "github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// Client implements provider.Provider for <Name>.
type Client struct {
    http *http.Client
    // ...board-specific state (embedded company list, base for MergeBoards, etc.)
}

// New creates a <Name> client.
func New( /* embedded seed data, config */ ) (*Client, error) {
    return &Client{
        http: &http.Client{Timeout: 30 * time.Second}, // always set a timeout (§10)
    }, nil
}

func (c *Client) Name() string { return "<name>" }

// Search returns jobs matching the given criteria.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
    // Bounded concurrency (semaphore/worker pool), ctx-aware, one board's
    // failure must never abort the whole search (§9, §10).
    return nil, nil
}

// Apply submits an application for a single job.
func (c *Client) Apply(ctx context.Context, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
    return provider.ApplyResult{}, nil
}
```

## 3. Wiring it in

Register the new provider **only** at its single construction point in
`internal/engine/engine_new.go` (import + `New(...)` call, following the
existing providers there). Do not add `if name == "<name>"` branches anywhere
else in the engine — that is the anti-pattern this plugin structure exists to
avoid (§8, §9).

## 4. AI-assisted custom questions (optional)

If the board has free-text/custom screening questions and you're wiring in AI
answering, reuse the pattern from `internal/provider/lever/questions.go` +
`answer.go`, not a new one:

- Answer questions one at a time with prior answers as context (batch answering
  with local models is unreliable).
- **Ground every numeric claim** (salary, notice period, years of experience,
  etc.) against resume/profile facts before using it — see the word-boundary
  check in `lever/answer.go`. This is a hard rule (AGENTS.md §14), not optional.
- If the apply page presents a CAPTCHA/anti-bot challenge, detect and halt —
  never attempt to solve or bypass it (AGENTS.md §14).

## 5. Before you finish

Run through AGENTS.md §17 (Definition of Done) — in particular: table-driven
tests including failure paths, `./scripts/verify.ps1` (or `.sh`) green, and a
package doc comment on the new package.
