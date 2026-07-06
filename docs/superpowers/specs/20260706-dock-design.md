# Openmind Dock (Tauri, macOS v1) — Design

Date: 2026-07-06 · Status: Approved (user: Quick Find + Quick Save, macOS-first; instant-save + panel hotkeys) · Milestone 4 "Everywhere", PRD §5.11 · Builds `apps/dock` (currently a placeholder)

## Goal

A small always-available desktop companion: save what you're looking at and search your mind from anywhere, without switching to the web app. v1 is **macOS-only** and ships the two hotkey experiences; the pinned Desk bar and Windows/Linux follow later.

## Product behaviour

- **⌘⇧S — Quick Save (no UI).** Grabs the frontmost browser tab's URL + title via AppleScript and `POST /api/items {url}` immediately; a native notification confirms ("Saved — <title>") or reports failure. No window, no confirmation step (capture is sacred; enrichment is async server-side). Supported browsers: Safari and the Chromium family (Chrome, Brave, Edge, Arc — all expose `URL of active tab`/`front document` scripting). Firefox exposes no AppleScript URL access → notification explains, suggests the panel. If the frontmost app isn't a supported browser → notification says so. First use triggers macOS's Automation consent prompt per browser — documented in the README.
- **⌘⇧O — Quick Find (Spotlight-style panel).** A frameless, transparent-cornered, always-on-top panel centred in the upper third of the screen; shows on hotkey (focused input), hides on Esc or blur. Behaviour by input:
  - Plain text → debounced (250ms) `GET /api/search?q=<text>&parse=true`; results list (title-or-host, card-type + tags caption, palette dots if present). ↑/↓ to select, **Enter opens** `{instanceUrl}/item/{id}` in the default browser. An "Understood as …" caption renders when the parse echo differs from the raw query.
  - Input matching `/^https?:\/\//i` → the list is replaced by a single "Save <host>" action; **Enter saves** (`POST /api/items {url}`).
  - **⌘Enter anywhere** → saves the raw input as URL (if URL-shaped) or note (`{note}`) — quick note capture.
  - Empty states: "Type to search your mind"; no results → "Nothing found". Errors: 401 → "Token rejected — open Settings"; network → "Instance unreachable".
- **Tray (menu-bar) icon** — the app is a background utility (`LSUIElement`, no Dock icon). Menu: Open panel · Save current tab · Settings · Quit.
- **Settings view** (inside the same panel, gear toggle): instance URL + token, Validate via `GET /api/auth/check`, sign-out. Persistence rule learned from the mobile bug: only a definitive 401 refuses to save; indeterminate results (unreachable/429/5xx) still persist with an honest "saved but unconfirmed" note.

## Architecture

- **Tauri v2** app in `apps/dock`, **inside the pnpm workspace** (like `apps/extension`; Vite has no Metro-style symlink problems). Frontend: Vite + React + TS, design tokens imported from `@openmind/ui` (no hardcoded colours), Instrument Sans/JetBrains Mono/Newsreader per the design system.
- **Single hidden window** toggled by the global shortcut; `decorations: false`, `alwaysOnTop: true`, `skipTaskbar: true`, `visible: false` at startup. Esc/blur hides (never quits). Tray keeps the app alive.
- **Rust side is thin** — one custom command plus plugins:
  - `grab_frontmost_tab() -> Result<{url, title, browser}, GrabError>`: runs `osascript` with a per-browser script selected by the frontmost app's bundle id (`NSWorkspace`/`frontmostApplication`), typed errors (`UnsupportedApp`, `NoTab`, `AutomationDenied`, `ScriptFailed`).
  - Plugins: `tauri-plugin-global-shortcut` (⌘⇧S, ⌘⇧O), `tauri-plugin-notification` (save toasts), `tauri-plugin-opener` (open results in default browser).
  - **Token storage: macOS Keychain** via the `keyring` crate behind two commands (`settings_get/settings_set/settings_clear`); instance URL stored alongside. The token never touches the frontend's localStorage and is never logged.
- **API client** (`src/lib/api.ts`, hand-written like extension/mobile): `checkToken`, `saveItem({url|note})`, `searchItems(q)` → Bearer to `{instanceUrl}/api/*`; network error → status 0. Thin client — no enrichment logic (CLAUDE.md).

## Server-side prerequisite (only web change)

The web ingress has **no `/api/search` proxy** (the web app searches in server components; discovered 2026-07-06). Add `apps/web/app/api/search/route.ts`: a `GET` pass-through to `{API_URL}/search` honouring the incoming Bearer header (same pattern as the items GET proxy), forwarding `q`, `color`, `parse` query params. No Go/API change; no contract change (the `/search` endpoint already exists in openapi.yaml — this is just the cookie/Bearer proxy layer catching up).

## Out of scope (v1) / follow-ups

Pinned Desk bar (tray dropdown), Windows/Linux (per-OS tab-grab), auto-launch at login, custom hotkey rebinding (defaults hardcoded), offline capture queue, Firefox tab-grab (no AppleScript surface — revisit via native messaging), app signing/notarisation + DMG distribution (build-from-source first, like the extension's load-unpacked).

## Testing / verification (all on this Mac — first fully-verifiable native slice)

- Unit: vitest for the API client (Bearer header, status-0 mapping, URL detection) and panel input-mode logic; `cargo test` for the bundle-id → script selection and osascript output parsing.
- Builds: `pnpm --filter dock build` (Vite) + `cargo build` (via `pnpm tauri build` debug) green; `tsc --noEmit` clean.
- Live e2e on this machine against production: launch the dev app; trigger ⌘⇧S with a real Chrome/Safari tab frontmost (item appears via `/api/items`); open the panel, search a known term, screenshot; Enter-open verified; settings validate against `openmind.gilla.fun`. Automation consent granted interactively if prompted (user may need to click once).
- Web proxy: curl `https://openmind.gilla.fun/api/search?q=…` with Bearer → 200 results after web redeploy.

## Execution

Subagent-driven. Task split: (1) web `/api/search` proxy + deploy check; (2) Tauri scaffold + settings/keychain + API client + panel UI (search/save modes) with unit tests; (3) Rust `grab_frontmost_tab` + global shortcuts + tray + notifications; (4) live e2e on this Mac + README + TODO. The dock joins pnpm-workspace + turbo (build/lint only; Rust build stays out of turbo).
