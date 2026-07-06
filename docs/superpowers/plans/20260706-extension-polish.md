# WXT Extension Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]` tracking.

**Goal:** Popup gains quick-tag-after-save, a recent-saves list, a save-current-tab keyboard shortcut, and refined states. Spec: `docs/superpowers/specs/20260706-extension-polish-design.md`.

**Architecture:** All in `apps/extension` (WXT MV3, React, tokens from @openmind/ui). Extend `lib/save.ts` (item-returning save + patchUserTags + recentItems), rework `Popup.tsx`, add a `commands` manifest entry + background listener. No server change, no deploy.

## Global Constraints

- Client-side only; consumes existing API: `POST /api/items` (201 → Item w/ id), `PATCH /api/items/{id} {userTags}`, `GET /api/items?limit=5`, `GET /api/auth/check`. All via the configured `{instanceUrl}` + Bearer token from `browser.storage.local` (existing `getSettings`). Token never logged.
- Dependency-free WXT + React (as today). Tokens-only styling from @openmind/ui. No banner comments. `tsc --noEmit` + `pnpm --filter extension build` + `build:firefox` green. Commit per task.
- PATCH sends the FULL user-tags list (server full-replaces). Full-replace matches web TagEditor.

---

### Task 1: lib + popup + keyboard shortcut

**Files:** `apps/extension/lib/save.ts`, `apps/extension/entrypoints/popup/Popup.tsx`, `apps/extension/entrypoints/background.ts`, `apps/extension/wxt.config.ts`

**Interfaces:**
- `lib/save.ts`: change `saveItem(body): Promise<{ok, status, item?: Item}>` (parse 201 JSON into `item`; define a minimal `Item` type {id,url,title?,summary?,status,userTags?}). Add `patchUserTags(id: string, userTags: string[]): Promise<{ok, status}>` (PATCH `{instanceUrl}/api/items/{id}`). Add `recentItems(limit: number): Promise<{ok, status, items: Item[]}>` (GET `{instanceUrl}/api/items?limit=`). All use getSettings + Bearer; network error → status 0; never log token.
- `background.ts`: add `browser.commands.onCommand.addListener` for `"save-page"` → query active tab → `saveItem({url})` → flashBadge ✓/! (reuse existing flashBadge). Keep existing context-menu handlers.
- `wxt.config.ts`: add `commands: { "save-page": { suggested_key: { default: "Ctrl+Shift+S", mac: "Command+Shift+S" }, description: "Save current page to Openmind" } }` to the manifest.

- [ ] Step 1: extend `lib/save.ts` (item-returning save, patchUserTags, recentItems, Item type). 
- [ ] Step 2: rework `Popup.tsx`:
  - On mount: `getSettings()` — if no token/instance → render a "Set up Openmind" prompt + button (`browser.runtime.openOptionsPage()`), no save UI.
  - Else: current-tab title/url + a Save button. On save → `saveItem({url})`: on `ok` show the saved title + a quick-tag row (input + chips; add → `patchUserTags(item.id, list)`, remove → PATCH filtered list; inline error, don't lose the save); on `status===401` → "Token rejected — open options"; on `status===0` → "Instance unreachable"; other → generic error.
  - A "Recently saved" section: on mount `recentItems(5)` → list title-or-host + a small state caption (status pending → "enriching…" cobalt, else the domain); each row opens `{instanceUrl}/item/{id}` via `browser.tabs.create`. Loading + empty states.
  - Compact ~360px, tokens-only, cobalt Save/links, mono meta, tag chips like the web.
- [ ] Step 3: background command listener + manifest `commands`.
- [ ] Step 4: `pnpm --filter extension build && pnpm --filter extension build:firefox && pnpm --filter extension lint` green; confirm `.output/chrome-mv3/manifest.json` has the `commands` entry + action/popup + existing permissions. Commit `feat(extension): quick-tag after save, recent saves, save-page shortcut`.

---

### Task 2: Driven e2e + docs

**Files:** `apps/extension/README.md`

- [ ] Step 1: run a local instance for testing — `OPENMIND_TOKEN=devtoken AI_PROVIDER=noop docker compose up -d --build api web` (db up; wait for /login 200). Build the extension (`pnpm --filter extension build`).
- [ ] Step 2: driven e2e with Playwright MCP (ToolSearch the browser_* tools). Load the unpacked extension is non-trivial headless; a pragmatic verification: since the popup is just a web page (`.output/chrome-mv3/popup.html`) that talks to `{instanceUrl}/api/*`, drive the popup's underlying logic by (a) confirming the built `manifest.json` contains `commands.save-page` + `action.default_popup` + permissions, and (b) exercising the exact API calls the popup makes against the live local instance via curl with the dev token: `POST /api/items {url}` → 201 with id; `PATCH /api/items/{id} {userTags:["from-extension"]}` → 200 + item carries the tag; `GET /api/items?limit=5` → returns recent incl. the new one. Record outputs. (Full in-browser popup driving needs a persistent context with the unpacked extension — if the MCP supports `--load-extension` via browser_run_code_unsafe or a launch arg, drive the popup and screenshot it to .superpowers/sdd/shot-ext-popup.png; otherwise document the manual load-unpacked steps and rely on the API-shape verification + build.) Stop api/web after.
- [ ] Step 3: `apps/extension/README.md` — document the new popup features (quick-tag, recent saves), the `Ctrl/Command+Shift+S` shortcut (+ how to rebind at chrome://extensions/shortcuts), and reaffirm load-unpacked + options setup. `TODO.md`: extension polish → Done (dated, evidence); note mobile app as the next slice (user chose both).
- [ ] Step 4: commit `feat(extension): e2e evidence + docs`. No server deploy (extension is loaded unpacked by the user); note in the report that the user should reload the unpacked extension to pick up the changes.
