# Openmind mobile

A thin Expo (SDK 57) client for [Openmind](../../README.md): capture links and notes
into your instance from your phone — especially via the **share sheet** (share any
link/text from another app → it lands in Capture, pre-filled) — plus a minimal
library view and token login. All enrichment stays server-side; the app only talks
to your instance's `/api/*` with a Bearer token.

Screens: **Library** (recent items), **Capture** (paste a URL / jot a note → Save),
**Settings** (instance URL + API token).

## Prerequisites

- Node 18+ and a running Openmind instance you can reach from the phone.
- An **API token** for that instance (the same `OPENMIND_TOKEN` you use for the
  browser extension — see the root `docs/self-hosting.md`).

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

Scan the QR code with **Expo Go** (iOS/Android). The Library, Capture, and Settings
screens all work in Expo Go.

On first launch nothing is configured, so you land on **Settings**:

1. Enter your **instance URL** (e.g. `https://openmind.example.com`).
2. Paste your **API token**.
3. Tap **Validate & save** — the app calls `GET /api/auth/check` and stores the
   token in the device keychain (`expo-secure-store`) on success.

Then Capture and Library become usable.

## Share sheet — requires a dev build

Share-sheet capture uses native code that **cannot run in Expo Go** — you need a
custom dev build:

```bash
cd apps/mobile
npx expo run:ios       # or: npx expo run:android
# (or build with EAS: npx eas build --profile development)
```

The two platforms behave differently by design:

- **iOS — saves inline, no app switch.** Sharing a link/text and picking
  **Openmind** opens a small **Share Extension** sheet *on top of* the app you're
  in ([`expo-share-extension`](https://github.com/MaxAst/expo-share-extension),
  root component `ShareExtension.tsx`). It shows the shared URL/text and a **Save**
  button, POSTs straight to your instance, and dismisses — you never leave the
  current app. `expo-share-intent`'s iOS half is disabled (`disableIOS: true`) so
  the two don't both register an extension.
- **Android — opens the app.** The `text/*` SEND intent
  ([`expo-share-intent`](https://github.com/achorein/expo-share-intent)) opens
  Openmind on **Capture**, pre-filled; tap **Save**. (An inline Android
  save-and-dismiss activity is a planned follow-up.)

### How the iOS extension reads your token

The Share Extension is a **separate process** and can't share React state with the
app, so it reads the instance URL + token from a **shared keychain access group**
(`group.fun.gilla.openmind`) written by `lib/settings.ts` (`accessGroup` option).
Both targets must carry a matching `keychain-access-groups` entitlement:

- The **main app** gets it from `ios.entitlements` in `app.json`.
- The **extension** gets it from the `extra.eas.build.experimental.ios.appExtensions`
  entitlements block — `expo-share-extension` merges those into the extension
  target on **EAS Build**.

> ⚠️ **Verify on a real build.** This share extension has only been type-checked
> and config-resolved on CI — it hasn't been compiled/run on a device here. Two
> things to confirm on your first `eas build` / `expo run:ios`:
> 1. **SDK compatibility.** `expo-share-extension`'s published compatibility table
>    currently tops out at Expo SDK 54; this app is on SDK 57. If the pinned
>    `^5.0.6` doesn't build against SDK 57, bump to whatever version (or the 6.x
>    line) lists SDK 57 support.
> 2. **Local prebuild keychain entitlement.** On a *local* `expo run:ios`,
>    `expo-share-extension` writes the extension's `.entitlements` file with only
>    the app group — not the keychain group. If the extension can't read the token
>    (shows "Connect this device…"), add `keychain-access-groups` to the
>    extension target in Xcode, or build via EAS where the `appExtensions`
>    entitlements block is honoured.

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
./node_modules/.bin/expo export --platform web   # web bundle builds
```
