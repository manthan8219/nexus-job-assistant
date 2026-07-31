# Contributing to Nexus

Thanks for your interest! The most impactful contribution is **adding a new job-board provider** — each one unlocks more companies for every user. This guide walks you through the full process.

---

## Table of contents

1. [Getting started](#getting-started)
2. [Project layout](#project-layout)
3. [Adding a new provider (the main path)](#adding-a-new-provider)
4. [Other contribution areas](#other-contribution-areas)
5. [Running tests](#running-tests)
6. [Opening a pull request](#opening-a-pull-request)
7. [Code style](#code-style)

---

## Getting started

```bash
git clone https://github.com/manthan8219/nexus-job-assistant
cd nexus-job-assistant
go build ./...   # should produce zero errors
go test ./...    # all tests pass
```

**Requirements:** Go 1.22+. No other runtime dependencies for the core engine.

---

## Project layout

```
internal/
  provider/<name>/   ← one package per job board  ← best place to contribute
  engine/            ← orchestrates providers, rate limits, results
  store/             ← SQLite persistence
  ui/                ← Bubble Tea TUI
  config/            ← user config load/save
data/
  companies.json     ← embedded Greenhouse company list
  *.json             ← per-ATS company seed files
```

---

## Adding a new provider

Every provider is a **plugin**: a package under `internal/provider/<name>/` that implements one Go interface. The engine never special-cases a provider by name — you add boards, you don't switch them on.

See `internal/provider/TEMPLATE.md` for the full spec. Here is the short version:

### Step 1 — implement the interface

```go
// internal/provider/provider.go
type Provider interface {
    Name()   string
    Search(ctx context.Context, c SearchCriteria) ([]Job, error)
    Apply(ctx context.Context, j Job, p Profile) (ApplyResult, error)
}
```

**Good reference implementations (read these first):**

| Provider | Type | Why read it |
|---|---|---|
| `remoteok` | Board-wide aggregator (JSON feed) | Smallest, cleanest example |
| `hackernews` | Board-wide aggregator (HTML parse) | Shows comment parsing |
| `workable` | Per-company ATS + `BoardMerger` | Mid-size, shows company list expansion |
| `greenhouse` | Per-company ATS + AI answers | Full-featured reference |

### Step 2 — create the package

```
internal/provider/<name>/
  client.go     ← New(), Name(), Apply()
  search.go     ← Search() + HTTP helpers
  api_types.go  ← JSON structs (if JSON API)
```

Minimal skeleton:

```go
package <name>

import (
    "context"
    "net/http"
    "time"

    "github.com/manthan8219/nexus-job-assistant/internal/provider"
)

type Client struct{ http *http.Client }

func New() *Client {
    return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "<name>" }

func (c *Client) Search(ctx context.Context, cr provider.SearchCriteria) ([]provider.Job, error) {
    // fetch → filter with provider.MatchesTitle / provider.MatchesLocation → return
    return nil, nil
}

func (c *Client) Apply(ctx context.Context, j provider.Job, p provider.Profile) (provider.ApplyResult, error) {
    // if no apply API: return ApplyResult{Status: "skipped", Reason: "apply manually at " + j.URL}
    return provider.ApplyResult{}, nil
}
```

### Step 3 — wire it into the engine

Open `internal/engine/engine_new.go` and add **one import + one `New()` call**, following the pattern of any existing provider. That is the only file you touch in the engine.

```go
import "github.com/manthan8219/nexus-job-assistant/internal/provider/<name>"

// inside New():
p := <name>.New()
providers = append(providers, p)
```

### Step 4 — add a test

At minimum a table-driven test for `Search()` with:
- A success case (golden response → expected jobs)
- A filter case (title/location mismatch → 0 jobs returned)
- An HTTP error case

See `internal/provider/remoteok/search_test.go` for a clean example.

### Step 5 — checklist before opening a PR

- [ ] `go build ./...` passes
- [ ] `go test ./...` passes (including your new package)
- [ ] `go vet ./...` clean
- [ ] Package has a doc comment: `// Package <name> implements provider.Provider for <Name>.`
- [ ] `http.Client` has a `Timeout` set (never use the zero-value default)
- [ ] `Search()` respects `ctx` cancellation (pass ctx to every HTTP request)
- [ ] One board's failure does not abort other boards (return error, don't panic)
- [ ] If the board requires an API key, document it clearly in the package doc and skip gracefully when the key is missing

---

## Other contribution areas

| Area | What to look for |
|---|---|
| **Providers (unimplemented)** | `instahyre`, `hirist`, `cutshort`, `workatastartup`, `echojobs` — see their `provider.go` for status notes |
| **Company seed data** | Add companies to `data/*.json` files (just JSON, no Go code) |
| **Bug fixes** | Check [open issues](https://github.com/manthan8219/nexus-job-assistant/issues) |
| **Tests** | Many providers under `internal/provider/` have no tests yet |

---

## Running tests

```bash
go test ./...                          # all packages
go test ./internal/provider/remoteok/  # one package
go test -run TestSearch ./...          # one test name
go test -v ./internal/ui/             # verbose (TUI model tests)
```

Tests do **not** hit live APIs — they use local fixtures or mock HTTP servers.

---

## Opening a pull request

1. Fork the repo and create a branch: `git checkout -b provider/<name>`
2. Make your changes
3. Run `go test ./...` and `go vet ./...`
4. Open a PR against `main` with a short description of:
   - What board you added / what bug you fixed
   - How you tested it (even manually)
   - Any known limitations (e.g. "Apply not implemented, Search only")

PRs that add a working `Search()` — even without `Apply()` — are welcome. Incremental is fine.

---

## Code style

- Standard `gofmt` formatting (run `gofmt -w .` before committing)
- No third-party HTTP client libraries — use `net/http` directly
- Errors wrapped with `fmt.Errorf("context: %w", err)`, not discarded
- No `log.Fatal` / `os.Exit` inside packages — return errors to the caller
- Keep packages small and focused; one file per concern is fine

Questions? Open an issue or start a discussion. Happy to help.
