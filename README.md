# ⚡ JobPilot — Automated Job Applier

[![CI](https://github.com/manthan8219/nexus-job-assistant/actions/workflows/ci.yml/badge.svg)](https://github.com/manthan8219/nexus-job-assistant/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Version](https://img.shields.io/badge/version-0.1.0-6D28D9)](./main.go)

JobPilot is a Go CLI/TUI that automates the boring part of a job search: it searches
**38+ job boards**, scores jobs against your résumé, **applies for you** (with your
explicit consent and rate limits), and runs a full **recruiter outreach pipeline**
(find a contact → draft an email → send → follow up → detect replies) — all from
one terminal dashboard.

It works **fully offline-AI** (local Ollama) or with a remote LLM (Anthropic, OpenAI, Google, DeepSeek, Groq, Mistral, Together, OpenRouter, or xAI),
and keeps you safe: explicit apply consent, per-run/per-day caps, a delay between
applications, idempotent "never apply twice" checks, and a hard CAPTCHA stop that
hands control back to you.

---

## Table of contents

- [What it does](#what-it-does)
- [How it works (architecture)](#how-it-works-architecture)
- [Prerequisites](#prerequisites)
- [Install / build](#install--build)
- [Quick start](#quick-start)
- [CLI reference](#cli-reference)
- [The TUI](#the-tui)
- [Configuration](#configuration)
- [Job boards](#job-boards)
- [AI options](#ai-options)
- [Recruiter outreach](#recruiter-outreach)
- [Notifications](#notifications)
- [Data & privacy](#data--privacy)
- [User accounts & authentication](#user-accounts--authentication)
- [Safety & responsible use](#safety--responsible-use)
- [Project structure](#project-structure)
- [Development](#development)
- [Helper utilities (`cmd/`)](#helper-utilities-cmd)
- [Roadmap / limitations](#roadmap--limitations)
- [Disclaimer](#disclaimer)

---

## What it does

- **Search 38+ job boards** in one run — Greenhouse, Lever, Ashby, Workday,
  RemoteOK, Hacker News "Who's hiring", and many more (see [Job boards](#job-boards)).
- **AI fit-scoring** — scores each job against your résumé (0–100); jobs below your
  `min_fit_score` are recorded as *skipped*, not applied.
- **Auto-apply with consent** — fills Greenhouse/Lever/Workable/SmartRecruiters
  application forms via Playwright browser automation. Requires explicit consent;
  respects per-run and per-day caps and a configurable delay between submissions.
- **Never apply twice** — every application is checked against the local history
  store before submitting (idempotent).
- **Résumé tooling** — parse a PDF résumé, get an AI fit report, generate a
  tailored cover letter, and render a tailored PDF.
- **Recruiter outreach pipeline** — discover a recruiter/HM email (OSINT,
  Hunter.io, Apollo, GitHub, pattern-guessing), draft an email (template **or**
  AI-written + AI-reviewed), send via SMTP or Gmail, run a +3/+7/+14-day
  follow-up sequence, and detect replies from the inbox.
- **Notifications** — Discord webhook and Telegram bot, fanned out in parallel;
  a single channel failure never blocks the others.
- **Local or remote AI** — run everything on a local Ollama model, or use an
  Anthropic, OpenAI, Google, DeepSeek, Groq, Mistral, Together, OpenRouter, or
  xAI API key. AI off or unreachable → core search/apply still works.
- **TUI dashboard** — one screen to configure everything, watch runs, browse
  history, manage companies, run outreach, and view diagnostics.

## How it works (architecture)

JobPilot follows a strict layering (see [`AGENTS.md`](./AGENTS.md) for the full
constitution):

```
ui  →  engine  →  provider / store  →  textutil / geo
```

- **`main.go`** — flag parsing, wires TUI vs engine mode. No business logic.
- **`internal/engine`** — the apply engine: orchestrates providers, rate limits,
  fit-gate, results, notifications.
- **`internal/provider/<name>`** — one package per job board (plugin pattern).
  Add a board = drop in a new package, zero changes to existing providers.
- **`internal/store`** — SQLite persistence (applications, sessions, contacts,
  outreach log). Idempotency checks live here.
- **`internal/outreach`** — the full outreach pipeline (find → draft → review →
  send → follow-up → reply).
- **`internal/resume`** — résumé parsing, AI fit-scoring, cover-letter & PDF.
- **`internal/ui`** — the Bubble Tea dashboard.
- **`internal/agentx` / `internal/localllm`** — generic LLM-agent builder and
  local (Ollama) model management.

## Prerequisites

- **Go 1.26+** (module `github.com/manthan8219/nexus-job-assistant`).
- **SQLite** — bundled via `modernc.org/sqlite` (pure Go, no CGO, no system install).
- **Playwright browsers** (only for the auto-apply feature) — install once with:
  ```powershell
  go run ./cmd/pwinstall
  ```
- **Ollama** (optional) — to use a local LLM for fit-scoring, cover letters, and
  outreach drafting. JobPilot detects your hardware and recommends a model.
- **Remote LLM API key** (optional) — Anthropic, OpenAI, Google Gemini, DeepSeek,
  Groq, Mistral, Together AI, OpenRouter, or xAI, if you prefer API mode.

## Install / build

```powershell
git clone https://github.com/manthan8219/nexus-job-assistant.git
cd nexus-job-assistant
go build -o nexus .
./nexus            # launches the TUI
```

Or run directly without building a binary:

```powershell
go run .                  # TUI
go run . --run            # engine once
```

## Quick start

1. **Launch the dashboard:** `./nexus`
2. **Fill the Config tab** — personal info, target job titles, locations, work type,
   min salary, résumé path. Everything saves automatically to `~/.nexus/config.json`.
3. **Turn on Apply consent** in the Config → Apply Safety section. This is required
   before any application is ever submitted.
4. (Optional) **Enable AI Assist** and pick local (Ollama) or API mode.
5. (Optional) **Add a Discord webhook or Telegram bot** for notifications.
6. **Dry-run first** to see what matches without applying:
   ```powershell
   ./nexus --run --dry-run
   ```
7. **Apply for real** (respects caps + delay):
   ```powershell
   ./nexus --run --limit 5
   ```

## CLI reference

```text
USAGE:
  nexus [flags]

MODES:
  (no flags)        Launch the interactive TUI dashboard
  --run             Run the apply engine once and exit
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

OUTPUT FLAGS:
  --verbose         Print detailed logs including skipped jobs
  --test-notify     Send a test notification to all configured channels and exit
  --version         Print version and exit
```

### Examples

```powershell
./nexus                              # Open the TUI
./nexus --run                        # Apply to jobs (max 10)
./nexus --run --limit 5              # Apply to max 5 jobs
./nexus --run --no-limit             # Apply to all found jobs
./nexus --run --dry-run              # See matching jobs without applying
./nexus --run --delay 15             # Wait at least 15s between applications
./nexus --run --provider greenhouse --limit 3
./nexus --run --verbose
./nexus --check-replies              # Inbox sweep + due follow-ups
./nexus --test-notify                # Verify Discord/Telegram wiring
```

## The TUI

The dashboard (`./nexus` with no flags) has 8 tabs:

| Tab | What it does |
|---|---|
| **Dashboard** | Run/stop the engine, dry-run, auto-apply, live stats |
| **Config** | All settings (personal, job prefs, AI, notifications, apply safety, outreach) |
| **Resume** | AI fit report for your résumé, cover-letter generation |
| **Jobs** | Application history with search, detail view, fit score, outcome funnel |
| **Companies** | Local employer DB; scraped-jobs detail; add a company |
| **Outreach** | Setup / Email / LinkedIn / Sent sub-tabs; build & fire the queue |
| **Contacts** | OSINT recruiter discovery + clipboard |
| **Logs** | Diagnostics + local disk/memory usage snapshot |

Navigation: `← →` / `tab` switch tabs, `1-8` jump, `enter` focus a tab, `esc` back
to tab mode, `ctrl+z` background, `ctrl+c` quit.

## Configuration

The primary way to configure JobPilot is the **Config tab in the TUI** — it writes
`~/.nexus/config.json` automatically. You can also edit the JSON directly. Key
fields:

- **Personal** — `first_name`, `last_name`, `email`, `phone`, `linkedin_id`,
  `resume_path`, `city`, `years_of_experience`.
- **Job preferences** — `target_job_titles`, `job_intent` (free-text), `work_type`
  (Remote/Onsite/Hybrid), `target_locations`, `currency`, `min_salary`.
- **Apply safety (required for auto-apply)** — `apply_consent` +
  `apply_consent_at`, `max_apps_per_run`, `max_apps_per_day`, `apply_delay_sec`,
  `min_fit_score`, `company_blocklist`, `work_auth`, `notice_period_days`,
  `office_days_per_week`, `cover_letter_mode`, `cover_letter_text`.
- **AI** — `ai_assist`, `ai_provider` (`local` | `api`), `anthropic_key` /
  `openai_key` (API mode), `local_llm_url` + `local_llm_model` (local mode).
- **Notifications** — `notify_channels`, `discord_webhook_url`,
  `telegram_bot_token`, `telegram_chat_id`.
- **Outreach** — `outreach_consent`, `outreach_max_email`, `outreach_max_li`,
  `outreach_li_mode`, `outreach_li_cookie`, `outreach_auto_queue`,
  `outreach_ai_compose`, `outreach_ai_review`, `outreach_gen_model`,
  `outreach_check_model`, `outreach_min_score`, `outreach_max_retries`,
  `outreach_smtp_verify`, `outreach_follow_ups_off`, `reply_lookback_days`.
- **Email send** — `gmail_app_password` (SMTP) **or** Gmail OAuth
  (`gmail_oauth_client_id` / `_secret` / `_refresh_token`).
- **Contact discovery keys** — `hunter_key` (Hunter.io), `apollo_key` (Apollo.io).
- **Provider keys** — `provider_keys` map for boards that require auth.

## Job boards

JobPilot ships 38+ provider packages under `internal/provider/`. Each is an isolated
plugin — one board failing never aborts the run.

<details>
<summary><b>Full list (click to expand)</b></summary>

`arbeitnow`, `ashby`, `bamboohr`, `breezy`, `careerscraper`, `cutshort`,
`echojobs`, `fourday`, `getonbrd`, `greenhouse`, `hackernews`, `himalayas`,
`hirist`, `instahyre`, `jobicy`, `jobspresso`, `jobvite`, `justjoin`, `lever`,
`linkedin`, `nodesk`, `nofluffjobs`, `personio`, `pinpoint`, `recruitee`,
`remoteok`, `remotive`, `smartrecruiters`, `teamtailor`, `thehub`, `themuse`,
`weworkremotely`, `workable`, `workatastartup`, `workday`, `workingnomads`, `wttj`

</details>

A local employer DB (`~/.nexus/companies.db`) is seeded from `data/companies.json`
and merged per-region by the engine. Add a board by following
[`internal/provider/TEMPLATE.md`](./internal/provider/TEMPLATE.md).

## AI options

| Mode | Setup | Used for |
|---|---|---|
| **Off** | Nothing | Core search + apply still works (graceful degradation) |
| **Local** | Ollama running locally | Fit-scoring, cover letters, answers, outreach drafts/reviews — fully private |
| **API** | One API key — Anthropic, OpenAI, Google, DeepSeek, Groq, Mistral, Together, OpenRouter, or xAI | Same features via a remote model (the first key set wins: Anthropic → OpenAI → Google → DeepSeek → Groq → Mistral → Together → OpenRouter → xAI) |

JobPilot uses the [`eino`](https://github.com/cloudwego/eino) agent framework. The
local mode detects your hardware (CPU/RAM/GPU) and recommends a fitting model.
Every remote provider (except Anthropic) is reached through its
OpenAI-compatible chat-completions endpoint — no extra SDKs to install.

## Recruiter outreach

The outreach pipeline (Config → Outreach tab, or driven by the engine):

1. **Find** a recruiter/hiring-manager contact — OSINT + stored application history,
   Hunter.io, Apollo, GitHub, or email-pattern guessing (with optional SMTP verify).
2. **Draft** an email — templated by default, or AI-written when
   `outreach_ai_compose` is on.
3. **Review** — a second LLM scores the draft (0–100); below `outreach_min_score`
   it's regenerated, up to `outreach_max_retries`.
4. **Send** via SMTP (Gmail app password) or the Gmail API (OAuth).
5. **Follow up** — automatic +3 / +7 / +14-day sequence (disable with
   `outreach_follow_ups_off`); stops the moment a reply is detected.
6. **Detect replies** — `--check-replies` scans the inbox, stops answered
   sequences, records rejections, alerts you on human replies, and fires due
   follow-ups.

Run modes: `confirm` (ask y/n each), `queue` (manual step-through), `auto`
(run the whole queue with delays).

## Notifications

Discord webhooks and Telegram bots are supported behind a single `Notifier`
interface and fanned out in parallel by `MultiNotifier`. A channel failure is
recorded, never fatal. Events: `job_applied`, `job_failed`, `captcha`, `error`,
`daily_summary`, `weekly_summary`, `run_started`, `run_complete`, `reply_received`,
and `custom`.

Add a channel by implementing `Notifier` and registering it — the UI and config
pick it up automatically.

## Data & privacy

Everything lives under `~/.nexus/`:

| Path | Purpose |
|---|---|
| `~/.nexus/config.json` | Your settings (mode `0600`) |
| `~/.nexus/applications.db` | Application history (idempotency source) |
| `~/.nexus/companies.db` | Local employer DB |
| `~/.nexus/sessions/` | Browser-automation sessions |
| `~/.nexus/resumes/` | Generated/tailored PDF résumés |

- **No telemetry.** JobPilot does not phone home.
- **Secrets stay local** — API keys, tokens, and webhook URLs are read from
  `config.json` and never logged. `config.json`, `*.db`, and `.env` are gitignored.
- **Never commit** `~/.nexus` contents or résumés.

## User accounts & authentication

JobPilot authenticates through an identity provider (Supabase Auth) instead of
storing passwords. When auth is enabled, the web dashboard sits behind a login
wall (email + password or magic link) and every API request carries the user's
session token; the backend verifies the token and routes each user to their own
data island — **no user can see another user's config, applications, contacts,
outreach, or runs**.

- **Backend env vars**
  - `NEXUS_SUPABASE_JWT_SECRET` — the Supabase project's classic JWT secret
    (HS256). Setting it enables auth: every `/api/*` route returns 401 without
    a valid token, and each signed-in user gets their own data under
    `NEXUS_HOME/users/<userID>/` (config, applications, contacts, companies,
    plus per-user engine runs and mission streams). Unset → legacy
    single-user mode, unchanged.
  - `NEXUS_SUPABASE_JWKS_URL` — **alternative to the JWT secret** for newer
    Supabase projects that sign access tokens with asymmetric keys (ES256
    P-256). Set it to the project's Discovery URL, e.g.
    `https://<project>.supabase.co/auth/v1/.well-known/jwks.json`. The key set
    is fetched lazily and cached (auto-refreshes on key rotation). If both are
    set, the JWT secret wins.
  - `NEXUS_SUPABASE_URL` *(optional)* — enables the issuer-claim check.
  - `NEXUS_SUPABASE_JWT_AUD` *(optional)* — audience; defaults to
    `authenticated`.
  - `NEXUS_ADMIN_EMAILS` *(optional)* — comma-separated emails whose first
    login claims the legacy single-user data (copied once, never deleted).
  - `NEXUS_ALLOWED_ORIGINS` *(optional)* — comma-separated CORS allow-list for
    the web dashboard; unset allows any origin (local development).
- **Frontend env vars** (`terminal-job-ui`)
  - `VITE_SUPABASE_URL` + `VITE_SUPABASE_ANON_KEY` — when both are set the
    dashboard shows the login wall (auth enabled); leave unset for local
    auth-disabled use.
- **TUI / CLI never require login** — they read the local data directory
  directly, exactly as before.
- `/health` and `/api/auth/status` stay public so hosts/load-balancers and the
  frontend can probe liveness and auth state.

## Safety & responsible use

This tool acts **on your behalf**, so the guardrails are strict (see
[`AGENTS.md` §14](./AGENTS.md)):

- **Explicit consent required** — `apply_consent` must be true before any
  application is submitted. There is no flag or code path that bypasses this.
- **Rate limits are untouchable** — `max_apps_per_run`, `max_apps_per_day`, and
  `apply_delay_sec` are enforced on every apply path.
- **Idempotent** — every job is checked against the history store before applying;
  you can never apply to the same job twice.
- **Dry-run is honest** — `--dry-run` guarantees zero submissions: no "harmless"
  POSTs in dry-run paths.
- **CAPTCHA = hard stop** — JobPilot never attempts to solve or evade a CAPTCHA or
  anti-bot challenge. It halts the automated flow and surfaces it for you to
  complete manually.
- **AI answer grounding** — any AI-generated answer containing a number, date,
  or figure is checked against your résumé/profile facts before use, so a
  hallucinated salary or experience figure is never submitted.
- **Provider isolation** — one board timing out or returning garbage is skipped
  and recorded; the run continues with the rest.
- **Politeness** — shared HTTP client with explicit timeouts, bounded retries with
  backoff, `Retry-After` honored.

## Project structure

```
main.go                 # entry point: flags → TUI or engine (no business logic)
cmd/<tool>/             # small standalone utilities
internal/engine         # apply engine: providers, rate limits, fit-gate, results
internal/provider/<name>  # one package per job board (plugin pattern)
internal/store          # SQLite persistence (applications, sessions, contacts)
internal/companies      # local employer DB
internal/config         # ~/.nexus/config.json load/save
internal/notifier       # notification channels behind a Notifier interface
internal/scraper        # career-page scraping
internal/enrich         # re-fetch a job description for re-analysis
internal/outreach       # recruiter outreach pipeline (find→draft→send→follow-up→reply)
internal/osint          # recruiter contact discovery
internal/resume         # résumé parsing, AI fit-scoring, cover letters, PDF
internal/workcontext    # user project/work-history store (enriches tailoring)
internal/agentx         # generic typed LLM-agent builder (eino)
internal/localllm       # Ollama integration: hardware detection, model catalog
internal/ui             # Bubble Tea dashboard
internal/textutil       # shared text helpers (check here before writing new ones)
internal/geo            # location/country logic (check here before writing new ones)
internal/usage          # local disk/memory footprint snapshot
data/                   # seed data (companies.json, …)
scripts/                # verify.ps1 / verify.sh / coverage-floor / hook installer
tests/                  # black-box integration tests
.github/workflows/      # CI gate
AGENTS.md               # the engineering constitution — read first
```

## Development

```powershell
go build ./...                 # must pass before you finish
go vet ./...                   # must be clean
gofmt -l .                     # must output nothing
go test ./...                  # must pass
go test -race ./...            # for any concurrency change
./scripts/verify.ps1           # wraps the whole chain (+ coverage floor)
```

CI (`.github/workflows/ci.yml`) runs on every push/PR to `main`: `gofmt`, `vet`,
`build`, `test -race`, and a coverage floor on changed packages. A red check
blocks the merge.

**Contributing:** read [`AGENTS.md`](./AGENTS.md) in full first — it is the
constitution of this repo (golden rules, SOLID, DRY, architecture, testing,
safety invariants). Install the optional pre-push hook:

```powershell
./scripts/install-hook.ps1
```

## Helper utilities (`cmd/`)

| Tool | Purpose |
|---|---|
| `cmd/companies-seed` | Seed the local employer DB from `data/companies.json` |
| `cmd/pwinstall` | Install Playwright browsers (for auto-apply) |
| `cmd/test-scraper` | Try the career-page scraper |
| `cmd/test-apply` | Test a single provider's apply path |
| `cmd/test-crawl` | Crawl a career page |
| `cmd/test-osint` | Test OSINT contact discovery |
| `cmd/greenhouseapply` | Browser-apply utility for Greenhouse |
| `cmd/leverapply` | Browser-apply utility for Lever |

## Roadmap / limitations

- Auto-apply form-filling is implemented for a subset of ATSs (Greenhouse, Lever,
  Workable, SmartRecruiters, …); other boards currently support search-only.
- LinkedIn outreach uses a logged-in browser session (cookie-based), not an API.
- No CAPTCHA solving by design — this is a feature, not a gap.
- Windows-first development; cross-platform where possible.
- No license file yet — the project is currently not licensed for public reuse.

## Disclaimer

JobPilot automates applying to jobs and emailing recruiters **on your behalf**.
You are responsible for reviewing what it submits, respecting each employer's
and platform's Terms of Service, and using the rate limits and consent controls
responsibly. The tool is provided as-is, without warranty.

