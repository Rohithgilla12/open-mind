# Openmind Dock (macOS)

A menu-bar companion for Openmind: save what you're looking at and search your
mind from anywhere, without opening the web app. Tauri v2, macOS-only for now.

## Hotkeys

- **⌘⇧S — Quick Save.** Grabs the frontmost browser tab (URL + title) and saves
  it instantly. No window — a notification confirms. Supported browsers:
  Safari, Chrome, Brave, Edge, Arc. Firefox exposes no AppleScript tab access —
  use the panel instead.
- **⌘⇧O — Quick Find.** A Spotlight-style panel: type to search your library
  (↑/↓ to pick, Enter opens the item in your browser), or paste/type a URL and
  Enter saves it. ⌘Enter saves whatever you typed as a URL or note. Esc hides.

If a hotkey is taken by another app you'll get a notification and the tray menu
still covers both actions (Open panel / Save current tab / Settings / Quit).

## Install

- **Download**: grab the latest `.dmg` from [GitHub Releases](https://github.com/Rohithgilla12/open-mind/releases) (signed + notarized — opens without warnings). Also on the [CrabNebula download page](https://web.crabnebula.cloud/rohith-gilla/openmind/releases), or grab the [latest dmg directly](https://cdn.crabnebula.app/download/rohith-gilla/openmind/latest/platform/dmg-aarch64).
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
- The app is unsigned — build from source. Signing/notarisation + DMG
  distribution is a follow-up.
- Not yet: pinned Desk bar, Windows/Linux, launch-at-login, hotkey rebinding.
