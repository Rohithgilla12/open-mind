# WXT Extension Polish — Design

Date: 2026-07-06 · Status: Designed autonomously (user: mobile/extension → both, extension first) · Polishes the existing `apps/extension` (WXT MV3)

## Goal

Turn the extension's popup from a single "Save page" button into a genuinely useful capture surface: save → immediately tag, see recent saves, and save the current tab via a keyboard shortcut. All client-side against the user's configured instance (no server change, no deploy — the extension calls the already-deployed API).

## Scope

1. **Save returns the item.** `saveItem` currently returns `{ok, status}`. Change it to return the created item (`{ok, status, item?}` — parse the 201 JSON `Item`, which has `id`). This unblocks post-save tagging.
2. **Quick-tag after save** (popup). After a successful page save, show the saved item's title + a small tag input; adding a tag calls `PATCH {instanceUrl}/api/items/{id} {userTags:[...]}` (new `patchUserTags(id, tags)` in `lib/save.ts`). Chips with × to remove; the popup holds the working list and PATCHes the full list each change (matches the server's full-replace semantics). Skippable — the save already succeeded.
3. **Recent saves** (popup). A "Recently saved" section listing the last 5 items via `GET {instanceUrl}/api/items?limit=5` (new `recentItems(limit)`), each showing title (or host) + a subtle enriching/enriched state. Clicking one opens `{instanceUrl}/item/{id}` in a new tab. Gives the popup ongoing utility and confirms saves land + enrich.
4. **Keyboard shortcut.** Add a WXT `commands` manifest entry `save-page` (suggested `Ctrl+Shift+S` / `Command+Shift+S`) + a background `commands.onCommand` listener that saves the active tab's URL (badge flash ✓/!), no popup needed.
5. **Refined states.** Popup: if unconfigured (no token/instance) → a clear "Set up in options" prompt with a button to open options (not a save failure). Distinguish reachable-but-401 ("token rejected — check options") from network-unreachable ("instance unreachable"). Success shows the saved title + the quick-tag affordance rather than just a checkmark.

## Non-goals

No new server endpoints (all consume existing `/api/items` POST/GET, `/api/items/{id}` PATCH, `/api/auth/check`). No display/browse beyond the recent-5 list. No offline queue. No changes to the context menus (save selection/image) beyond their existing behaviour. Keep it dependency-free WXT + React as today.

## Design

Match the app's warm-editorial feel within the popup's small canvas, using `@openmind/ui` tokens (the extension already imports them): paper background, ink text, cobalt for the primary Save + links, JetBrains-Mono-style small meta for the recent-item state, tag chips like the web. Keep the popup compact (~360px wide); the recent list scrolls if needed. Respect the existing options page.

## Testing

- `tsc --noEmit` (extension lint) + `pnpm --filter extension build` (chrome) + `build:firefox` green; manifest includes the `commands` entry + existing permissions.
- Manual/driven e2e: run a local instance (`docker compose up` api+web, token), build the extension, load `.output/chrome-mv3` unpacked, and drive the popup with Playwright (options → set instance+token → validate; popup → save the current tab → see it in recent saves → add a tag → verify via the API that the item got the user tag). The keyboard command is registered (verify in the built manifest); actual key-chord firing is browser-runtime and documented for manual check.

## Execution

Subagent-driven, 2 tasks: (1) lib + popup + background + manifest implementation; (2) build + driven e2e + docs (README update). No server deploy — update `apps/extension/README.md` with the new features + shortcut. The extension points at whatever instance the user configures (their `openmind.gilla.fun` or localhost).
