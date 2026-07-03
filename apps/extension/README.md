# openmind extension

WXT + React browser extension (thin capture client). Enrichment logic stays
server-side; this app only captures and displays.

## Scripts

- `pnpm --filter extension dev` — dev with HMR (Chrome)
- `pnpm --filter extension build` — production build → `.output/chrome-mv3`
- `pnpm --filter extension build:firefox` — Firefox build
- `pnpm --filter extension lint` — `tsc --noEmit`

## Features

- **Popup** (toolbar icon) — saves the active tab's URL to your instance. Shows
  saving / saved / error states; when no token is set or the instance returns
  401, it offers an "Open settings" button.
- **Context menus** —
  - _Save selection to Openmind_ (right-click on selected text): saves the text
    as a note, appending the page URL as a source line.
  - _Save image to Openmind_ (right-click on an image): saves the image URL.
  - Results flash the action badge (`✓` / `!`); failures also raise a
    notification.

## Loading unpacked

### Chrome / Edge / Brave

1. `pnpm --filter extension build` → produces `.output/chrome-mv3`.
2. Open `chrome://extensions`, enable **Developer mode** (top right).
3. Click **Load unpacked** and select `apps/extension/.output/chrome-mv3`.

### Firefox

1. `pnpm --filter extension build:firefox` → produces `.output/firefox-mv2`.
2. Open `about:debugging#/runtime/this-firefox`.
3. Click **Load Temporary Add-on…** and select any file inside
   `apps/extension/.output/firefox-mv2` (e.g. `manifest.json`). Temporary
   add-ons are removed on browser restart.

## Settings walkthrough

1. Open the extension's **Settings** page (Chrome: right-click the toolbar icon
   → _Options_; or via `chrome://extensions` → _Details_ → _Extension options_).
2. Enter your **Instance URL** (defaults to `https://openmind.gilla.fun`).
3. Paste your **access token**.
4. Click **Validate** to confirm the token, then **Save settings**.

Once a valid token is saved, the popup and context menus will save to your
instance.

## Notes

- Manifest requests broad `host_permissions` (`https://*/*`, `http://localhost/*`)
  for now. Narrow-origin **optional** host permissions (requesting access per
  instance origin at runtime) are deferred to a later task.
- Icons under `public/icon/` are placeholder solid-cobalt (`#2438FF`) squares.
