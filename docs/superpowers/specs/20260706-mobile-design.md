# Openmind Mobile (Expo) — Design

Date: 2026-07-06 · Status: Designed autonomously (user: mobile/extension → both; mobile second) · Builds `apps/mobile` (currently a placeholder)

## Goal

A phone app to capture into Openmind from anywhere — especially the **share sheet** (share a link/text from any app → saved). Plus a minimal library view and token login. Thin client (capture + display only, per CLAUDE.md); all enrichment stays server-side.

## Verification reality (explicit)

I can build + typecheck the app and screenshot its **web preview** (Expo runs react-native-web) here. I **cannot** run an iOS/Android simulator or the native **share extension** from here — that needs a device via Expo Go (for the app) or a custom dev build (for share-sheet, which can't run in Expo Go). Those parts are wired + configured + documented; the user runs them. This is the accepted trade-off from the direction choice.

## Architecture

**Standalone Expo app** (Expo SDK 52+, TypeScript, expo-router) with its **own package.json + lockfile — NOT added to the pnpm workspace.** Rationale: Expo/Metro's resolver clashes with pnpm's symlinked node_modules; keeping mobile isolated (its own node_modules) sidesteps a whole class of "won't bundle" failures and is far more likely to run on the user's device. Cost: the handful of design tokens (paper/ink/cobalt/… + font names) are **inlined** in a local `theme.ts` rather than importing `@openmind/ui`. The API contract is simple enough to hand-write a tiny client.

## Config & auth

- `expo-secure-store` holds `{ instanceUrl, token }` (never in async-storage plaintext). A Settings screen sets/validates them (`GET {instanceUrl}/api/auth/check` with Bearer → 200/401). App scheme `openmind://`.
- API client (`lib/api.ts`): `fetch({instanceUrl}/api/...)` with `Authorization: Bearer <token>`. Endpoints used: `POST /api/items {url|note}` (201 → item), `GET /api/items?limit=` (list), `GET /api/auth/check`. (These are the web app's proxy routes, which honour the Bearer header — confirmed working for the extension.)

## Screens (expo-router)

- **Library** (home tab): `GET /api/items`, FlatList of cards (title/host + a small enriching/enriched state + card-type hint), pull-to-refresh, tap → open `{instanceUrl}/item/{id}` in the system browser (expo-web-browser). Empty + unconfigured states.
- **Capture** (tab or FAB): a text field (auto-detect URL vs note like the web quick-add) + Save → `POST /api/items` → toast/confirmation + it appears in Library. 
- **Settings**: instance URL + token inputs, Validate, save to secure-store; a sign-out that clears them. Unconfigured app routes here first.
- Warm-editorial feel adapted to native: paper background, ink text, cobalt accents, a serif (Newsreader via expo-font, optional) for titles; otherwise the platform font — keep it clean, not a pixel port.

## Share sheet (the marquee feature)

- Use **`expo-share-intent`** (community config plugin; handles iOS Share Extension + Android `SEND`/`SEND_MULTIPLE` intents without manual native code). `app.json` config plugin + scheme. On a shared URL/text, the app opens (or foregrounds) to **Capture pre-filled** with the shared content; the user taps Save (or an auto-save option). 
- Dependency justification: a share extension genuinely cannot be done in pure JS/Expo Go; `expo-share-intent` is the standard, maintained way and avoids ejecting. This is the one new dependency, scoped to mobile only (not the Go/self-host core).
- **Requires a dev build** (`expo run:ios` / `expo run:android` or EAS) — the share extension is not available in Expo Go. Documented clearly.

## Testing

- `tsc --noEmit` (or `expo-doctor` + tsc) green; `npx expo export --platform web` builds; a web-preview screenshot of the Library + Capture + Settings screens via Playwright against `expo start --web`.
- The share-intent path: config present in `app.json` (plugin + intent filters), documented device-run steps. Not runnable here.
- No server change, no deploy.

## Out of scope

Offline capture queue, push notifications, in-app search/lenses/desk/drift (capture + library only for v1 — the phone is a capture surface), image capture from the camera roll (URL/note first; share-sheet covers shared images' URLs), biometric lock, app-store submission.

## Execution

Subagent-driven. Because Expo installs are heavy and device-bound, the plan front-loads a clean scaffold + honest web-preview verification and treats share-intent as wired-and-documented. `apps/mobile/README.md` gets full run instructions (Expo Go for app; dev build for share-sheet). Not added to pnpm-workspace; own install.
