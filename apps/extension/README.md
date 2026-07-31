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
  - **Quick-tag after save** — once the page is saved the popup reveals a tag
    editor. Type a tag and press Enter (or blur the field) to add it; each tag
    shows as a chip with a remove (×) button. Edits are written straight through
    with `PATCH /api/items/{id}` (the full desired tag list is sent each time —
    the server replaces, it does not merge). Failures roll the chips back and
    surface an inline error.
  - **Recent saves** — the popup lists your five most recent items
    (`GET /api/items?limit=5`) so you can confirm the save landed and glance at
    what you last captured.
- **Save-page keyboard shortcut** — `Ctrl+Shift+S` (Windows/Linux) /
  `Command+Shift+S` (macOS) saves the current tab without opening the popup
  (badge flashes `✓` / `!`). Rebind it at `chrome://extensions/shortcuts` if it
  clashes with another extension or a site shortcut.
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

A fresh install has **no instance configured** — the popup sends you straight to
the options page.

1. Open the extension's **Settings** page (Chrome: right-click the toolbar icon
   → _Options_; or via `chrome://extensions` → _Details_ → _Extension options_).
2. Enter your **Instance URL** (e.g. `https://openmind.example.com`), or click
   _use the hosted instance_ to opt into the maintainer-run one.
3. Approve the permission prompt for that origin (see _Host permissions_ below).
4. Paste your **access token**, or redeem a **connect code** from a signed-in
   device.
5. Click **Validate** to confirm the token, then **Save settings**.

Once a valid token is saved, the popup and context menus will save to your
instance.

## Host permissions

The extension declares **no** up-front host access. Instead it asks for the one
origin you configure, at the moment you configure it:

- MV3 (Chrome) declares `optional_host_permissions`; MV2 (Firefox) puts the same
  patterns in `optional_permissions`. `wxt.config.ts` branches on
  `manifestVersion`.
- `lib/permissions.ts` narrows the saved instance URL to a single
  `scheme://host/*` pattern and requests it.
- Chrome only shows the prompt during a user gesture, so requests happen from
  clicks on the **options page**. The popup and the background service worker
  cannot prompt — the popup detects a missing grant and links to options; the
  keyboard shortcut and context menus surface a failure badge.
- Switching instances revokes the grant for the previous origin.

## Notes

- The extension talks to `{instanceUrl}/api/*` with an `Authorization: Bearer`
  header (no cookie). Point **Instance URL** at your web app origin (e.g.
  `http://localhost:3000`): the web `/api/*` routes proxy the Bearer header
  straight through to the Go API. Recent-saves needs an up-to-date web app —
  the `GET /api/items` proxy handler was added alongside this popup, so
  self-hosters must redeploy the web app to see the recent list populate.
- Icons under `public/icon/` are placeholder solid-cobalt (`#2438FF`) squares —
  replace them before any store submission.
- `pnpm --filter extension zip` (and `zip:firefox`) produce the store upload
  archives in `.output/`.
