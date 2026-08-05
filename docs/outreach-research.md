# Research: Better Cold Outreach & Higher Job-Selection Rate

Goal: raise the % of applications that turn into interviews by (a) improving cold
outreach to companies and (b) being more selective about *which* jobs we apply to.

Note on sources: live web fetches were blocked in the dev environment, so the
benchmarks below are drawn from widely published studies (Backlinko 12M-email
outreach study, Woodpecker follow-up research, Yesware reply-rate data, Jobvite
recruiting benchmarks). Treat exact numbers as directional, not gospel.

---

## 1. What the research says about cold outreach

### Benchmarks (so we know what "good" looks like)
| Metric | Typical | Good | Great |
|---|---|---|---|
| Cold apply → interview | ~1–3% | 5% | 10%+ (with referral) |
| Cold email reply rate | 1–5% | 8–12% | 15–25% |
| LinkedIn note acceptance | ~20–30% | 40%+ | — |
| Referred candidate → hire | ~5–10x more likely than cold applicant | | |

### Findings that matter for JobPilot
1. **Personalization is the #1 lever.** Backlinko's 12M-email study found
   personalized *bodies* improved response ~33%, and personalized subject lines
   help meaningfully. Mail-merge `{{company}}` is the bare minimum — recruiters
   can smell a template. Real personalization = a specific, true detail about
   the company/team (product, blog post, funding, tech stack from the JD).
2. **Follow-ups generate most of the replies.** Woodpecker's data: campaigns
   with 4–7 touches get ~3x the reply rate of 1–3 touches. Yesware: ~70% of
   unanswered chains stop after email #1, yet follow-ups keep converting.
   **Single-shot outreach leaves the majority of replies on the table.**
3. **Multi-threading works.** Contacting *several* people at one org (Backlinko:
   ~+93% response) beats one email to one recruiter. Ideal set: hiring manager,
   recruiter/talent, and a peer engineer (referral source).
4. **Keep it short.** 50–125 words, 3–4 short paragraphs max. One idea, one ask.
5. **One small, specific CTA.** "Open to a 15-min chat this week?" or
   "Who's the right person to talk to?" — never "please review my application".
6. **No attachments in email #1.** Links instead (LinkedIn, portfolio, PDF on a
   URL). Attachments hurt deliverability and get ignored.
7. **Timing.** Tue–Thu, recipient's local morning (~8–10am). Avoid Mon morning
   and Fri afternoon. Send within 24–48h of the job posting going live — early
   applicants get disproportionately more interviews.
8. **Deliverability discipline.** Verify addresses before sending (bounces <3%),
   keep daily volume low (our 10/day cap is good), plain text (we already do
   this), consistent From name/address. A burned sender address = everything
   dies.
9. **Subject lines.** Short + specific beats clever: role name + company, or a
   genuine question. Avoid spam-trigger words ("opportunity", "hire me",
   exclamation marks, ALL CAPS).
10. **Warm the contact on LinkedIn first.** View profile / like a post / comment
    1–2 days before the email or connection note — acceptance and reply rates
    go up. Connection notes must fit ~300 chars: one line of context, one line
    of ask.


---

## 2. Gap analysis — current JobPilot implementation vs. best practice

| Area | What JobPilot does today | Gap | Evidence in code |
|---|---|---|---|
| Message content | Static `{{var}}` mail-merge templates | No per-company/per-contact AI personalization, even though Ollama + Anthropic/OpenAI infra exists | `internal/outreach/draft.go`, `internal/localllm/generate.go` |
| Default templates | "I recently applied... would welcome any guidance" | Generic, no value prop, no specific hook, weak CTA | `ready.go: DefaultEmailBody` |
| Sequence | One email / one LinkedIn touch, then done | No follow-ups — biggest single loss of replies | `models.go`: statuses stop at `sent` |
| Contact targeting | Single "best" Hunter contact (recruiter-ish) | No multi-threading (HM + recruiter + peer engineer); LinkedIn flow just opens a people-search page | `finder.go: hunterDomainSearch`, `queue.go: BuildLinkedInQueue` |
| Domain guess | Assumes `company.com` from ATS slug | Misses `.io/.ai/.dev/.co` and legal-name mismatches → Hunter searches wrong domain | `finder.go: GuessDomain` |
| Send timing | Sends whenever the queue is run | No best-time scheduling (Tue–Thu AM recipient tz) | `email_send.go` |
| Reply tracking | None — `sent` is terminal | Can't measure reply rate, can't stop sequences on reply | `models.go: Status` enum |
| Outcome funnel | `applied/skipped/failed` only | No interview/rejected/ghosted statuses → no feedback loop on selection % | `internal/store/models.go` |
| Fit gating | LLM fit score (0–100) computed **after** insert, in background | Score never gates anything — low-fit jobs still get applied to | `engine.go: processJob` + `scoreJob`; no `MinFitScore` in config |
| Resume tailoring | Same resume file for every apply | No per-JD keyword mirroring (ATS pass-rate lever) — AI cover letter exists but the resume itself is static | `resume/improve.go` exists, not wired per-apply |
| Freshness | Jobs processed in scrape order | No "posted <48h ago" prioritization | engine ordering |
| Analytics | None | No funnel (sent → replied → interview), no per-template/per-provider stats | — |

### The three highest-ROI gaps (do these first)
1. **No follow-up sequence** → add 2–3 follow-ups, +3 / +7 / +14 days, auto-stop on reply.
2. **Fit score doesn't gate applying** → skip (or queue-only) below a threshold, e.g. 65–70.
3. **Template-only outreach** → use the existing LLM to write each email from the
   JD + resume highlights + contact title.

---

## 3. Improvement plan (prioritized)

### P0 — biggest wins, low effort (all use infra we already have)

**1. Follow-up sequences**
- New statuses: `followup_1_due`, `followup_2_due`, `replied`, `bounced`.
- Schedule: FU1 at +3 days, FU2 at +7, FU3 (breakup) at +14. Each follow-up is a
  *new short message*, not "bumping this" — add value (a relevant link, a one-liner
  about their product, a portfolio piece).
- Auto-stop the whole thread on reply detection or manual "replied" mark.
- Keep daily caps counting follow-ups too (10/day total).

**2. Fit-score gating**
- Add `MinFitScore` (default 65) to config. Score **before** deciding:
  below threshold → status `skipped-low-fit` (kept in History, visible, no apply).
  Above → apply + outreach.
- This alone typically 2–3x's interview rate per application: 20 tailored
  applications to high-fit roles beat 100 spray-and-pray.

**3. AI-personalized outreach drafts**
- New `outreach.Personalize(ctx, ai, job, resumeText, contact)` using the same
  `AIOptions`/Ollama plumbing as `resume.ScoreJobFit`.
- Prompt inputs: full JD, company name, contact name/title, 2–3 quantified
  resume highlights. Output JSON: `{subject, body}` ≤120 words, one CTA.
- Fall back to the template when AI is off — zero regression risk.
- Show drafts in the Outreach hub for quick edit before send (confirm mode already
  supports this).

**4. Better default templates** (research-backed, drop-in for `ready.go`)

Subject: `{{role}} — quick question`
```
Hi {{contact_name}},

Saw {{company}} is hiring for {{role}} — I just applied, and one thing in the
JD stood out: [specific requirement from the JD].

I've done exactly that: [1-line quantified achievement, e.g. "cut p95 latency
40% on a Go payments service at X"].

Worth a 15-minute chat? If you're not the right person, a pointer to who owns
this role would be hugely appreciated.

Thanks,
{{full_name}} · {{linkedin}}
```
Follow-up 1 (+3d): new angle, 2 lines — a relevant project/repo link.
Follow-up 2 (+7d): restate fit in one line + ask who's the right person.
Follow-up 3 (+14d, breakup): "Closing the loop — if timing's wrong, happy to
stay in touch." Breakup emails get surprisingly high replies.

LinkedIn note (≤300 chars):
```
Hi {{contact_name}} — applied to {{role}} at {{company}}. My background matches
the JD's core ask ([keyword]); would love to connect and hear what the team's
looking for. — {{full_name}}
```

### P1 — medium effort, high value

**5. Multi-contact targeting (multi-threading)**
- Return top 3 contacts from Hunter/OSINT, ranked: hiring manager (title match:
  "engineering manager", "head of", "director"), recruiter/talent, peer engineer.
- Email 1–2 of them; LinkedIn-note the third. Respect a per-company daily cap
  (e.g. 2) so we don't spam one org.

**6. Reply tracking + outcome funnel**
- Gmail IMAP poll (app password already configured) to detect replies from
  `ContactEmail` → mark `replied`, stop sequence, notify via Discord/Telegram.
- Add manual outcome statuses in History: `interview`, `rejected`, `ghosted`,
  `offer` — one keypress per row. This creates the dataset for everything below.

**7. Send-time scheduling**
- Queue sends land Tue–Thu, 8–10am. Approximate recipient tz from job location
  (we already have geo data). If unknown, default to the job's region.

### P2 — compounding improvements

**8. Trigger-event hooks**: `internal/enrich` already fetches pages — pull company
   blog/news/careers headline into the AI prompt ("saw you just launched X").
**9. A/B subjects**: alternate two subject styles; record which variant each item
   used; report reply rate per variant after ~50 sends.
**10. Funnel analytics tab**: applied → outreach sent → replied → interview →
    offer, sliced by provider, company size, fit-score band, template variant.
    This is how we actually learn what raises selection %.
**11. Per-JD resume variant** (with review): reorder/mirror JD keywords in the
    summary + skills sections using the AI pipeline; keep a "master" resume and
    save variants per application. Never invent facts — reordering emphasis only.


---

## 4. Raising "job selection %" — the full equation

Selection % = (fit of jobs picked) x (ATS pass rate) x (human screen rate) x (referral boost).

| Lever | Mechanism | JobPilot change |
|---|---|---|
| Pick better jobs | Only apply where fit ≥ threshold; deprioritize old postings (>2–3 weeks = often already in late rounds) | `MinFitScore` gate + posted-date sort |
| Pass the ATS | Mirror JD keywords (exact terms, e.g. "Kubernetes" not "k8s") in resume summary/skills; standard headings; no tables/columns in the PDF | Per-JD resume variant (P2 #11) |
| Pass the human screen | 6-second scan: top third of resume must show role title match + 2 quantified wins | Resume tab already scores strengths — surface "top-third" check |
| Referral boost | Referred candidates are several times more likely to be interviewed/hired | Outreach *before or same day as* applying, multi-threaded |
| Volume discipline | 10–15 high-quality applies/day beat 50 low-quality; protects sender reputation too | Fit gating enforces this naturally |
| Feedback loop | Double down on what converts: which titles, providers, company sizes, templates | Outcome statuses + funnel analytics (P1 #6, P2 #10) |

### Metrics to start tracking (so "selection %" is a number, not a vibe)
- **Apply → interview %** overall and per fit-score band (validates the LLM scorer).
- **Outreach reply %** per channel, per template variant, per contact type (HM vs recruiter vs engineer).
- **Interview % with outreach vs. without** — measures the referral lift directly.
- **Bounce %** — keep <3%; purge pattern-guessed addresses that bounce.
- Time-to-first-response distribution → tells us the optimal follow-up spacing for *our* market.

### Suggested build order
1. `MinFitScore` gate (engine) — half a day, immediate selectivity win.
2. Follow-up sequence engine + statuses — 1–2 days.
3. AI personalization of drafts (reuse `AIOptions` plumbing) — 1–2 days.
4. New default templates — minutes, ships with #3.
5. Outcome statuses in History + reply marking — 1 day.
6. Multi-contact resolution + per-company cap — 1 day.
7. Analytics tab — after 2+ weeks of data exists.
