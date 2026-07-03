# openmind extension

WXT + React browser extension (thin capture client). Enrichment logic stays
server-side; this app only captures and displays.

## Scripts

- `pnpm --filter extension dev` — dev with HMR (Chrome)
- `pnpm --filter extension build` — production build → `.output/chrome-mv3`
- `pnpm --filter extension build:firefox` — Firefox build
- `pnpm --filter extension lint` — `tsc --noEmit`

## Notes

- Manifest requests broad `host_permissions` (`https://*/*`, `http://localhost/*`)
  for now. Narrow-origin **optional** host permissions (requesting access per
  instance origin at runtime) are deferred to a later task.
- Popup and background entrypoints are added in Task 4; the options page is the
  only entrypoint so far.
