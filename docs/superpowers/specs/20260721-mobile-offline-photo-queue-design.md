# Mobile offline photo queue — durable image capture across in-app, Android share, and iOS share extension

Date: 2026-07-21. Closes the last online-only capture gap on mobile. Links,
notes, and shared URLs already survive being offline via the durable capture
queue (`lib/capture-queue.ts`, shipped 2026-07-16), but **images do not**: the
Capture screen's `uploadFiles` (Choose/Take photo and Android shared-image
intents) fails on a network error and the picked photo is lost, and the iOS
native share extension shows "Network error — is the instance reachable?" and
gives up. This violates *capture is sacred* — a save should never be lost to a
flaky connection.

## Goal

Every image capture path survives being offline and syncs automatically when
connectivity returns, funnelling into **one** unified queue:

- In-app Capture (**Choose photo** / **Take photo**).
- Android shared-image intents (share sheet → app → `uploadFiles`).
- iOS native share extension (`targets/share`, saves inline, never opens the app).

## Non-goals

- No new server endpoints or API changes — uploads continue via the existing
  `POST /api/assets` (multipart) and `POST /api/items` (url/note).
- No new required infrastructure or mobile native module. The iOS transport
  reuses the App Group already wired for the share extension.
- Web capture stays online-only (dev/preview surface; no durable FS/queue there).

## Current state (verified)

- `lib/capture-queue.ts` — durable, AsyncStorage-backed queue keyed
  `openmind.captureQueue`. Entries are `{ id, url?, note?, createdAt, attempts }`.
  Flushes oldest-first via `saveItem` (`POST /api/items`). Serialised with a
  lock chain; deduplicates pending URLs; caps at `MAX_QUEUE = 100` (drops
  oldest); drops permanent 4xx; stops the walk on 401; bumps `attempts` and
  stops the pass on network (status 0) / 429 / 5xx.
- `lib/capture-queue-context.tsx` — React surface: `pendingCount`, `flush`,
  `enqueue`, plus AppState-active / NetInfo-reconnect / focus flush triggers.
- `app/(tabs)/capture.tsx` — `uploadFiles(files)` loops over `uploadAsset`
  (`POST /api/assets`). On `status === 0` it shows an error and **loses** the
  image; on 415 it shows an unsupported-format error; multi-file uploads bail on
  the first failure.
- `lib/api.ts` — `uploadAsset({ uri, name, type })` does the multipart POST;
  RN `FormData` accepts a `{ uri, name, type }` file part, so a persisted local
  file path works identically to a picker URI.
- iOS share extension — `targets/share/ShareViewController.swift` reads
  `{ instanceUrl, token }` from the App Group UserDefaults suite
  `group.fun.gilla.openmind` (mirrored by `lib/settings.ts` via
  `@bacons/apple-targets` `ExtensionStorage`), uploads via an ephemeral
  `URLSession`, and on `error != nil` just reports failure.
- `@bacons/apple-targets` `ExtensionStorage` exposes **only** the App Group
  UserDefaults suite (`get`/`set`/`remove`, JSON via `setArray`/`setObject`).
  It does **not** expose the App Group container file path.
- `expo-file-system@57.0.1` is installed transitively but **not** declared in
  `apps/mobile/package.json`.

## Part A — Unified JS offline queue for images

Covers in-app Capture and Android shared-image intents (both go through
`uploadFiles`).

**Persistence.** Picker/share URIs are ephemeral cache files the OS may evict,
so on enqueue we **copy** the image into a durable queue directory
`<documentDir>/capture-queue/<id>.<ext>` and store that path. Declare
`expo-file-system` in `package.json` (pin to the installed `57.0.1`). The exact
copy/delete API surface (legacy `FileSystem.copyAsync`/`deleteAsync` vs the new
`File`/`Paths`) is confirmed against the installed version during
implementation; the design is independent of which variant.

**Extend `lib/capture-queue.ts`** — one queue, not two:

1. `QueuedCapture` gains an optional
   `asset?: { filePath: string; name: string; type: string }`, backward
   compatible with existing url/note rows. `readQueue`'s validation filter
   accepts asset rows (`asset` object with a string `filePath`).
2. New `enqueueAsset(files: AssetUpload[])`: under the queue lock, copy each
   source URI into the queue dir and append one entry per file. No dedupe for
   assets (two photos are two genuine saves, unlike a repeated URL).
3. `flushQueue` branches per entry: `entry.asset` → `uploadAsset({ uri:
   filePath, name, type })`; else url/note → `saveItem`. Status handling is
   shared verbatim (201/permanent-4xx-drop/401-stop/network·429·5xx-bump-stop).
4. `deleteAssetFile(entry)` helper runs on **every** removal path — flush
   success, permanent-4xx drop, `MAX_QUEUE` eviction (`slice(dropped)`), and
   `removeQueued`. This is the leak-prevention crux: no queue entry is dropped
   without deleting its backing file. Deletion is best-effort (a missing file
   is not an error).

**`lib/capture-queue-context.tsx`**: expose `enqueueAsset`. `pendingCount` and
all flush triggers already cover asset entries, so the "N waiting to sync /
Sync now" strip works unchanged.

**`app/(tabs)/capture.tsx` `uploadFiles`**: on `status === 0` (network) for a
file, `enqueueAsset` **that file plus every remaining un-attempted file**, then
set status to `queued` (reusing the existing queued affordance). Files already
uploaded in the loop stay saved. Genuinely-permanent failures (415 unsupported
format, other non-retryable 4xx) still surface as errors — never queue a save
that can never succeed. A 401 mid-loop still stops with the token-rejected
state.

Android shared images already route through `uploadFiles`, so they inherit this
with no extra code.

## Part B — iOS native share extension offline (approach B: container file + path via UserDefaults)

**`targets/share/ShareViewController.swift`** — on the `error != nil` branch in
`send(_:)`, instead of only reporting failure:

1. Resolve the App Group container:
   `FileManager.default.containerURL(forSecurityApplicationGroupIdentifier:
   "group.fun.gilla.openmind")`.
2. Write the payload's bytes to `<container>/pending-shares/<uuid>.<ext>`
   (image) — url/note payloads carry no file.
3. Append a manifest record to a `pendingShares` array in the App Group
   UserDefaults suite:
   `{ kind: "asset", path, name, mimeType, createdAt }` or
   `{ kind: "url" | "note", value, createdAt }`. Read-modify-write the existing
   array so multiple offline shares accumulate.
4. Cap `pendingShares` at 20, dropping the oldest (and its file) beyond that, to
   bound container disk.
5. Change the failure copy to **"Saved offline — will sync when you reopen
   Openmind."** (network case only; 401/415/other HTTP failures keep their
   existing messages — they are not transient).

The container path is obtained inside the extension (native Swift) and passed to
the app as a plain string; the App Group container is mounted at the **same
path** in both processes, so no native module is needed to expose it.

**App side — new `lib/shared-pending.ts`, wired into `CaptureQueueProvider`:**
`drainSharedPending()` runs on launch and on AppState→active (iOS only):

1. Read `pendingShares` via `ExtensionStorage.get` (returns the JSON manifest
   string); parse defensively (empty/malformed → no-op).
2. For each record, oldest-first:
   - `asset` → read the container file at `path` (prefix `file://` if the FS API
     requires it), copy it into the JS queue dir, `enqueueAsset`, then delete
     the container file.
   - `url` / `note` → `enqueue`.
3. **Clear each record from the `pendingShares` UserDefaults array only after
   its file is safely copied into the JS queue** (write the shrinking array
   back per record). A crash mid-drain therefore leaves un-drained records
   intact rather than double-saving.
4. After draining, `flush()` the unified queue.

This funnels iOS extension offline saves into the same queue and the same single
upload path as Part A.

## Error handling

The queue's existing taxonomy is reused unchanged for assets:

- **Network (0) / 429 / 5xx** — transient: bump `attempts`, stop the pass, retry
  on the next flush trigger.
- **401** — stop the walk (bad token); the file is retained for a later valid
  token.
- **Permanent 4xx (incl. 415)** — drop the entry **and** delete its file, so one
  bad image cannot wedge the queue.
- iOS drain is best-effort and idempotent by construction (clear-after-copy).

## Testing

- Table-driven Jest over `capture-queue`:
  - asset enqueue copies a file and appends an entry;
  - flush of an asset entry calls `uploadAsset` and, on 201, removes the entry
    **and** deletes the file;
  - mixed url + asset queue flushes oldest-first with correct branching;
  - permanent 4xx (415) drops the asset entry and deletes the file;
  - `MAX_QUEUE` eviction deletes the evicted files;
  - `removeQueued` deletes the backing file.
  `expo-file-system` and `api.uploadAsset`/`saveItem` are faked so tests are
  deterministic and offline.
- `shared-pending` manifest: parse valid/empty/malformed JSON; drain enqueues
  each record and clears it; a simulated mid-drain stop leaves the remaining
  records intact (idempotency / no double-save).
- Swift is not unit-tested here (consistent with the existing extension) and is
  verified manually on a dev build.

## Rollout

- **Part A** is JS-only and ships in the current build; no native change.
- **Part B** edits Swift, so it needs a fresh iOS dev/EAS build to exercise
  end-to-end. Ship Part A first if a build is not immediately available.
- No config, env var, or `docker-compose.yml` change (client-only).
