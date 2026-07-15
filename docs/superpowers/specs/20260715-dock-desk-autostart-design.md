# Dock v1.1 — Desk/Recents empty state + Launch at login

Date: 2026-07-15 · Status: Approved · Builds on `docs/superpowers/specs/20260706-dock-design.md`

## Goal

Make the dock feel like a daily companion when the panel is idle: show **Desk pins** and **recent saves** without typing, and let the user opt into **launch at login**. No tray pin submenu, no Win/Linux, no hotkey rebinding in this slice.

## Product behaviour

### Empty panel (query blank, search mode)

When the panel is open, settings are configured, and the input is empty:

1. Fetch in parallel (Bearer, same timeouts as search):
   - `GET /api/desk` → pinned items (newest-pinned first)
   - `GET /api/items?limit=8` → recent library items
2. Render two sections in the panel body:
   - **Desk** — up to 5 pins (if any). Section header “Desk”.
   - **Recent** — up to 8 items. If an item is already shown under Desk, omit it from Recent (dedupe by id). Section header “Recent”.
3. Each row matches the existing search-result row (title-or-host, caption with host · cardType · tags). ↑/↓ / Enter / click open `{instanceUrl}/item/{id}` in the browser, then hide the panel — same as search hits.
4. Loading: short “Loading…” in the body (do not block the input).
5. Errors: same copy as search (`Token rejected — open Settings` / `Instance unreachable` / status code). Failures on one endpoint do not blank the other — show whichever list succeeded.
6. Empty library + empty Desk: keep the calm hint (`Type to search · ⌘⇧S saves the front tab`).
7. Refresh: refetch when the panel gains focus or when returning from Settings to main (stale pins after a web pin are acceptable until next focus).

Typing anything leaves this home view and restores current search / save-url behaviour unchanged.

### Settings — Launch at login

- Toggle below the existing connect UI (when already connected or always visible once settings load): **Launch at login**.
- Backed by `tauri-plugin-autostart` with `MacosLauncher::LaunchAgent` (Accessory / menu-bar apps should not use Login Item APIs that expect a Dock icon).
- On Settings mount: read `isEnabled()` and reflect the toggle; on flip: `enable()` / `disable()`. Errors show a one-line status (“Couldn’t update login item”) without crashing.
- Default remains **off** until the user opts in (no surprise autostart on first install).

### Out of scope

Tray Desk submenu, Drift from the panel, colour swatches, clipboard capture, offline queue, Win/Linux tab-grab, hotkey rebinding, DMG/notarisation.

## Architecture

### Web ingress

Add `apps/web/app/api/desk/route.ts` — `GET` pass-through to `{API_URL}/desk`, honouring Bearer / cookie like `/api/search` and `/api/feeds`. No OpenAPI or Go change (`GET /desk` already exists).

`GET /api/items?limit=` already exists — reuse for recents.

### Dock client

- `listDesk(settings?)` → `Item[]`
- `listRecent(limit, settings?)` → `Item[]`
- Reuse existing `Item` type and timed fetch helper.

### Panel UI

- Home view component (or inline branch) when `mode === "search" && !query.trim()`: Desk + Recent lists sharing row chrome with search results.
- Selection model: single flat index across Desk then Recent rows for ↑/↓.

### Autostart

- Cargo: `tauri-plugin-autostart` (desktop targets).
- JS: `@tauri-apps/plugin-autostart`.
- Capabilities: allow autostart enable/disable/is-enabled.
- Register plugin in `lib.rs` setup with `MacosLauncher::LaunchAgent`.

## Testing / verification

- Vitest: desk/recent client parsing + error mapping; home-list dedupe helper (Desk ids excluded from Recent).
- `tsc` / dock build green.
- Manual: empty panel shows pins + recents against a connected instance; toggle launch-at-login and confirm a LaunchAgent appears/disappears; Esc/⌘⇧O behaviour unchanged.

## Success criteria

- Opening ⌘⇧O with an empty input shows Desk and/or Recent without typing.
- Launch at login can be turned on/off from Settings and survives app restart.
- Search, Quick Save, and Settings connect flows remain unbroken.
