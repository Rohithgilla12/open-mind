# Openmind Dock (macOS)

A menu-bar companion for Openmind: save what you're looking at and search your
mind from anywhere, without opening the web app. Tauri v2, macOS-only for now.

## Hotkeys

- **⌘⇧S — Quick Save.** Grabs the frontmost browser tab (URL + title) and saves
  it instantly. A notification confirms, and the panel pops up with a small
  **quick-tag strip**: type comma-separated tags and press Enter to file the
  save, or Esc (or just wait 5 seconds) to skip — the save has already
  happened either way. Supported browsers: Safari, Chrome, Brave, Edge, and Arc
  (supported since the first dock release), plus Chromium (bundle id
  confirmed against a real install), and Safari Technology Preview, Orion,
  Vivaldi, Opera, and Chrome Beta/Dev/Canary (bundle ids follow vendor
  convention and have **not been checked against a real install** — if one
  of these is your front app and the grab reports "Front app isn't a
  supported browser", please open an issue with the output of
  `osascript -e 'id of app "<Name>"'`).
  Firefox exposes no AppleScript tab access — use the panel instead.
- **⌘⇧O — Quick Find.** A Spotlight-style panel: type to search your library
  (↑/↓ to pick, Enter opens the item in your browser), or paste/type a URL and
  Enter saves it. ⌘Enter saves whatever you typed as a URL or note. Esc hides.

Both hotkeys are **rebindable**: Settings → Shortcuts — click a field, press
your combination (at least one modifier plus a key), Save. If a combination
is taken by the OS or another app you'll see an inline error and the old
binding stays active; **Reset to defaults** restores ⌘⇧S/⌘⇧O. Custom
bindings persist across restarts (stored in the app's config directory).

If a hotkey is taken by another app you'll get a notification and the tray menu
still covers both actions (Open panel / Save current tab / Settings / Quit).

## Install

- **Download**: grab the latest `.dmg` from [GitHub Releases](https://github.com/Rohithgilla12/open-mind/releases) (signed + notarised — and opens without warnings — when the maintainer's signing secrets are configured in CI; unsigned builds fall back with a Gatekeeper warning). Also on the [CrabNebula download page](https://web.crabnebula.cloud/rohith-gilla/openmind/releases), or grab the [latest dmg directly](https://cdn.crabnebula.app/download/rohith-gilla/openmind/latest/platform/dmg-aarch64).
- **Auto-updates**: installed docks check for new releases on every launch and update themselves — no reinstalling.

## Run / build

```bash
pnpm install                       # repo root — dock is a workspace member
pnpm --filter dock test            # vitest (api client, input modes)
cd apps/dock
pnpm exec tauri dev                # dev app (vite + cargo)
pnpm exec tauri build              # release .app bundle (unsigned)
```

Rust unit tests: `cd apps/dock/src-tauri && cargo test`.

## First-run setup

1. Launch the app — it lives in the menu bar (no Dock icon).
2. Tray → **Settings** (or ⌘⇧O): enter your instance URL
   (e.g. `https://openmind.example.com`) and the same token you use for the web
   login (`OPENMIND_TOKEN`), then **Validate & save**. Settings are stored in
   the **macOS Keychain**, not on disk.
3. First ⌘⇧S triggers macOS **Automation** consent prompts — one for System
   Events and one per browser ("openmind-dock wants to control Safari…").
   Approve them (System Settings → Privacy & Security → Automation if you
   dismissed the prompt).

## Notes

- The instance must be reachable over HTTPS; search goes through the web app's
  `/api/search` proxy (needs a server running the 2026-07-06 web build or later).
- **Offline saves are never lost.** A save that fails on a network error — or
  on a 5xx/429 from the instance — is queued to disk and retried: on launch,
  every 60 seconds, whenever the panel regains focus, and on demand from the
  tray ("Retry pending saves") or the panel strip ("Retry now"). Pending saves
  show in a strip at the top of the panel, where each can be discarded. A
  rejected token stops the queue rather than burning it, and the cap is 100
  captures (oldest dropped).
- **The panel is resizable** and remembers its size and position across
  restarts. A position saved on a display you have since disconnected is
  recentred rather than restored offscreen.
- The tray menu has a **Desk** submenu of your pinned items — refreshed on
  launch, after a save, and when the panel regains focus.
- Not yet: Windows/Linux tab grab.
