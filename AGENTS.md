# AGENTS.md — Nexus Job Assistant

> **Read this file FIRST, before writing or modifying any code in this repository.**
> This file is the constitution of this codebase. It is mandatory for every AI coding agent (Cline, Copilot, Cursor, Codex, Aider, …) and every human contributor.
> If a quick fix conflicts with a rule here, follow the rule — or stop and ask (§16).

---

## 1. Project Overview

**Nexus** is a Go CLI/TUI application that automates job searching and applying.

- **Language:** Go 1.26+ (module: `github.com/manthan8219/nexus-job-assistant`)
- **UI:** Bubble Tea TUI (`charmbracelet/bubbletea`, `lipgloss`) — *the TUI owns the terminal in TUI mode* (§11)
- **Storage:** SQLite via `modernc.org/sqlite` (data in `~/.nexus/`)
- **Browser automation:** `playwright-go` (form filling / applying)
- **Docs:** `ledongthuc/pdf` (read), `phpdave11/gofpdf` (write) for resumes
- **Entry point:** `main.go` at repo root (flags → TUI dashboard or headless `--run` engine)

### Repository layout

| Path | Purpose |
|---|---|
| `main.go` | Root binary: flag parsing, TUI vs engine mode wiring — *no business logic* |
| `cmd/<tool>/` | Small standalone utilities (`go run ./cmd/test-scraper`, …) |
| `internal/engine` | Apply engine: orchestrates providers, rate limits, results |
| `internal/provider/<name>/` | One package per job board (greenhouse, lever, …) — **plugin pattern** |
| `internal/store` | SQLite persistence (applications, sessions) |
| `internal/companies` | Local employer DB (seeded from `data/companies.json`) |
| `internal/config` | User config load/save (`~/.nexus/config.json`) |
| `internal/notifier` | Notification channels behind a `Notifier` interface (Discord, Telegram, …) |
| `internal/scraper` | Job-page scraping |
| `internal/osint` / `internal/outreach` | Recruiter contact discovery + outreach |
| `internal/ui` | Bubble Tea dashboard |
| `internal/textutil` / `internal/geo` | **Shared utilities — check here before writing helpers** |
| `data/` | Seed data (companies.json, …) |
| `scripts/` | One-off helper scripts |

---

## 2. Golden Rules (non-negotiable)

1. **READ BEFORE YOU WRITE.** Before adding any function/package, search the codebase for existing code that does it or almost does it. `internal/textutil`, `internal/geo`, and `internal/config` already hold shared helpers.
2. **DRY — zero duplicate code.** The same logic must never appear twice. Extract/extend the shared version (§7).
3. **REUSE BEFORE YOU CREATE.** New capability → extend an existing package. New job board → implement the existing provider pattern. New channel → implement `notifier.Notifier`. Never fork a parallel implementation.
4. **YAGNI / KISS — build ONLY what was asked.** No speculative features, no extra options "for flexibility", no abstraction with a single caller, no "while I'm here" improvements, no unrequested files (including summary/report `.md` files). If you spot a worthwhile unrelated improvement, put it in your final report — do not implement it.
5. **NO PLACEHOLDERS.** Never leave `TODO`, `// implement me`, stubbed bodies, or half-finished code. Deliver complete, compiling, working code or explicitly state what is missing and why.
6. **SMALLEST CHANGE THAT WORKS.** Touch only files relevant to the task. No drive-by refactors, no renames, no reformatting of unrelated code. Deleting dead code is encouraged; moving/renaming working code is not.
7. **FOLLOW EXISTING CONVENTIONS.** Match the style, naming, and patterns of the package you are editing — even over your personal preference.
8. **VERIFY BEFORE YOU FINISH.** Every change must compile and pass tests (§4). Never claim "done" on code you have not built and tested.
9. **NO NEW DEPENDENCIES without justification.** Check `go.mod` first — prefer stdlib and already-vendored deps (bubbletea, sqlite, playwright, gofpdf…). Adding a dependency requires explicit justification in your report.
10. **ASK BEFORE IRREVERSIBLE.** Schema changes, deleting data/files, changing consent or rate-limit behavior, breaking config format, renaming exported APIs → stop and ask first (§16).

---

## 3. Rule Zero — When Rules Conflict

Rules collide. Resolve conflicts in this strict priority order:

1. **Correctness & user safety** — never lose/corrupt user data, never double-apply, never violate consent or rate limits.
2. **Security & privacy** — secrets, personal data, injection vectors.
3. **Simplicity & readability** — clear beats clever; boring beats brilliant.
4. **Consistency** — match the existing codebase.
5. **Performance** — only with measurement (§10).

**Worked example:** DRY (rule §7) never justifies a wrong abstraction — simplicity (3) outranks deduplication. *Duplication is far cheaper than the wrong abstraction.* If two code paths only look similar but evolve for different reasons, keep them separate until the third concrete use proves they are one concept.

---

## 4. Commands (build / test / verify)

```powershell
go build ./...        # MUST pass before you finish any change
go vet ./...          # MUST be clean
gofmt -l .            # MUST output nothing (all files formatted)
go test ./...         # MUST pass
go test -race ./...   # MUST pass for any change touching goroutines/channels/shared state
go test ./internal/geo -run TestAliases   # run a single package/test
go run . --help                             # run the CLI
go run ./cmd/test-scraper                   # run a cmd/ utility
go mod tidy           # after adding/removing any import
```

After editing: run `gofmt -w .` on changed files, then `go build ./... && go vet ./... && go test ./...`.

---

## 5. Code Style (Go, idiomatic — per Effective Go & Go Code Review Comments)

### Formatting & imports
- `gofmt` (or `goimports`) on everything. No hand-aligned oddities; let the tool decide.
- Import grouping: stdlib block, blank line, third-party block (as in `main.go`). No dot imports. Blank imports only when truly needed (driver registration), with a comment.

### Naming
- **Packages:** short, lowercase, single word, no underscores (`textutil`, `notifier`, `geo`).
- **Locals:** short names for short scopes (`i`, `r`, `cfg`, `st`); the further a name travels from its declaration, the more descriptive it must be.
- **Exported names:** `CamelCase`; unexported: `camelCase`. Initialisms stay all-caps: `URL`, `HTTP`, `ID`, `ATS` (not `Url`, `Http`, `Id`).
- **Receivers:** 1–2 letters, consistent across all methods of a type (`func (s *Store) …`, never `this`/`self`).
- Interfaces that do one thing end in `-er` where natural (`Notifier`, `BoardMerger`).

### Comments & docs
- Every **package** has a doc comment (`// Package notifier …`) — see `internal/notifier/notifier.go`, `internal/companies/doc.go` for the house style.
- Every **exported** symbol has a doc comment that is a full sentence starting with its name and ending in a period:
  `// LoadFrom reads a Config from path.` — not `// loads config`.
- Comment *why*, not *what*. The code already says what it does.

### Functions & control flow
- Keep functions short and single-purpose (§6 SRP). If a function needs a comment per block, it wants to be several functions.
- **Handle the error first, indent the happy path** — early return, not nested `if` pyramids.
- Avoid naked returns in long functions; avoid `panic` in library code (return `error`).
- Prefer **synchronous functions**; let the caller add goroutines. Every goroutine you spawn must have an obvious owner and exit path (§10).
- `context.Context` is the first parameter of any function that does I/O or can block: `func Fetch(ctx context.Context, url string) (…, error)`. Thread it through; don't store it in structs; never swallow cancellation.

### Types, generics & state
- Default to **concrete types**. `any` only for genuinely heterogeneous data (e.g. JSON decoding); generics only when 2+ real instantiations exist — never for a single use.
- **No mutable package-level state.** Package-level *immutable* values are fine: sentinel errors, compiled regexes (`regexp.MustCompile`), lookup tables. Mutable globals and singletons are not (§9).
- No `init()` with side effects (I/O, network, goroutines). Initialization belongs in constructors (`engine.New`, `store.Open`).
- **Time:** pass `time.Time`, never strings; serialize as RFC3339. Business logic that depends on "now" takes it as a parameter (or a small clock func) so tests can control it — no scattered `time.Now()` calls.
- **Paths:** `filepath.Join`, never string concatenation; `os.UserHomeDir()` for `~/.nexus`. Windows-first dev, cross-platform code.

### Errors
- **Always handle errors** — never `_ =` them away silently (a justified ignore gets a comment).
- Error strings: lowercase, no trailing punctuation: `errors.New("store: no such company")`.
- Wrap with context as errors travel up: `fmt.Errorf("engine: fetch %s: %w", url, err)`.
- Define sentinel errors (`var ErrNotFound = errors.New(...)`) when callers must branch on them; check with `errors.Is` / `errors.As`.
- Return `error` as the **last** return value. Don't mix errors into return values (no "in-band" errors).
- **User-facing errors must be actionable** — follow the existing pattern: `run 'nexus' to open the TUI and fill in your profile`. Tell the user what to do next, not just what failed.

---

## 6. SOLID Principles (applied to Go)

These five principles (Robert C. Martin) govern all design in this repo. Rule Zero (§3) still applies when they conflict.

### S — Single Responsibility
> *A type/package/function should have one — and only one — reason to change.*

- One package = one concern: `notifier` notifies, `store` persists, `scraper` scrapes. Never let a provider package parse resumes or a UI package talk to SQLite directly.
- One function = one job. If you describe a function with "and", split it.
- **Bad:** a `Provider` that scrapes, persists, *and* sends Discord messages. **Good:** provider returns jobs; engine persists; notifier notifies (exactly how this repo is structured — keep it that way).

### O — Open/Closed
> *Open for extension, closed for modification.*

- Add behavior by adding new implementations, not by editing working code.
- **In practice:** a new job board = a new `internal/provider/<name>` package implementing the existing provider interface. You must NOT need to modify other providers or switch-statements scattered around the engine. Same for notifiers and stores.

### L — Liskov Substitution
> *Implementations of an interface must be interchangeable without breaking the caller.*

- Every `Notifier` must honor the contract of `Send(ctx, ev) error` — no implementation may panic, ignore `ctx` cancellation, or require special pre-calls the interface doesn't declare.
- If one implementation needs special handling in the caller, the interface is wrong — fix the interface, don't add `if name == "discord"` hacks.

### I — Interface Segregation
> *Many small interfaces beat one fat interface.*

- Go idiom: interfaces of 1–3 methods, defined **where they are used**, not in a giant `interfaces.go`.
- Don't force a provider to implement `Apply()` if it can only `Search()` — split the interfaces and compose.
- Keep `Notifier` minimal (`Name`, `Send`) — do not grow it into a god-interface.

### D — Dependency Inversion
> *Depend on abstractions, not concretes. High-level modules must not depend on low-level modules.*

- The engine depends on the `Notifier` interface and a store abstraction — never on `DiscordNotifier` or a concrete SQLite type.
- **Accept interfaces, return concrete structs** (Go proverb): constructors (`engine.New`, `store.Open`) return `*T`; function parameters take the smallest interface that covers the need (`io.Reader`, `Notifier`).
- Inject dependencies through constructors/parameters — including clocks, HTTP clients, and notifiers. **No package-level mutable singletons** for anything you might fake in tests.

---

## 6b. Design Patterns for Extensibility

> *Use design patterns intentionally — where they create a clear extension point for future code. Never add a pattern speculatively ("just in case"). YAGNI (§2 rule 4) still applies: the pattern must solve a real, present need that has a high likelihood of growing.*

### Patterns already in use in this codebase

| Pattern | Where | Why |
|---|---|---|
| **Provider / Plugin** | `internal/provider/<name>/` | Each job board implements the same interface; adding a new board means dropping in a new package, zero changes to existing providers. |
| **Strategy** | `internal/notifier/` (Notifier interface) | Each notification channel (Discord, Telegram, …) is a pluggable strategy behind `Notifier.Send()`. The engine picks strategies at runtime from config. |
| **Strategy** | `internal/scraper/py/` (LLMExtractionStrategy) | Extraction method is a strategy passed to the crawler; swap models without changing crawl logic. |
| **Factory / Factory Method** | `notifier.FromConfig()` | Constructs the right set of `Notifier` implementations based on config — callers never instantiate concrete notifiers directly. |
| **Dependency Injection** | `engine.New(cfg, store, companies)`, `notifier.FromConfig()` | Dependencies are injected through constructors, not created internally. Enables testing with fakes. |
| **Repository** | `internal/store/`, `internal/companies/` | Data access abstracted behind a store interface; the rest of the code queries companies/applications through it, not SQL directly. |

### When to reach for a pattern

Consider a design pattern when you see **two or more** of these signals:

1. **You are adding a new implementation of an existing interface.** That's already a Strategy pattern — lean into it.
2. **You are writing a switch/if-else chain that dispatches on type or name.** That's a smell for a Strategy, Factory, or Registry pattern — extract the variant into its own type.
3. **You need to construct a family of related objects based on config or context.** Use a Factory (constructor function) or Factory Method, never a `switch` scattered across callers.
4. **You are wrapping behavior around a core operation (logging, retry, timing).** The Decorator pattern (a struct wrapping an interface) is idiomatic Go — see `http.RoundTripper` wrappers.
5. **You need to notify multiple consumers when something happens, without the producer knowing them.** The Observer pattern (via channels or callback slices) is appropriate — but prefer Go channels over a generic `Observer` interface.
6. **An algorithm has a fixed skeleton but steps vary.** Template Method (a function that accepts interface-typed callbacks/strategies for its variable steps) keeps the structure in one place.

### Balancing patterns with YAGNI

- **✅ Do use a pattern** when you already have a second concrete variant and a third is likely (the "Rule of Three" from §7 applies here too).
- **✅ Do use a pattern** when the interface/abstraction is the *simplest* correct expression of the code (e.g., accepting `io.Reader` instead of `*os.File` is just good Go, not over-engineering).
- **❌ Don't add a pattern** for a single concrete use "because we might need it later." Duplication is cheaper than the wrong abstraction (§3).
- **❌ Don't add a pattern** that requires new exported types, interfaces, or registration points unless the current code genuinely needs the flexibility it provides.
- **❌ Don't use reflection, `any`-based generics, or complex object hierarchies** to force a pattern. Go patterns are simple: interfaces, structs, functions, closures, channels.

### Adding a new pattern

If you introduce a design pattern that is not already established in this codebase, document it in the relevant package doc comment and mention it in your report (§16). The guiding principle: **the pattern should make the code simpler and more obviously correct, not more abstract.**

---

## 7. DRY & Reuse Playbook (the "no duplicate code" contract)

**Before writing any new helper, run this checklist:**

1. **Search first.** Search for the capability in `internal/` — especially `internal/textutil` (string/text helpers), `internal/geo` (location/country logic), `internal/config` (paths, settings). If it exists, use it. If it's 80% right, **generalize it** — don't clone it.
2. **Rule of three.** Similar code needed a 2nd time → strongly consider extracting. A 3rd time → extraction is mandatory into the shared package. But remember §3: duplication is cheaper than the *wrong* abstraction — extract only when the shared concept is real.
3. **Copy-paste is forbidden.** If you find yourself copying a block, stop — extract a function with parameters for the differences.
4. **One source of truth per concept.** Defaults, limits, regexes, table names, env-var names → named constants in exactly one place (rate-limit defaults live with the engine/config, not re-typed in `main.go`, UI, and each provider).
5. **Extend the plugin points.** New job board → `internal/provider/<name>` implementing the shared provider interface; new channel → new `Notifier`. Reuse shared HTTP, retry, delay, and parsing helpers across all of them.
6. **Shared utilities go down, never up.** Helpers used by 2+ packages belong in the lowest common package (`textutil`, `geo`) — never duplicated per-package, and never in `main`.
7. **When you refactor duplication out**, delete the old copies in the same change. Two "temporary" copies are tech debt the moment they land.

---

## 8. Architecture Rules (where code goes)

- **New job board / ATS** → new package under `internal/provider/<name>/`. Copy the *pattern* of an existing provider (e.g. `greenhouse`, `lever`), not its code. Register it in the single registry/discovery point the engine uses — nowhere else.
- **New notification channel** → implement `notifier.Notifier` in `internal/notifier/`, wire it in `notifier.FromConfig`.
- **New CLI utility** → `cmd/<tool>/main.go`. Keep it thin: flags + calling into `internal/`. Real logic never lives in `cmd/` or root `main.go`.
- **Shared text/location logic** → `internal/textutil` / `internal/geo`. **Persistence** → `internal/store` or `internal/companies` only. UI never opens the DB; providers never touch the UI.
- Respect the existing layering: `ui → engine → provider/store → textutil/geo`. No upward imports, no import cycles.
- **Changing this layering is an architecture decision.** If your task seems to require it, stop and flag it (§16) — don't quietly establish a new pattern.

---

## 9. Anti-Patterns — Forbidden

Concrete "never do this" list. Violations get rejected in review, no exceptions:

| ❌ Forbidden | ✅ Instead |
|---|---|
| God objects, functions > ~80 lines, files > ~500 lines | Split by responsibility (§6-S) |
| Mutable package-level vars, hidden singletons | Inject via constructors (§6-D) |
| `init()` doing I/O, network, or spawning goroutines | Explicit constructors (`New`, `Open`) |
| `panic` in library code | Return `error`; `os.Exit` only in `main`/`cmd` after a clear message |
| `fmt.Print*` / `log` to stdout/stderr inside `internal/` | Return errors, emit events (§11) — the TUI owns the screen |
| New `http.Client` per request, or requests with no timeout | One shared client per provider, explicit timeout (§10) |
| SQL built with `fmt.Sprintf` / string concatenation | Parameterized queries only |
| Bare `any`, reflection, single-use generics | Concrete types (§5) |
| `time.Now()` scattered through business logic | Inject a clock for testability (§5) |
| Unbounded goroutine fan-out (one per job/item) | Bounded worker pool / semaphore (§10) |
| Blocking send/select without `ctx.Done()` | Respect cancellation everywhere |
| Copy-pasted code, parallel implementations | Extract/extend shared code (§7) |
| `TODO` / `FIXME` / stubbed bodies committed | Complete code only (§2) |
| Catching an error and continuing silently | Handle, wrap, or explicitly justify in a comment |

---

## 10. Networking, Concurrency & Performance

This app is a scraper/auto-applier — these rules are core, not optional.

### Networking
- **One shared `*http.Client` per provider**, created once (constructor), with an explicit `Timeout`. Never `http.Get`/`http.DefaultClient` (no timeout = hanging forever).
- **Every request carries the caller's `ctx`** (`http.NewRequestWithContext`). Cancellation must actually abort in-flight work.
- **Retry only transient failures** (network errors, 429, 5xx) with exponential backoff + jitter, bounded to a small max attempts; honor `Retry-After`. Never retry 4xx (except 429) — it will fail again.
- **Provider isolation:** one provider failing, timing out, or returning garbage must never abort the engine run — record the error, skip, continue with the rest.
- **Politeness is a feature:** respect configured delays/rate limits. Getting the user rate-limited or banned is a product failure, not a performance win.

### Concurrency
- Any change touching goroutines/channels/shared state must pass `go test -race ./...`.
- No shared mutable state across goroutines without explicit synchronization (mutex) or clear channel ownership. Document who owns what.
- **Every goroutine has an owner and an exit path.** Know what stops it and when. No fire-and-forget leaks.
- Bounded fan-out: worker pool or semaphore for N concurrent items — never one unbounded goroutine per job posting.
- Channel discipline: the *sender* closes channels; receivers never close; don't send after close.
- Prefer `sync.WaitGroup` / `errgroup`-style patterns over hand-rolled bookkeeping.

### Performance
- **Measure first.** No optimization without a benchmark or pprof evidence. Clarity wins until a path is proven hot (§3 priority 5).
- Avoid obvious waste regardless: compiling regexes per call (compile once at package level), string concatenation in loops (`strings.Builder`), unbounded in-memory result accumulation for streaming data.

---

## 11. Logging & Observability

- **`internal/` packages never write to stdout/stderr.** In TUI mode the Bubble Tea program owns the screen — a single stray `fmt.Println` corrupts the UI. This is forbidden (§9), not discouraged.
- Library code communicates by **returning errors** and **emitting events** through the existing patterns (engine `ResultCh`, `notifier.Event`). If a package genuinely needs diagnostics, accept an injected logger/verbose callback as a dependency (§6-D) — never grab the global one.
- Root `main.go` and `cmd/` tools own the terminal in their mode and may print.
- Gate noisy detail behind the existing `Verbose` flag; keep default output clean and actionable.
- **Never log secrets or personal data**: no tokens, API keys, webhook URLs, resume content, or message payloads. Log identifiers (job ID, URL, provider name, counts), not content.

---

## 12. Config, Data & Compatibility

`config.json`, the SQLite DBs, and session files are **the user's data**. Breaking them on upgrade is a severity-1 bug.

- **Config schema:** additive only. New fields use `omitempty` with sane zero-value defaults so old configs keep working. Never rename/remove a key or change a value's meaning without a migration path.
- **SQLite schema:** changes go through versioned migrations in `store` — never destructive, never "delete and recreate" user history. Test migrations with a real file DB in `t.TempDir()`.
- **CLI flags:** additive; changing flag behavior updates `helpText` in `main.go` in the same change. Bump the `version` const on user-visible changes.
- **Dependencies:** run `go mod tidy` after adding/removing imports. No committed `replace` directives. New deps need justification (§2 rule 9).
- **Idempotency of operations:** engine runs must be safely re-runnable — applying twice to the same job, double-recording history, or double-sending a notification are all bugs (§14).

---

## 13. Testing

- Every new exported behavior gets a test in `*_test.go` next to the code (house style: table-driven tests — see `internal/geo/*_test.go`, `internal/engine/region_boards_test.go`).
- Table-driven pattern: `tests := []struct{ name string; in …; want … }{…}` with `t.Run(tt.name, …)`.
- Failure messages must say input, got, want: `t.Errorf("Parse(%q) = %v; want %v", tt.in, got, tt.want)`.
- Tests must be hermetic: no network, no real `~/.nexus` writes, no wall-clock dependence — use `t.TempDir()`, file-based SQLite in temp dirs, fake `Notifier`/HTTP implementations, and injected clocks (§5).
- **Test failure paths, not just happy paths**: malformed JSON, provider timeout, DB closed, missing config, cancellation mid-run.
- **No `time.Sleep` in tests** — synchronize on channels, injected clocks, or `Eventually`-style polling with timeout. Sleeps make suites slow and flaky.
- Keep tests fast (seconds, not minutes). A slow suite stops being run.
- Add or update tests for **every** behavior change, even if not asked. A change that breaks existing tests is not finished until they're green.

---

## 14. Security & Domain Safety (this app acts on the user's behalf — be strict)

### Security & privacy
- **Never hardcode secrets.** API keys/tokens/webhooks come from `config.Config` (`provider_keys`, `anthropic_key`, `telegram_bot_token`, …). Never print, log, or embed their values (§11).
- Never commit `~/.nexus` content, resumes, `applications.db`, or anything containing personal data. Check `.gitignore` before adding data files.
- Validate/sanitize all external input: scraped HTML, JSON from job boards, config values. No string-built SQL — parameterized queries only.
- Browser automation (playwright) must use the existing session/stealth helpers — do not roll parallel browser-launch code.

### Domain invariants (never violate)
- **Idempotency:** never apply to the same job twice. Check `store` before every apply; make every retry safe; multi-step DB writes use transactions.
- **Consent & rate limits are untouchable:** `ApplyConsent`, `MaxAppsPerRun/Day`, `ApplyDelaySec` checks must stay intact in any code path that submits applications. Never add a flag, flag-combination, or code path that bypasses them.
- **Graceful degradation:** `AIAssist` off or the LLM unreachable → core search/apply still works. A provider down or returning garbage → skip it, record, continue (§10). Missing optional config → that feature disables itself, never the whole app.
- **Dry-run honesty:** `--dry-run` must guarantee zero submissions — no "harmless" network POSTs in dry-run paths.

---

## 15. Git & PRs

- Small, focused commits. Conventional-style messages: `feat(provider): add workable board`, `fix(engine): respect daily cap`, `refactor(textutil): dedupe title normalizers`.
- Don't commit build artifacts (`nexus.exe`, `test-apply`, …), data, or credentials.
- Before committing: `gofmt -l .` clean, `go vet ./...` clean, `go build ./... && go test ./...` green.

---

## 16. Agent Workflow (how to approach any task here)

1. **Read** this file + the package docs of the area you'll touch.
2. **Search** for existing implementations you can reuse or must extend (§7).
3. **Plan** the smallest change consistent with the architecture (§8) and SOLID (§6). State assumptions explicitly.
4. **Implement** completely — no stubs, no placeholders, matching existing style.
5. **Test**: add/update tests; run the full verification suite (§4).
6. **Report**: summarize what changed, where, verification output, and any assumptions made or improvements spotted (but not implemented — §2 rule 4).

### STOP and ask before:
- Deleting data, files, tables, or user history
- Changing config schema, DB schema, or their meaning (§12)
- Touching consent, rate-limit, or apply-submission logic (§14)
- Adding a dependency or changing the architecture/layering (§8)
- Renaming or moving exported APIs used elsewhere
- Any ambiguous decision whose wrong outcome is expensive

For everything else: make the reasonable call, state the assumption in your report, and proceed. Don't pepper the user with questions about reversible details.

---

## 17. Definition of Done (check every box before finishing)

- [ ] Did exactly what was asked — no speculative features, no unrequested files (§2 rule 4)
- [ ] No duplicated logic introduced; existing helpers reused (searched first)
- [ ] New code lives in the right package per §8 and follows SOLID per §6
- [ ] Nothing from the forbidden list (§9)
- [ ] `gofmt` clean, `go vet` clean, `go build ./...` passes
- [ ] `go test ./...` passes; new behavior covered by table-driven tests incl. failure paths
- [ ] Concurrency change → `go test -race ./...` passes
- [ ] Network code: shared client, explicit timeout, ctx propagated, bounded retries
- [ ] No stdout/stderr writes from `internal/` packages (§11)
- [ ] Config/DB changes additive or migrated; user's existing data keeps working (§12)
- [ ] Idempotency, consent, and rate-limit invariants intact (§14)
- [ ] No secrets, personal data, or artifacts committed; config-driven values only
- [ ] Exported symbols documented; package docs updated if behavior changed
- [ ] AGENTS.md / `.clinerules` updated if commands, layout, or conventions changed (§18)

---

## 18. Maintaining This File

- This is a **living document**. If your change alters commands, layout, conventions, or architecture, update `AGENTS.md` (and the `.clinerules` summary) **in the same change** — stale rules are worse than no rules.
- Changes to this file are rare and deliberate. Keep it dense, imperative, and example-backed. Every rule must be **enforceable** — no vague advice like "write good code".
- If a rule repeatedly conflicts with getting real work done, that's a signal the rule is wrong — propose changing the rule explicitly rather than quietly ignoring it.







