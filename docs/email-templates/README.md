# Supabase Auth Email Templates (Confirm signup)

Branded HTML + plain-text fallback for the Supabase Auth **"Confirm signup"**
(and "Confirm signup link") email templates, matching the Nexus violet identity
(`#6D28D9` / `#7C3AED`). These are pasted into the Supabase Dashboard — they are
not rendered by this repo.

Files:
- `supabase-confirm-signup.html` — the styled HTML body (paste into the
  **Body (HTML)** tab).
- `supabase-confirm-signup.txt` — the plain-text fallback (paste into the
  **Body (text)** tab).

---

## ✋ You must configure Custom SMTP first

Supabase locks template editing behind a **Custom SMTP** setup, and the default
(built-in) mailer on the free plan is restricted:

- **Only sends to your project team's email addresses** — every other recipient
  fails with *"Error sending confirmation email"*.
- **Rate-limited to ~2 messages/hour.**

Configure **Authentication → Emails → SMTP Settings → Enable Custom SMTP**, e.g.
with Gmail:

| Field | Value |
|---|---|
| Host | `smtp.gmail.com` |
| Port | `465` (SSL) |
| Username | full Gmail address (`you@gmail.com`) |
| Password | a 16-char **app password** (Google Account → Security → App passwords; requires 2-Step Verification) |
| Sender name | `Nexus` |
| Sender email | the *same* Gmail address as Username |

Deliverability error codes: `535` = bad credentials (app password, must match
the Username exactly); TLS/timeouts = wrong port (`465` SSL); `421/454` =
Google throttling (common on brand-new Gmail accounts).

---

## Where to paste (30 seconds)

1. **Supabase Dashboard → Authentication → Emails → Templates**.
2. Make sure **Authentication → Providers → Email → Confirm email** is **ON**.
3. Edit **Confirm signup**:
   - Subject → one of the options below.
   - **Body (HTML)** tab → `supabase-confirm-signup.html`.
   - **Body (text)** tab → `supabase-confirm-signup.txt`.
4. Set **Authentication → URL Configuration → Site URL** to the real dashboard
   URL (feeds `{{ .SiteURL }}`).
5. **Save** → **Send test email**.

The **Confirm signup link** (magic-link) template is separate — reuse the same
HTML there; the button href stays `{{ .ConfirmationURL }}`.

---

## Subject line options

| Subject | Notes |
|---|---|
| **Confirm your Nexus account** | recommended — clean, short, no spammy words |
| **⚡ Activate your Nexus account** | boldest; the ⚡ can marginally affect deliverability scoring |
| **One quick click to activate your Nexus account** | friendly/frictionless angle |
| **Verify your email for Nexus** | most generic of the four, still fine |

---

## Template variables used

Supabase templates are **Go templates**. This template uses a deliberately small
surface (all safe to paste — they are valid on every GoTrue version):

- `{{ .ConfirmationURL }}` — the verification link (button + fallback link).
- `{{ .Email }}` — the address being confirmed (greeting + footer).
- `{{ .SiteURL }}` — the project's Site URL (footer link).

To greet by name instead, replace `Hi {{ .Email }}` with
`Hi {{ .Data.full_name }}` — only if the signup passes `full_name` in
`user_metadata`, otherwise it renders `<no value>`.

---

## Design decisions / gotchas

- **Table layout + inline CSS** — the only reliably-rendered layout across
  Gmail, Apple Mail, and Outlook.
- **Bulletproof button** — the `<!--[if mso]>` VML block gives Outlook a proper
  clickable button instead of a plain link.
- **Mobile** — the `<style>` block collapses the card to full width and makes
  the button block-level under 480px.
- **Dark mode** — `<meta name="color-scheme">` + a `prefers-color-scheme` rule
  that deepens only the page background; the card stays light so text contrast
  is preserved.
- **No images, no web fonts** — auth emails should stay image-light (spam
  filters) and font stacks degrade to system fonts.
