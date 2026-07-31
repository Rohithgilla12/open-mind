# Chrome Web Store submission — Openmind extension

Copy for the Chrome Web Store developer dashboard, plus the reasoning behind
each privacy answer. Field names match the dashboard as of 2026-07; confirm
against the live form, Google reshuffles it.

Upload artefact: `apps/extension/.output/extension-1.0.0-chrome.zip`
(`pnpm --filter extension zip`).

---

## Store listing

**Name** (43 chars; manifest `name` caps at 75)

```
Openmind — the self-hosted commonplace book
```

The qualifier is deliberate — the name sweep (`docs/20260715-openmind-name-sweep.md`)
found a robotics "OpenMind" with pending USPTO application 99278181. Do not ship
the bare word.

**Short description / summary** (132 char limit)

```
Save pages, quotes, and images to your own Openmind instance in one click, then tag them without leaving the page.
```

**Category:** Productivity → Workflow & Planning
**Language:** English (UK)

**Detailed description**

```
Openmind is an open-source, self-hostable commonplace book. Save anything you
come across — articles, quotes, images, notes — and it gets organised for you,
so you can find it later by a fragment: a keyword, a colour, a half-remembered
vibe.

This extension is the capture end of that. It talks only to the Openmind
instance you point it at.

WHAT IT DOES

• Save the current page from the toolbar icon, or with Ctrl+Shift+S
  (Command+Shift+S on macOS).
• Right-click selected text to save it as a quote, keeping the source page as
  an attribution line.
• Right-click any image to save it.
• Tag a save straight from the popup, without leaving the page.
• See your five most recent saves, so you can confirm one landed.

YOUR SERVER, YOUR DATA

Openmind is AGPL-3.0 licensed and designed to be self-hosted: Postgres plus a
single Go binary, brought up with `docker compose up`. This extension sends your
saves to the instance URL you enter in its settings — nowhere else. There is no
Openmind analytics service, no telemetry, and no account with us.

If you would rather not run a server, the settings page offers a one-click
option to use the maintainer's hosted instance. That is opt-in and clearly
labelled; a fresh install has no instance configured at all.

Source code, self-hosting guide, and issue tracker:
https://github.com/Rohithgilla12/open-mind
```

**Support URL:** `https://github.com/Rohithgilla12/open-mind/issues`
**Homepage URL:** `https://github.com/Rohithgilla12/open-mind`

---

## Privacy practices

**Single purpose**

```
Openmind saves web content the user explicitly chooses — the current page, a
text selection, or an image — to a self-hosted Openmind instance whose URL the
user configures, and lets the user tag what they just saved.
```

**Permission justifications**

| Permission | Justification |
|---|---|
| `storage` | Stores the user's instance URL and API token locally so they aren't re-entered on every save. Never transmitted anywhere except as the `Authorization` header to that instance. |
| `activeTab` | Reads the URL and title of the tab the user is on, only when they invoke a save (toolbar click or keyboard shortcut). No background tab reading. |
| `contextMenus` | Adds the two right-click entries, "Save selection to Openmind" and "Save image to Openmind", which are the primary way quotes and images get captured. |
| `notifications` | Shows a single notification when a background save fails (bad token, unreachable server) — otherwise a context-menu save could fail silently with no UI open to report it. |
| Optional host permissions (`https://*/*`, `http://*/*`) | Requested **per origin at runtime**, never up front. The extension needs to `fetch` the one Openmind instance the user configures, and that address is unknown until they type it, so the pattern must be broad while the grant stays narrow — exactly one `scheme://host/*` at a time. `http://` is included because self-hosters commonly run on a LAN address or `localhost` without TLS. Switching instances revokes the previous grant. See `apps/extension/lib/permissions.ts`. |

**Remote code:** No. All JavaScript ships in the package; nothing is `eval`'d or
fetched at runtime.

**Data collected** — declare these two:

- **Website content** — the URL and title of pages the user saves, text they
  select to save, and image URLs. Transmitted only to the user's configured
  instance.
- **Authentication information** — the API token, stored locally, sent as a
  bearer token to that same instance.

Do **not** tick: PII, health, financial, location, personal communications.

> Judgment call worth knowing about: reviewers occasionally read "saved page
> URLs" as the **Web history** category. It isn't — the extension never enumerates
> browsing history and only ever sees a page the user deliberately saved. If a
> reviewer pushes back, that is the argument; the `activeTab`-only design (no
> `tabs` permission) is the evidence.

**Certifications** — all three are truthfully checkable: no sale of data to
third parties, no use beyond the single purpose, no creditworthiness/lending use.

**Privacy policy URL:** `https://openmind.gilla.fun/privacy`

⚠️ Gate before submitting: that page renders a contact address from the
`CONTACT_EMAIL` env var and falls back to neutral placeholder text when unset.
Set it on the production web service (docker-compose `api`/`web` env — see the
"env needs compose passthrough" trap) and load the page yourself. A privacy
policy with no reachable contact is a routine rejection.

---

## Assets

Icons — **done**. Derived from the app's own sidebar mark (a cobalt card with
three fading text lines), source SVGs in `docs/store/`:

| File | Use |
|---|---|
| `apps/extension/public/icon/{16,32,48,128}.png` | Manifest icons (16 is pixel-tuned separately, not scaled) |
| `docs/store/chrome-store-icon-128.png` | Store icon — 96×96 artwork centred in a 128×128 transparent canvas |
| `docs/store/mark.svg`, `mark-16.svg` | Vector sources |

Screenshots — **done**, all exactly 1280×800 in `docs/store/screenshots/`:

| File | Shows |
|---|---|
| `extension-popup.png` | The popup over a live tab: title, URL, Save page, recent saves |
| `extension-options.png` | Settings — instance URL, token, connect-by-code |
| `the-mind.png` | Where saves land — the library |
| `search-colour.png` | Colour search |
| `reader.png` | Reader with highlights |
| `desk.png` / `drift.png` | Spares if a listing slot allows more |

Regenerate any of these with [`tools/screenshots`](../tools/screenshots). They
are captured from a throwaway stack seeded with mock data — never from a real
library.

## Pre-submit checklist

- [ ] `CONTACT_EMAIL` set in production, `/privacy` and `/terms` verified live
      (still the one hard blocker — the privacy URL above must resolve with a
      working contact address)
- [ ] Load `.output/chrome-mv3` unpacked and walk the full first-run path by
      hand: no instance → options → grant prompt → validate → save → tag.
      The capture harness bypasses the permission prompt, so that prompt is the
      one flow no automated run has exercised.
- [ ] Confirm the install-time permission warning mentions no host access
- [ ] Register the developer account (one-time US$5) and complete verification
- [ ] Decide on the extension UI font gap (below) before or after first submit
- [ ] Optional promo tiles: 440×280 small, 1400×560 marquee

## Known cosmetic gap

The popup and options page render in the **system font**, not the brand faces.
`packages/ui` tokens reference `var(--font-instrument-sans)` etc., which the web
app defines via `next/font`; the extension has no equivalent, so the variables
resolve to nothing and the stack falls through to `system-ui`. Visible in
`extension-options.png`.

Not a submission blocker — plenty of extensions use system fonts, and a small
popup arguably reads better that way. To fix, self-host the faces in the
extension (e.g. `@fontsource/instrument-sans` + `@fontsource/jetbrains-mono`
plus an `@font-face` block in the popup/options entrypoints). Do not load them
from Google Fonts at runtime: remote CSS/font fetches in an extension are a
review risk and break the offline case.
