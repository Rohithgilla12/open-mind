# Openmind mobile

A thin Expo (SDK 57) client for [Openmind](../../README.md): capture links, notes,
and photos into your instance from your phone — especially via the **share sheet**
(share any link/text/image from another app → it lands in Openmind) — plus a
minimal library view with search and token login. All enrichment stays
server-side; the app only talks to your instance's `/api/*` with a Bearer token.

Screens: **Library** (recent items + search), **Desk** (pinned items), **Feed**
(subscribed feeds), **Capture** (paste a URL / jot a note / choose or take a
photo → Save, with an offline queue for URL/note), **Settings** (instance URL +
API token).

## Prerequisites

- Node 18+ and a running Openmind instance you can reach from the phone.
- An **API token** for that instance, OR Clerk sign-in credentials (see
  [Mobile authentication](#mobile-authentication) below).

## Mobile authentication

The app supports two authentication methods:

### Clerk sign-in (recommended for cloud instances)

If your Openmind instance uses Clerk, users can sign in directly from the app with
**Continue with Google** (OAuth) or **email authentication code**. After successful
authentication, the app exchanges the Clerk JWT for a long-lived device API key
(prefixed `omk_`), stores it securely in the device keychain, and signs out of Clerk
— subsequent requests use the device key.

**Clerk setup (instance maintainer):**

1. Log into your Clerk dashboard (e.g. `clerk.openmind.gilla.fun` for the default
   cloud instance).
2. Create a new **Native** application: **Applications** → **+ Create application**,
   select **Native**.
3. Enable the **Google** social connection under **Social Connections**.
4. In the application's OAuth redirect settings, add: `openmind://`
5. Copy your **Publishable Key**.

**Defaults:** out of the box the app targets the hosted instance
(`https://openmind.gilla.fun`) and its Clerk, so a plain build has working
sign-in with no configuration. Both are overridable via `EXPO_PUBLIC_*`:

- `EXPO_PUBLIC_CLERK_PUBLISHABLE_KEY` — your Clerk publishable key. Defaults to
  the hosted instance's key (a publishable key is public and safe to ship). Set
  this to point in-app sign-in at your own Clerk.
- `EXPO_PUBLIC_INSTANCE_URL` — your instance URL; defaults to
  `https://openmind.gilla.fun`.

Expo automatically inlines `EXPO_PUBLIC_*` variables into the app during builds.

Self-hosters on an instance **without** Clerk should build with
`EXPO_PUBLIC_CLERK_PUBLISHABLE_KEY` set to their own key, or use manual
authentication (see below).

### Manual authentication (self-hosters and alternative instances)

For instances without Clerk or when deploying on a custom domain, authenticate by
entering an API token manually in the app's Settings screen. On first launch, you
land on **Settings**:

1. Enter your **instance URL** (e.g., `https://openmind.example.com`).
2. Paste your **API token** (the same `OPENMIND_TOKEN` used for the browser
   extension — see the root `docs/self-hosting.md`).
3. Tap **Validate & save** — the app calls `GET /api/auth/check` and stores the
   token in the device keychain (`expo-secure-store`) on success.

Then Capture and Library become usable.

## Install

```bash
cd apps/mobile
npm install
```

This app has its own lockfile and is **not** part of the pnpm workspace — always
install from inside `apps/mobile`. Use `./node_modules/.bin/expo install <lib>` to
add Expo-compatible libraries so versions match SDK 57.

## Run the app (Expo Go)

```bash
cd apps/mobile
npx expo start        # or: ./node_modules/.bin/expo start
```

Scan the QR code with **Expo Go** (iOS/Android). The Library, Desk, Feed, Capture,
and Settings screens all work in Expo Go.

On first launch nothing is configured, so you land on **Settings**:

1. Enter your **instance URL** (e.g. `https://openmind.example.com`).
2. Paste your **API token**.
3. Tap **Validate & save** — the app calls `GET /api/auth/check` and stores the
   token in the device keychain (`expo-secure-store`) on success.

Then Capture and Library become usable.

## Library search

The Library tab has a search field. An empty query lists recent items
(`GET /api/items`). Typing a query calls `GET /api/search?q=…&parse=true` (same
NL-parse behaviour as the web app) and shows optional “understood” chips when the
server returns them. Search requires a network connection — there is no local
full-text index of the library.

Long-press a card for **Pin to desk**, **Open original**, **Copy link**,
**Share**, or **Delete**. Lists use a TanStack Query cache (≈60s stale window) so
switching tabs or returning from an item detail does not flash a full reload.

## Desk (pins)

Pinned items live on the **Desk** tab (`GET /api/desk`). Pin or unpin from the
long-press sheet, the item detail top bar, or the web app — they stay in sync.

## Offline capture queue

Capture is optimistic about bad networks:

1. Save tries `POST /api/items` as usual.
2. On a network failure (status 0), URL saves first attempt a short recovery check
   (same URL recently created). If that fails — or for notes — the payload is
   written to a durable **AsyncStorage** queue (`openmind.captureQueue`).
3. The field clears and you see “Queued — will sync when you’re back online.”
4. The queue flushes automatically on app foreground, tab focus, and when NetInfo
   reports connectivity returning. Capture also shows “N waiting to sync” with a
   **Sync now** button. Library’s subtitle shows a queued count when non-zero.

URL entries already in the queue are deduped. Cap is 100 (oldest dropped).

Photo uploads (`POST /api/assets`) are **online-only** for now — a network failure
shows an error instead of queuing the blob.

> **Share extension caveat:** the offline queue covers **in-app Capture** (and
> Android share → Capture prefill for URL/text). The iOS inline share extension
> (`targets/share`) still does a live POST only — failed shares there are not
> queued yet.

## Share sheet — requires a dev build

Sharing is native code and **cannot run in Expo Go** — you need a custom dev build:

```bash
cd apps/mobile
npx expo run:ios       # or: npx expo run:android
# (or build with EAS: npx eas build --profile development)
```

The two platforms take different routes:

- **iOS — inline save, no app switch.** A native Swift share extension
  (`targets/share/`, built with
  [`@bacons/apple-targets`](https://github.com/EvanBacon/expo-apple-targets))
  POSTs shared URL/text to `POST {instanceUrl}/api/items`, or shared images
  (converted to JPEG) to `POST {instanceUrl}/api/assets`, and shows a small
  "Saved to Openmind" card over the host app. It reads `{ instanceUrl, token }`
  from the App Group (`group.fun.gilla.openmind`) shared UserDefaults, which
  the app mirrors from Settings — so **connect the app once before using the
  share sheet**. Activation rules (one URL, web pages, plain text, one image)
  live in `targets/share/Info.plist`. Swift edits don't need a new prebuild;
  config changes do (`npx expo prebuild --clean -p ios`).
- **Android — opens the app.** [`expo-share-intent`](https://github.com/achorein/expo-share-intent)
  handles `text/*` and `image/*` SEND intents (iOS side disabled in `app.json`);
  the app opens on **Capture** — URL/text is pre-filled for Save; images upload
  immediately via `POST /api/assets`. Offline URL/note saves from this path use
  the same Capture queue.

In-app Capture also has **Choose photo** / **Take photo** (photo library +
camera via `expo-image-picker`) so you can save images without leaving Openmind.

> Note: the token is mirrored into App Group UserDefaults (not the keychain) so
> the extension can authenticate. On-device it is sandboxed to this app group;
> treat a self-host token leak as revocable via a new token.

> Do not commit the generated `ios/` and `android/` folders; regenerate with
> `expo prebuild` / `expo run:*` before an EAS build.

## Web preview

`npx expo start --web` (or `npx expo export --platform web`) runs the app through
react-native-web for quick UI checks. The share-intent native module is absent on
web and no-ops there; `expo-secure-store` falls back to `localStorage`. The web
target is a preview surface only — ship the native app for real use.

## Verify

```bash
cd apps/mobile
./node_modules/.bin/tsc --noEmit           # types
./node_modules/.bin/expo config --json > /dev/null   # app config resolves
```
