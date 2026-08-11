# Dock functional polish — offline queue, tray Desk, window memory, more browsers

Date: 2026-08-11
Scope: `apps/dock` only. No API, web, mobile, or contract changes.

## Why

The dock's last dedicated pass was dock v2 (2026-07-16). Since then it has only
been touched by sweep changes (hosted default #48, pagination envelope #66),
while web and mobile each got full design passes. Four functional gaps remain,
one of which loses user data:

`quick_save` (⌘⇧S) on a network error fires a notification — `"Save failed
(network error)"` — and **the capture is gone**. No queue, no retry, no record.
That contradicts principle #1 (*capture is sacred*), and mobile already solved
the same problem with a durable queue in PR #44. The dock's own README admits
it: *"Not yet: Windows/Linux, offline save queue."*

The other three are the long-standing `TODO.md` "Dock follow-ups" line, minus
Win/Linux tab grab (deliberately deferred — see Out of scope). Note that same
TODO line is stale on two counts: hotkey rebinding and DMG/notarisation both
shipped in dock v2.

## Decisions taken before design

Two questions were raised at design review and not answered; the calls below
were made by the implementer and are **maintainer-overridable**.

1. **5xx is queued, but worded differently from offline.** Queueing 5xx risks an
   instance that is up but broken looking like an offline save. Rather than
   narrow the policy (which would drop captures during a deploy blip), the
   notification wording separates the cases: a network failure says "Saved
   offline — will retry", a 5xx/429 says "Instance error — queued, will retry".
   `lastError` is surfaced in the expanded strip so a persistent 500 is visible
   rather than inferred.
2. **No background polling for the tray Desk submenu.** It refreshes on launch,
   after any successful save, and on panel focus. Panel focus already fires
   (`onFocusChanged` → `bumpHome()`) and is a better "the user is active" signal
   than a 5-minute timer, with no background HTTP when the dock is idle.

## 1. Offline save queue — `src-tauri/src/queue.rs` (new)

The queue is **Rust-owned**. The failing ⌘⇧S save originates in Rust, so there
is no handoff to lose; the tray needs the pending count and the tray is Rust;
and it drains on launch without depending on the webview. The alternative —
mirroring mobile's TS module and having Rust emit a `save-failed` event for the
panel to enqueue — was rejected because that enqueue would depend on the webview
receiving an event at the exact moment a save failed. `Panel.tsx` ships a
`PanelErrorBoundary`, so a crashed or reloading webview is a real state, and
losing that handoff loses the capture: precisely the bug being fixed.

Unifying both HTTP paths behind a single Rust `save_item` command is the cleaner
end state but is a refactor past this scope. Approach A leaves the door open.

### Storage

JSON array at `app_config_dir()/queue.json`, beside the existing
`shortcuts.json`. Entry:

```rust
#[serde(rename_all = "camelCase")]
struct QueuedCapture {
    id: String,
    url: Option<String>,
    note: Option<String>,
    created_at: i64,        // ms since epoch
    attempts: u32,
    last_error: Option<String>,
}
```

`camelCase` at the serde boundary so the TS side needs no field mapping (unlike
`settings.rs`, which exposes `instance_url` and is remapped in `settings.ts`).
`last_error` is display text only — **never** the token, and never a raw
response body.

Ids avoid a `uuid` dependency (CLAUDE.md: justify every new dependency):
`created_at` millis plus a process-lifetime `AtomicU64` counter.

### Policy

Ported verbatim from mobile's `capture-queue.ts` so both clients behave
identically:

- Cap `MAX_QUEUE = 100`. On overflow drop oldest, `log::warn!` the **count
  only**, never contents.
- URL dedupe: enqueueing a URL already pending returns the existing id and
  `deduped: true`. Notes never dedupe — two notes are two genuine saves.
- Flush walks oldest-first. Per entry:
  - `201` → remove.
  - `401` → **stop the entire pass**, entry intact. A bad token must not burn
    the queue.
  - permanent 4xx (400–499 except 401 and 429) → drop with a `log::warn!`, so
    one poison entry cannot block the rest.
  - network failure / 429 / 5xx → `attempts += 1`, record `last_error`, **stop
    the pass**.

### Two deviations forced by Rust

Both are consequences of using a raw file where mobile had AsyncStorage:

- **Atomic writes.** Write `queue.json.tmp`, then `fs::rename`. A crash
  mid-write must not truncate the queue. An unparsable file is treated as empty
  with a `log::warn!` — the same fallback shape `load_shortcuts` already uses.
- **No lock held across `await`.** `std::sync::Mutex` cannot be held across an
  await point, and flush performs async HTTP. Instead of mobile's `lockChain`, a
  `Mutex` guards short read-modify-write sections and an `AtomicBool` guarantees
  a single flush at a time: lock → take next entry → unlock → await HTTP → lock
  → apply result → unlock. Same clobber-safety as the lock chain, and no
  `tokio::sync` dependency (tokio is in the tree via tauri but not declared
  directly).

### Enqueue triggers

- Rust `quick_save`: the network-error arm and the 429/5xx arms. 401 and other
  4xx keep today's wording, since they are not queued.
- TS `performSave`: `status === 0`, 429, and 5xx invoke `queue_enqueue`,
  replacing the current red error toast with the pending strip.

### Drain triggers

- Launch, once, and only after the settings read succeeds — no point burning
  `attempts` while the dock is unconfigured.
- A 60s interval that idles entirely when the queue is empty.
- Panel focus (`onFocusChanged`, which already fires).
- Manual: tray → "Retry pending saves", and the strip's "Retry now".

### Commands and events

`queue_list`, `queue_enqueue`, `queue_flush`, `queue_remove`. Every mutation
writes the file, emits `queue-changed` with the new list to the panel, and
triggers a tray rebuild.

## 2. Pending strip — `src/panel/PendingStrip.tsx` (new)

Rendered under the confirm strip. Both can be visible at once: confirm is
transient, pending persists. Absent entirely when the queue is empty.

- **Collapsed** (default): `"3 saves waiting to sync"` + `Retry now` + an expand
  chevron.
- **Expanded**: up to 5 rows — `host(url)` or a note excerpt, relative age, and
  a `×` to discard — then "+N more".
- **Colour**: `noteSurface` background like the confirm strip, with the **gold**
  accent. Pending is neither error nor success. `danger` is reserved for entries
  at `attempts >= 5`, which reads as genuinely stuck.

The strip is **chrome**: it must not steal focus from the search input, and must
not join the ↑/↓ `navigable` list — Tab-reachable only. This is the same trap
the confirm strip already had to solve (see the focus-race comments in
`Panel.tsx`); the same discipline applies.

Typed invoke wrappers and the `queue-changed` subscription live in
`src/lib/queue.ts`, keeping the component a pure renderer.

### Targeted cleanup

`Panel.tsx` is already 994 lines. The existing confirm-strip block is extracted
into `src/panel/ConfirmStrip.tsx` as `PendingStrip.tsx` is added beside it, so
Panel gets shorter rather than longer. In scope because it is the file being
worked in; no unrelated refactoring.

## 3. Tray Desk submenu

A `Submenu` between "Save current tab" and "Settings", populated from
`GET /api/desk` in Rust (same reqwest client shape as `quick_save`): 8 items
max, label `truncate(title, 48)`, id `desk:{item_id}`, click opens
`{instance_url}/item/{id}` via `tauri_plugin_opener` (`opener:default` is
already granted, so no capability change).

Tauri v2 exposes no reliable "tray menu about to open" hook, so refresh happens
on launch, after any successful save, and on panel focus — see Decision 2.

When the dock is unconfigured or the fetch fails, the submenu holds a single
**disabled** item ("Open Settings first" / "Couldn't load Desk") rather than
vanishing. A disappearing menu item is a worse bug report than a disabled one.

The pending count lives here too: a disabled `"N pending saves"` plus an enabled
`"Retry pending saves"`, both omitted when the queue is empty. Tauri v2 rebuilds
tray menus to change them, so a single `rebuild_tray_menu()` helper owns the
whole menu and both the queue and Desk refresh paths call it.

## 4. Window behaviour — `src-tauri/src/window.rs` (new)

- `tauri.conf.json`: `resizable: true`, `minWidth: 520`, `minHeight: 360`, and
  `center: true` dropped — restore logic owns placement.
- `styles.shell` in `Panel.tsx` currently hardcodes `width: 640, height: 420`, a
  second copy of the config's dimensions. It goes fluid: `100%` / `100vh`. The
  border radius stays; the `body` flex already handles the rest.
- New `window.json` in the config dir (`{x, y, width, height}`), written
  debounced ~500ms on move and resize, read at startup.

**Clamping on restore is the substance of this item.** Walk
`available_monitors()`; if the saved rect does not overlap some monitor's work
area by at least 80×80 logical px, centre on the primary instead. A position saved on a
since-disconnected display must not resurrect offscreen. Size is clamped to the
target monitor too, so a rect saved on a 5K display cannot exceed a laptop
panel.

Written as a pure `clamp_rect(saved, monitors) -> Rect` so it is unit-testable
against synthetic monitor sets without a display server.

## 5. More browsers in tab grab — `src-tauri/src/grab.rs`

`script_for` gains, Chromium-shaped: Vivaldi, Opera, Chromium, and Chrome
Beta/Dev/Canary. Safari-shaped: Safari Technology Preview and Orion. The
existing `chromium_bundles_map` test extends to every new id, keeping the
`com.spotify.client` negative control.

**Caveat, to be carried into the PR description:** only bundle ids for browsers
actually installed on the build machine can be verified; the rest follow vendor
convention and need a real-app check before being claimed as supported. Orion in
particular — Kagi's bundle id has changed historically, and its AppleScript
dictionary mimics Safari's without being guaranteed identical. Any unverified
entry is harmless when wrong (it simply never matches, and the user gets today's
"Front app isn't a supported browser") but must not be advertised in the README
as verified.

## Testing

**Rust (`cargo test`)**, pure-function style matching the existing accelerator
tests:

- Flush policy table: `201` removes; `401` stops the pass with the entry intact;
  `404` drops; `500`, `429`, and network-failure each bump `attempts` and stop.
- Cap-100 eviction drops the oldest.
- URL dedupe returns the existing id; notes do not dedupe.
- Corrupt / truncated `queue.json` → empty queue, no panic.
- `clamp_rect` against synthetic monitor sets: fully-visible rect passes
  through; offscreen rect re-centres; oversized rect shrinks to the monitor.
- `script_for` covers every new bundle id, negative control intact.

The flush-policy tests require the HTTP call to sit behind a small trait or
function pointer so a fake can return canned statuses. That seam is worth having
regardless.

**TS (vitest)**: `lib/queue.ts` wrappers, and a pure `pendingSummary()` for the
strip's collapsed/expanded label logic — extracted the way `save-confirm.ts` and
`home-lists.ts` already are, so the component stays a renderer.

**Requires a human; cannot be automated:**

- The real offline round trip: drop wifi → ⌘⇧S → observe the queued
  notification → reconnect → confirm the drain and the item's arrival.
- macOS Automation consent for each newly-added browser.
- Multi-monitor restore, including the disconnected-display case.
- That the pending strip does not steal focus mid-keystroke in practice.

## Out of scope

- **Win/Linux tab grab.** No AppleScript equivalent exists; it would need
  per-browser platform hacks or a bridge through the existing browser
  extension — which already covers those platforms. Largest effort, least
  payoff.
- **Unifying the two HTTP paths** (Rust reqwest at 15s, TS plugin-http at 12s)
  behind one Rust `save_item` command. The right end state, but a refactor
  beyond this pass.
- **Image/asset captures in the queue.** The dock has no image capture path;
  mobile's `enqueueAsset` and `asset-store` have no dock analogue to mirror.

## Files touched

New: `src-tauri/src/queue.rs`, `src-tauri/src/window.rs`,
`src/lib/queue.ts`, `src/panel/PendingStrip.tsx`, `src/panel/ConfirmStrip.tsx`.

Changed: `src-tauri/src/lib.rs`, `src-tauri/src/grab.rs`,
`src-tauri/tauri.conf.json`, `src/panel/Panel.tsx`, `apps/dock/README.md`
(offline queue and browser list), plus the test files above.

No capability changes. No new dependencies.
