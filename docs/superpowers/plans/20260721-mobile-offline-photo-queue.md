# Mobile Offline Photo Queue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every mobile image-capture path (in-app Capture, Android share intents, iOS native share extension) survive being offline and sync automatically when connectivity returns.

**Architecture:** Extend the existing durable capture queue (`lib/capture-queue.ts`) to hold image assets alongside URLs/notes, persisting the image bytes to a durable app directory via `expo-file-system` so ephemeral picker/share URIs cannot be lost. The iOS share extension, on a network failure, writes the image into the shared App Group container plus a small UserDefaults manifest; the app drains that manifest into the same JS queue on foreground. All paths converge on one flush routine.

**Tech Stack:** Expo SDK 57, React Native, TypeScript, `expo-file-system@57` (new `Paths`/`File`/`Directory` API), `@react-native-async-storage/async-storage`, `@bacons/apple-targets` `ExtensionStorage`, Swift (iOS share extension), `jest-expo` for unit tests.

## Global Constraints

- **Capture is sacred** — a save must never be lost or blocked; a network failure enqueues, it does not error away the content.
- **Idempotent, retryable** — every queue entry is safe to flush twice; draining clears a record only after its bytes are safely in the JS queue.
- **No new required infrastructure and no new native module** — the iOS transport reuses the App Group (`group.fun.gilla.openmind`) already wired for the share extension.
- **Thin client** — mobile does capture + display only; no business/enrichment logic.
- Pin `expo-file-system` to the installed **`57.0.1`** (do not float the range).
- Package manager for `apps/mobile` is **npm** (there is a `package-lock.json`; EAS runs `npm ci`).
- Use the new `expo-file-system` object API (`Paths`, `File`, `Directory`) — not the `expo-file-system/legacy` subpath.
- **No comment banner blocks** (no `// ======` section dividers). Match the terse top-of-file comment style already in `lib/`.
- UK English in copy; do not hardcode palette colours (use existing tokens / the Swift constants already in `ShareViewController.swift`).
- App Group identifier is exactly `group.fun.gilla.openmind`.
- Work happens from `apps/mobile/`. All paths below are relative to `apps/mobile/` unless noted.

---

### Task 1: Test harness + `expo-file-system` dependency

Stand up `jest-expo` (the app currently has no test runner) and declare `expo-file-system`. Deliverable: `npm test` runs and a trivial test passes.

**Files:**
- Modify: `package.json` (add dep + devDeps + `test` script)
- Create: `jest.config.js`
- Create: `jest.setup.js`
- Create: `lib/__tests__/harness.test.ts` (throwaway proof; deleted at the end of this task's commit is optional — keep it, it is cheap)

**Interfaces:**
- Consumes: nothing.
- Produces: a working `npm test`; global AsyncStorage mock available to all tests.

- [ ] **Step 1: Install SDK-aligned versions via `expo install`**

`jest-expo` and `expo-file-system` versions must match the installed Expo SDK (this app is `expo@~57`), so let Expo pick them rather than hardcoding. Run (from `apps/mobile`; note `npx` is shimmed in this environment — use the local binary):

```bash
./node_modules/.bin/expo install expo-file-system jest-expo
```

Expected: `expo-file-system` resolves to `~57.0.1` and `jest-expo` to the SDK-57-compatible version; both are written into `package.json`. If it reports a peer conflict, the repo already resolves EAS installs with `--legacy-peer-deps` — re-run `npm install --legacy-peer-deps` once afterwards.

- [ ] **Step 2: Add jest, its types, and the `test` script**

`jest` and `@types/jest` are plain devDependencies (not SDK-versioned):

```bash
npm install -D jest @types/jest
```

Then add a `test` script to `scripts` in `package.json`:

```json
"test": "jest"
```

Confirm `package.json` now lists `expo-file-system` under `dependencies` and `jest`, `jest-expo`, `@types/jest` under `devDependencies`.

- [ ] **Step 3: Create `jest.config.js`**

```js
// Jest config for the Expo app. jest-expo supplies the RN/Expo transform;
// tests mock native modules (expo-file-system, ExtensionStorage, ./api) so the
// pure queue/drain logic runs under Node without a device.
module.exports = {
  preset: "jest-expo",
  setupFiles: ["<rootDir>/jest.setup.js"],
  testMatch: ["**/__tests__/**/*.test.ts", "**/*.test.ts"],
  transformIgnorePatterns: [
    "node_modules/(?!((jest-)?react-native|@react-native(-community)?|expo(nent)?|@expo(nent)?/.*|@expo-google-fonts/.*|@bacons/.*|react-navigation|@react-navigation/.*|@unimodules/.*|unimodules|sentry-expo|native-base|react-native-svg))",
  ],
};
```

- [ ] **Step 4: Create `jest.setup.js`**

```js
// Global AsyncStorage mock for every test.
jest.mock("@react-native-async-storage/async-storage", () =>
  require("@react-native-async-storage/async-storage/jest/async-storage-mock"),
);
```

- [ ] **Step 5: Create `lib/__tests__/harness.test.ts`**

```ts
import AsyncStorage from "@react-native-async-storage/async-storage";

test("jest harness runs and AsyncStorage is mocked", async () => {
  await AsyncStorage.setItem("k", "v");
  expect(await AsyncStorage.getItem("k")).toBe("v");
});
```

- [ ] **Step 6: Run tests**

Run: `npm test`
Expected: PASS (1 test). If jest-expo fails to resolve a native ESM module, widen `transformIgnorePatterns` to include it, then re-run.

- [ ] **Step 7: Typecheck**

Run: `npm run typecheck`
Expected: no errors (adding `@types/jest` makes `test`/`expect` types available).

- [ ] **Step 8: Commit**

```bash
git add package.json package-lock.json jest.config.js jest.setup.js lib/__tests__/harness.test.ts
git commit -m "test(mobile): add jest-expo harness; declare expo-file-system"
```

---

### Task 2: `asset-store.ts` — durable queue-directory helpers

Isolate all `expo-file-system` access behind a small module so `capture-queue` stays pure logic and mockable.

**Files:**
- Create: `lib/asset-store.ts`
- Create: `lib/__tests__/asset-store.test.ts`

**Interfaces:**
- Consumes: `expo-file-system` (`Paths`, `File`, `Directory`).
- Produces:
  - `extForMime(mime: string): string` — returns `"jpg" | "png" | "webp" | "gif"` (default `"jpg"`).
  - `copyIntoQueue(sourceUri: string, id: string, mime: string): Promise<string>` — copies `sourceUri` into `<documentDir>/capture-queue/<id>.<ext>`, returns the destination `file://` uri.
  - `deleteQueueFile(uri: string): void` — best-effort delete (missing file is not an error).

- [ ] **Step 1: Write the failing test**

`lib/__tests__/asset-store.test.ts`:

```ts
import { copyIntoQueue, deleteQueueFile, extForMime } from "../asset-store";

const copyMock = jest.fn();
const deleteMock = jest.fn();

jest.mock("expo-file-system", () => {
  class Directory {
    uri: string;
    exists = false;
    constructor(...parts: unknown[]) {
      this.uri = parts.map(String).join("/");
    }
    create() {
      this.exists = true;
    }
  }
  class File {
    uri: string;
    exists = true;
    constructor(...parts: unknown[]) {
      this.uri = parts.map((p) => (p && (p as { uri?: string }).uri) || String(p)).join("/");
    }
    copy(dest: { uri: string }) {
      copyMock(this.uri, dest.uri);
      return Promise.resolve();
    }
    delete() {
      deleteMock(this.uri);
    }
  }
  return { Paths: { document: { uri: "DOC" } }, Directory, File };
});

test("extForMime maps known types and defaults to jpg", () => {
  expect(extForMime("image/png")).toBe("png");
  expect(extForMime("image/webp")).toBe("webp");
  expect(extForMime("image/gif")).toBe("gif");
  expect(extForMime("image/jpeg")).toBe("jpg");
  expect(extForMime("application/octet-stream")).toBe("jpg");
});

test("copyIntoQueue copies source into the queue dir and returns the dest uri", async () => {
  const dest = await copyIntoQueue("file:///tmp/pick.jpg", "abc", "image/png");
  expect(dest).toContain("capture-queue");
  expect(dest).toContain("abc.png");
  expect(copyMock).toHaveBeenCalledWith("file:///tmp/pick.jpg", dest);
});

test("deleteQueueFile deletes when the file exists", () => {
  deleteQueueFile("file:///DOC/capture-queue/abc.png");
  expect(deleteMock).toHaveBeenCalled();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- asset-store`
Expected: FAIL — cannot find module `../asset-store`.

- [ ] **Step 3: Write `lib/asset-store.ts`**

```ts
// Durable storage for queued image captures. Picker/share URIs are ephemeral
// cache files the OS can evict, so a queued image is copied into the app's
// document directory and referenced by that path until it uploads.
import { Directory, File, Paths } from "expo-file-system";

const QUEUE_DIR_NAME = "capture-queue";

export function extForMime(mime: string): string {
  switch (mime) {
    case "image/png":
      return "png";
    case "image/webp":
      return "webp";
    case "image/gif":
      return "gif";
    default:
      return "jpg";
  }
}

function queueDir(): Directory {
  return new Directory(Paths.document, QUEUE_DIR_NAME);
}

function ensureQueueDir(): Directory {
  const dir = queueDir();
  if (!dir.exists) dir.create({ intermediates: true, idempotent: true });
  return dir;
}

export async function copyIntoQueue(
  sourceUri: string,
  id: string,
  mime: string,
): Promise<string> {
  const dir = ensureQueueDir();
  const dest = new File(dir, `${id}.${extForMime(mime)}`);
  await new File(sourceUri).copy(dest, { overwrite: true });
  return dest.uri;
}

export function deleteQueueFile(uri: string): void {
  try {
    const file = new File(uri);
    if (file.exists) file.delete();
  } catch {
    // Best-effort — a missing file is already the desired state.
  }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm test -- asset-store`
Expected: PASS (3 tests).

- [ ] **Step 5: Typecheck**

Run: `npm run typecheck`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add lib/asset-store.ts lib/__tests__/asset-store.test.ts
git commit -m "feat(mobile): asset-store — durable capture-queue image dir"
```

---

### Task 3: Extend `capture-queue.ts` with image assets

Add asset entries to the durable queue, flush them via `uploadAsset`, and delete their backing files on every removal path.

**Files:**
- Modify: `lib/capture-queue.ts`
- Modify: `lib/__tests__` — Create: `lib/__tests__/capture-queue.test.ts`

**Interfaces:**
- Consumes: `copyIntoQueue`, `deleteQueueFile` from `./asset-store`; `saveItem`, `uploadAsset`, `AssetUpload` from `./api`.
- Produces (used by Tasks 4, 5, 6):
  - `QueuedCapture` now includes optional `asset?: { filePath: string; name: string; type: string }`.
  - `enqueueAsset(files: AssetUpload[]): Promise<{ ids: string[] }>`.
  - `flushQueue()` unchanged signature: `Promise<{ sent: number; remaining: number }>`, now uploads asset entries.

- [ ] **Step 1: Write the failing test**

`lib/__tests__/capture-queue.test.ts`:

```ts
import AsyncStorage from "@react-native-async-storage/async-storage";

const copyIntoQueue = jest.fn(
  async (_src: string, id: string, _mime: string) => `file:///q/${id}.jpg`,
);
const deleteQueueFile = jest.fn();
jest.mock("../asset-store", () => ({
  copyIntoQueue: (s: string, id: string, m: string) => copyIntoQueue(s, id, m),
  deleteQueueFile: (u: string) => deleteQueueFile(u),
  extForMime: () => "jpg",
}));

const uploadAsset = jest.fn();
const saveItem = jest.fn();
jest.mock("../api", () => ({
  uploadAsset: (f: unknown) => uploadAsset(f),
  saveItem: (p: unknown) => saveItem(p),
}));

import { enqueueAsset, flushQueue, listQueued } from "../capture-queue";

beforeEach(async () => {
  await AsyncStorage.clear();
  copyIntoQueue.mockClear();
  deleteQueueFile.mockClear();
  uploadAsset.mockClear();
  saveItem.mockClear();
});

test("enqueueAsset copies each file and stores an asset entry", async () => {
  const { ids } = await enqueueAsset([
    { uri: "file:///tmp/a.jpg", name: "a.jpg", type: "image/jpeg" },
  ]);
  expect(ids).toHaveLength(1);
  expect(copyIntoQueue).toHaveBeenCalledTimes(1);
  const pending = await listQueued();
  expect(pending[0].asset).toEqual({
    filePath: `file:///q/${ids[0]}.jpg`,
    name: "a.jpg",
    type: "image/jpeg",
  });
});

test("flush uploads an asset entry, then removes it and deletes its file", async () => {
  uploadAsset.mockResolvedValue({ ok: true, status: 201 });
  await enqueueAsset([{ uri: "file:///tmp/a.jpg", name: "a.jpg", type: "image/jpeg" }]);
  const res = await flushQueue();
  expect(res).toEqual({ sent: 1, remaining: 0 });
  expect(uploadAsset).toHaveBeenCalledWith(
    expect.objectContaining({ name: "a.jpg", type: "image/jpeg" }),
  );
  expect(deleteQueueFile).toHaveBeenCalledWith(expect.stringContaining("file:///q/"));
});

test("permanent 4xx on an asset drops the entry and deletes its file", async () => {
  uploadAsset.mockResolvedValue({ ok: false, status: 415 });
  await enqueueAsset([{ uri: "file:///tmp/a.jpg", name: "a.jpg", type: "image/jpeg" }]);
  const res = await flushQueue();
  expect(res.remaining).toBe(0);
  expect(deleteQueueFile).toHaveBeenCalled();
});

test("network error on an asset keeps the entry and file, bumps attempts", async () => {
  uploadAsset.mockResolvedValue({ ok: false, status: 0 });
  await enqueueAsset([{ uri: "file:///tmp/a.jpg", name: "a.jpg", type: "image/jpeg" }]);
  const res = await flushQueue();
  expect(res.remaining).toBe(1);
  expect(deleteQueueFile).not.toHaveBeenCalled();
  const pending = await listQueued();
  expect(pending[0].attempts).toBe(1);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- capture-queue`
Expected: FAIL — `enqueueAsset` is not exported.

- [ ] **Step 3: Extend the `QueuedCapture` type and imports**

In `lib/capture-queue.ts`, update the import line and the type. Replace:

```ts
import AsyncStorage from "@react-native-async-storage/async-storage";
import { saveItem } from "./api";
```

with:

```ts
import AsyncStorage from "@react-native-async-storage/async-storage";
import { saveItem, uploadAsset, type AssetUpload } from "./api";
import { copyIntoQueue, deleteQueueFile } from "./asset-store";
```

Replace the `QueuedCapture` type with:

```ts
export type QueuedCapture = {
  id: string;
  url?: string;
  note?: string;
  /** Present for image captures; bytes live at filePath in the queue dir. */
  asset?: { filePath: string; name: string; type: string };
  createdAt: number;
  attempts: number;
};
```

- [ ] **Step 4: Accept asset rows in `readQueue`**

In `readQueue`, replace the `.filter(...)` predicate body so asset rows validate:

```ts
    return parsed.filter(
      (row): row is QueuedCapture =>
        !!row &&
        typeof row === "object" &&
        typeof (row as QueuedCapture).id === "string" &&
        typeof (row as QueuedCapture).createdAt === "number",
    );
```

(no change needed — the existing predicate already admits asset rows because it only checks `id`/`createdAt`; leave as-is. This step is a verification step: confirm the predicate does not reference `url`/`note`.)

- [ ] **Step 5: Add a cleanup helper and `enqueueAsset`**

Add, just after the `enqueue` function:

```ts
/** Delete an asset entry's backing file when it leaves the queue for good. */
function cleanupAsset(entry: QueuedCapture): void {
  if (entry.asset) deleteQueueFile(entry.asset.filePath);
}

/**
 * Enqueue one entry per image. Each source is copied into the durable queue
 * dir first, so the ephemeral picker/share URI can be reclaimed safely. No
 * dedupe — two photos are two genuine saves.
 */
export async function enqueueAsset(
  files: AssetUpload[],
): Promise<{ ids: string[] }> {
  return withQueueLock(async () => {
    let items = await readQueue();
    const ids: string[] = [];
    for (const file of files) {
      const id = newId();
      const filePath = await copyIntoQueue(file.uri, id, file.type);
      items = [
        ...items,
        {
          id,
          asset: { filePath, name: file.name, type: file.type },
          createdAt: Date.now(),
          attempts: 0,
        },
      ];
      ids.push(id);
    }
    if (items.length > MAX_QUEUE) {
      const dropped = items.length - MAX_QUEUE;
      for (const e of items.slice(0, dropped)) cleanupAsset(e);
      console.warn(`[capture-queue] cap ${MAX_QUEUE} exceeded; dropping ${dropped} oldest`);
      items = items.slice(dropped);
    }
    await writeQueue(items);
    return { ids };
  });
}
```

- [ ] **Step 6: Delete files on the URL/note enqueue cap-eviction path too**

In the existing `enqueue` function, its cap-eviction block currently slices without cleanup. Replace:

```ts
    if (items.length > MAX_QUEUE) {
      const dropped = items.length - MAX_QUEUE;
      console.warn(`[capture-queue] cap ${MAX_QUEUE} exceeded; dropping ${dropped} oldest`);
      items = items.slice(dropped);
    }
```

with:

```ts
    if (items.length > MAX_QUEUE) {
      const dropped = items.length - MAX_QUEUE;
      for (const e of items.slice(0, dropped)) cleanupAsset(e);
      console.warn(`[capture-queue] cap ${MAX_QUEUE} exceeded; dropping ${dropped} oldest`);
      items = items.slice(dropped);
    }
```

- [ ] **Step 7: Clean up the file in `removeQueued`**

Replace `removeQueued` with:

```ts
export async function removeQueued(id: string): Promise<void> {
  return withQueueLock(async () => {
    const items = await readQueue();
    const target = items.find((q) => q.id === id);
    if (!target) return;
    cleanupAsset(target);
    await writeQueue(items.filter((q) => q.id !== id));
  });
}
```

- [ ] **Step 8: Branch the flush on asset vs url/note, and clean up files**

In `flushQueue`, replace the body of the `for` loop from the `const payload = ...` line through the permanent-failure block. Replace:

```ts
      const payload = entry.url ? { url: entry.url } : { note: entry.note ?? "" };
      const res = await saveItem(payload);
      if (res.ok) {
        sent += 1;
        items = items.filter((q) => q.id !== entry.id);
        await writeQueue(items);
        continue;
      }
      if (res.status === 401) {
        break;
      }
      if (isPermanentFailure(res.status)) {
        console.warn(
          `[capture-queue] dropping entry ${entry.id} after permanent HTTP ${res.status}`,
        );
        items = items.filter((q) => q.id !== entry.id);
        await writeQueue(items);
        continue;
      }
```

with:

```ts
      const res = entry.asset
        ? await uploadAsset({
            uri: entry.asset.filePath,
            name: entry.asset.name,
            type: entry.asset.type,
          })
        : await saveItem(entry.url ? { url: entry.url } : { note: entry.note ?? "" });
      if (res.ok) {
        sent += 1;
        cleanupAsset(entry);
        items = items.filter((q) => q.id !== entry.id);
        await writeQueue(items);
        continue;
      }
      if (res.status === 401) {
        break;
      }
      if (isPermanentFailure(res.status)) {
        console.warn(
          `[capture-queue] dropping entry ${entry.id} after permanent HTTP ${res.status}`,
        );
        cleanupAsset(entry);
        items = items.filter((q) => q.id !== entry.id);
        await writeQueue(items);
        continue;
      }
```

- [ ] **Step 9: Run tests to verify they pass**

Run: `npm test -- capture-queue`
Expected: PASS (4 tests). Also run `npm test` to confirm Tasks 1–2 still pass.

- [ ] **Step 10: Typecheck**

Run: `npm run typecheck`
Expected: no errors.

- [ ] **Step 11: Commit**

```bash
git add lib/capture-queue.ts lib/__tests__/capture-queue.test.ts
git commit -m "feat(mobile): queue image assets in the durable capture queue"
```

---

### Task 4: `shared-pending.ts` — drain iOS share-extension offline saves

Read the App Group `pendingShares` manifest the extension writes on failure, funnel each record into the JS queue, and clear it as it goes.

**Files:**
- Create: `lib/shared-pending.ts`
- Create: `lib/__tests__/shared-pending.test.ts`

**Interfaces:**
- Consumes: `enqueue`, `enqueueAsset` from `./capture-queue`; `Paths`, `File`, `Directory` from `expo-file-system`; `ExtensionStorage` from `@bacons/apple-targets`; `Platform` from `react-native`.
- Produces (used by Task 5): `drainSharedPending(): Promise<number>` — returns how many records were drained; a no-op returning `0` on non-iOS or when the native module / manifest is absent.

- [ ] **Step 1: Write the failing test**

`lib/__tests__/shared-pending.test.ts`:

```ts
jest.mock("react-native", () => ({ Platform: { OS: "ios" } }));

const enqueue = jest.fn(async () => ({ id: "x", deduped: false }));
const enqueueAsset = jest.fn(async () => ({ ids: ["y"] }));
jest.mock("../capture-queue", () => ({
  enqueue: (i: unknown) => enqueue(i),
  enqueueAsset: (f: unknown) => enqueueAsset(f),
}));

const fileDelete = jest.fn();
jest.mock("expo-file-system", () => {
  class File {
    uri: string;
    exists = true;
    constructor(...parts: unknown[]) {
      this.uri = parts.map((p) => (p && (p as { uri?: string }).uri) || String(p)).join("/");
    }
    delete() {
      fileDelete(this.uri);
    }
  }
  return {
    File,
    Directory: class {},
    Paths: { appleSharedContainers: { "group.fun.gilla.openmind": { uri: "GROUP" } } },
  };
});

let store: Record<string, unknown> = {};
const set = jest.fn((k: string, v: unknown) => {
  if (v == null) delete store[k];
  else store[k] = v;
});
const get = jest.fn((k: string) => (store[k] == null ? null : JSON.stringify(store[k])));
jest.mock("@bacons/apple-targets", () => ({
  ExtensionStorage: class {
    constructor(_group: string) {}
    get(k: string) {
      return get(k);
    }
    set(k: string, v: unknown) {
      set(k, v);
    }
  },
}));

import { drainSharedPending } from "../shared-pending";

beforeEach(() => {
  store = {};
  enqueue.mockClear();
  enqueueAsset.mockClear();
  fileDelete.mockClear();
  set.mockClear();
});

test("drains an asset record: enqueues it, deletes the container file, clears the manifest", async () => {
  store.pendingShares = [
    { kind: "asset", filename: "u.jpg", name: "u.jpg", mimeType: "image/jpeg", createdAt: 1 },
  ];
  const n = await drainSharedPending();
  expect(n).toBe(1);
  expect(enqueueAsset).toHaveBeenCalledWith([
    expect.objectContaining({ name: "u.jpg", type: "image/jpeg" }),
  ]);
  expect(fileDelete).toHaveBeenCalled();
  expect(store.pendingShares).toBeUndefined();
});

test("drains url and note records", async () => {
  store.pendingShares = [
    { kind: "url", value: "https://e.com", createdAt: 1 },
    { kind: "note", value: "hi", createdAt: 2 },
  ];
  const n = await drainSharedPending();
  expect(n).toBe(2);
  expect(enqueue).toHaveBeenCalledWith({ url: "https://e.com" });
  expect(enqueue).toHaveBeenCalledWith({ note: "hi" });
});

test("malformed manifest is a no-op", async () => {
  store.pendingShares = "not-an-array" as unknown;
  const n = await drainSharedPending();
  expect(n).toBe(0);
});

test("a throw mid-drain leaves the remaining records intact", async () => {
  store.pendingShares = [
    { kind: "url", value: "https://a.com", createdAt: 1 },
    { kind: "url", value: "https://b.com", createdAt: 2 },
  ];
  enqueue.mockImplementationOnce(async () => ({ id: "1", deduped: false }));
  enqueue.mockImplementationOnce(async () => {
    throw new Error("boom");
  });
  const n = await drainSharedPending();
  expect(n).toBe(1);
  expect((store.pendingShares as unknown[]).length).toBe(1);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- shared-pending`
Expected: FAIL — cannot find module `../shared-pending`.

- [ ] **Step 3: Write `lib/shared-pending.ts`**

```ts
// Drains iOS share-extension offline saves into the JS capture queue. When the
// native share extension (targets/share) cannot reach the instance, it writes
// the image into the App Group container and appends a record to the
// `pendingShares` manifest in the App Group UserDefaults. This runs on the
// app's foreground and funnels those records into the same durable queue.
import { Directory, File, Paths } from "expo-file-system";
import { Platform } from "react-native";
import { enqueue, enqueueAsset } from "./capture-queue";

const APP_GROUP = "group.fun.gilla.openmind";
const MANIFEST_KEY = "pendingShares";
const CONTAINER_SUBDIR = "pending-shares";

type PendingShare =
  | { kind: "asset"; filename: string; name: string; mimeType: string; createdAt: number }
  | { kind: "url"; value: string; createdAt: number }
  | { kind: "note"; value: string; createdAt: number };

type Storage = { get: (key: string) => string | null; set: (key: string, value?: unknown) => void };

function extensionStorage(): Storage | null {
  if (Platform.OS !== "ios") return null;
  try {
    const { ExtensionStorage } = require("@bacons/apple-targets");
    return new ExtensionStorage(APP_GROUP) as Storage;
  } catch {
    return null;
  }
}

function containerDir(): Directory | null {
  return Paths.appleSharedContainers?.[APP_GROUP] ?? null;
}

function parseManifest(raw: string | null): PendingShare[] {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((r): r is PendingShare => {
      if (!r || typeof r !== "object") return false;
      const rec = r as PendingShare;
      if (rec.kind === "asset") {
        return (
          typeof rec.filename === "string" &&
          typeof rec.name === "string" &&
          typeof rec.mimeType === "string"
        );
      }
      return (rec.kind === "url" || rec.kind === "note") && typeof rec.value === "string";
    });
  } catch {
    return [];
  }
}

/**
 * Move every pending share into the JS queue, oldest-first. Each record is
 * cleared from the manifest only after its content is safely enqueued, so a
 * crash mid-drain leaves un-drained records intact rather than double-saving.
 * Returns the number of records drained.
 */
export async function drainSharedPending(): Promise<number> {
  const storage = extensionStorage();
  if (!storage) return 0;
  const records = parseManifest(storage.get(MANIFEST_KEY));
  if (records.length === 0) return 0;

  const container = containerDir();
  let remaining = [...records];
  let drained = 0;

  for (const rec of [...records].sort((a, b) => a.createdAt - b.createdAt)) {
    try {
      if (rec.kind === "asset") {
        if (!container) break;
        const src = new File(container, CONTAINER_SUBDIR, rec.filename);
        if (src.exists) {
          await enqueueAsset([{ uri: src.uri, name: rec.name, type: rec.mimeType }]);
          src.delete();
        }
      } else {
        await enqueue(rec.kind === "url" ? { url: rec.value } : { note: rec.value });
      }
      remaining = remaining.filter((r) => r !== rec);
      storage.set(MANIFEST_KEY, remaining.length ? remaining : undefined);
      drained += 1;
    } catch (err) {
      console.warn("[shared-pending] drain stopped", err);
      break;
    }
  }
  return drained;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm test -- shared-pending`
Expected: PASS (4 tests).

- [ ] **Step 5: Typecheck**

Run: `npm run typecheck`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add lib/shared-pending.ts lib/__tests__/shared-pending.test.ts
git commit -m "feat(mobile): drain iOS share-extension offline saves into the queue"
```

---

### Task 5: Wire `enqueueAsset` + drain into `CaptureQueueProvider`

Expose `enqueueAsset` through the context, and drain the iOS manifest on mount and foreground before each flush.

**Files:**
- Modify: `lib/capture-queue-context.tsx`

**Interfaces:**
- Consumes: `enqueueAsset` from `./capture-queue`; `drainSharedPending` from `./shared-pending`; `AssetUpload` from `./api`.
- Produces (used by Task 6): context value gains `enqueueAsset: (files: AssetUpload[]) => Promise<{ ids: string[] }>`.

- [ ] **Step 1: Update imports**

Replace:

```ts
import {
  enqueue as enqueueRaw,
  flushQueue,
  listQueued,
  subscribeQueue,
  type QueuedCapture,
} from "./capture-queue";
```

with:

```ts
import type { AssetUpload } from "./api";
import {
  enqueue as enqueueRaw,
  enqueueAsset as enqueueAssetRaw,
  flushQueue,
  listQueued,
  subscribeQueue,
  type QueuedCapture,
} from "./capture-queue";
import { drainSharedPending } from "./shared-pending";
```

- [ ] **Step 2: Add `enqueueAsset` to the context type**

In `CaptureQueueContextValue`, add after the `enqueue` line:

```ts
  enqueueAsset: (files: AssetUpload[]) => Promise<{ ids: string[] }>;
```

- [ ] **Step 3: Add the `enqueueAsset` callback and a drain+flush helper**

After the existing `enqueue` callback, add:

```ts
  const enqueueAsset = useCallback(async (files: AssetUpload[]) => {
    const result = await enqueueAssetRaw(files);
    setPending(await listQueued());
    return result;
  }, []);

  const drainAndFlush = useCallback(async () => {
    try {
      await drainSharedPending();
    } catch {
      // Draining is best-effort; a failure must never block the flush.
    }
    return flush();
  }, [flush]);
```

- [ ] **Step 4: Use `drainAndFlush` in the mount + foreground effects**

In the AppState effect, replace both `void flush()` calls with `void drainAndFlush()`:

```ts
  useEffect(() => {
    const onChange = (next: AppStateStatus) => {
      if (next === "active") void drainAndFlush();
    };
    const sub = AppState.addEventListener("change", onChange);
    void drainAndFlush();
    return () => sub.remove();
  }, [drainAndFlush]);
```

Leave the NetInfo effect calling `flush` (a reconnect does not produce new share-extension records) — no change there.

- [ ] **Step 5: Expose `enqueueAsset` in the memoised value**

In the `useMemo` value object add `enqueueAsset,` after `enqueue,`, and add `enqueueAsset` to the dependency array:

```ts
  const value = useMemo<CaptureQueueContextValue>(
    () => ({
      pending,
      pendingCount: pending.length,
      flushing,
      refresh,
      enqueue,
      enqueueAsset,
      flush,
    }),
    [pending, flushing, refresh, enqueue, enqueueAsset, flush],
  );
```

- [ ] **Step 6: Typecheck**

Run: `npm run typecheck`
Expected: no errors.

- [ ] **Step 7: Run the full test suite**

Run: `npm test`
Expected: PASS (all prior tests).

- [ ] **Step 8: Commit**

```bash
git add lib/capture-queue-context.tsx
git commit -m "feat(mobile): expose enqueueAsset; drain shared-pending on foreground"
```

---

### Task 6: Queue images on network failure in the Capture screen

Change `uploadFiles` so a network error enqueues the failing file plus every not-yet-attempted file, instead of erroring the content away.

**Files:**
- Modify: `app/(tabs)/capture.tsx`

**Interfaces:**
- Consumes: `enqueueAsset` from `useCaptureQueue()`.

- [ ] **Step 1: Pull `enqueueAsset` from the context**

Replace:

```ts
  const { pendingCount, flushing, enqueue, flush } = useCaptureQueue();
```

with:

```ts
  const { pendingCount, flushing, enqueue, enqueueAsset, flush } = useCaptureQueue();
```

- [ ] **Step 2: Rewrite `uploadFiles` to queue on network error**

Replace the whole `uploadFiles` callback body with an indexed loop that queues the remaining files on `status === 0`:

```ts
  const uploadFiles = useCallback(
    async (files: AssetUpload[]) => {
      if (files.length === 0) return;
      setStatus({ kind: "saving" });
      let saved = 0;
      let lastStatus = 0;
      for (let i = 0; i < files.length; i += 1) {
        const res = await uploadAsset(files[i]);
        lastStatus = res.status;
        if (res.ok) {
          saved += 1;
        } else if (res.status === 401) {
          setStatus({ kind: "rejected" });
          if (saved > 0) invalidateLists();
          return;
        } else if (res.status === 0) {
          // Network error: queue this file and every one not yet attempted so
          // nothing is lost, then let the durable queue sync later.
          await enqueueAsset(files.slice(i));
          if (saved > 0) invalidateLists();
          setStatus({ kind: "queued" });
          return;
        } else if (res.status === 415) {
          setStatus({
            kind: "error",
            message: "That format isn't supported — try JPEG, PNG, WebP, or GIF.",
          });
          if (saved > 0) invalidateLists();
          return;
        } else {
          setStatus({ kind: "error", message: "Couldn't save photo — try again." });
          if (saved > 0) invalidateLists();
          return;
        }
      }
      if (saved > 0) {
        setStatus({ kind: "saved", count: saved });
        invalidateLists();
      } else {
        setStatus({
          kind: "error",
          message: lastStatus
            ? `Couldn't save photo (HTTP ${lastStatus}).`
            : "Couldn't save photo — try again.",
        });
      }
    },
    [invalidateLists, enqueueAsset],
  );
```

- [ ] **Step 3: Typecheck**

Run: `npm run typecheck`
Expected: no errors.

- [ ] **Step 4: Manual verification (dev build or Expo Go with a dev server)**

1. Connect the app to an instance.
2. Enable airplane mode.
3. Capture → **Choose photo**, pick 2 images.
4. Expect status: "Queued — will sync when you're back online." and the "N waiting to sync" strip increments by 2.
5. Disable airplane mode; on the next foreground/focus the strip drains to 0 and the images appear in Library.

- [ ] **Step 5: Commit**

```bash
git add "app/(tabs)/capture.tsx"
git commit -m "feat(mobile): queue photos offline on network error in Capture"
```

---

### Task 7: iOS share extension — persist to the App Group on network failure

On a network error, write the payload into the App Group container plus a `pendingShares` manifest so the app can drain it, and show an "saved offline" confirmation.

**Files:**
- Modify: `targets/share/ShareViewController.swift`

**Interfaces:**
- Produces: App Group UserDefaults key `pendingShares` — a JSON array of `{ kind: "asset", filename, name, mimeType, createdAt }` or `{ kind: "url"|"note", value, createdAt }`, and image files under `<container>/pending-shares/<uuid>.<ext>`. These are consumed by `drainSharedPending` (Task 4). The `createdAt` is epoch milliseconds (matches the JS `createdAt` ordering).

- [ ] **Step 1: Add a `PendingRecord` enum and thread it through the send path**

Add this enum inside the class, next to `SharedPayload`:

```swift
  private enum PendingRecord {
    case asset(Data, name: String, mimeType: String)
    case url(String)
    case note(String)
  }
```

- [ ] **Step 2: Pass a `PendingRecord` into `postItem` / `uploadImage` / `send`**

Change `save(_:)`'s switch cases to build a pending record:

```swift
    switch payload {
    case .image(let data, let filename, let mimeType):
      uploadImage(data: data, filename: filename, mimeType: mimeType, instanceUrl: instanceUrl, token: token)
    case .url(let url):
      postItem(body: ["url": url.absoluteString], pending: .url(url.absoluteString), instanceUrl: instanceUrl, token: token)
    case .text(let text):
      let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
      guard !trimmed.isEmpty else {
        finish(success: false, message: "Nothing shareable found.")
        return
      }
      if let asURL = URL(string: trimmed), let scheme = asURL.scheme,
         ["http", "https"].contains(scheme.lowercased()) {
        postItem(body: ["url": trimmed], pending: .url(trimmed), instanceUrl: instanceUrl, token: token)
      } else {
        postItem(body: ["note": trimmed], pending: .note(trimmed), instanceUrl: instanceUrl, token: token)
      }
    }
```

Change `postItem` to accept and forward `pending`:

```swift
  private func postItem(body: [String: String], pending: PendingRecord, instanceUrl: String, token: String) {
    guard let endpoint = URL(string: "\(instanceUrl)/api/items"),
          let payload = try? JSONSerialization.data(withJSONObject: body)
    else {
      finish(success: false, message: "Invalid instance URL.")
      return
    }

    var request = URLRequest(url: endpoint)
    request.httpMethod = "POST"
    request.httpBody = payload
    request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
    request.setValue("application/json", forHTTPHeaderField: "content-type")
    request.timeoutInterval = 15
    send(request, pending: pending)
  }
```

Change `uploadImage` to build the pending asset record and forward it (add this line before `send` and change the `send` call):

```swift
    request.timeoutInterval = 45
    send(request, pending: .asset(data, name: filename, mimeType: mimeType))
```

- [ ] **Step 3: Persist on network failure inside `send`**

Replace `send(_ request:)` with:

```swift
  private func send(_ request: URLRequest, pending: PendingRecord) {
    let session = URLSession(configuration: .ephemeral)
    session.dataTask(with: request) { [weak self] _, response, error in
      DispatchQueue.main.async {
        if error != nil {
          self?.persistPending(pending)
          self?.finishOffline()
          return
        }
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        switch status {
        case 201:
          self?.finish(success: true, message: "Saved")
        case 401:
          self?.finish(success: false, message: "Token rejected — reconnect in the app.")
        case 415:
          self?.finish(success: false, message: "That image format isn't supported.")
        default:
          self?.finish(success: false, message: "Save failed (HTTP \(status)).")
        }
      }
    }.resume()
  }
```

- [ ] **Step 4: Add `persistPending`, `fileExtension`, and `finishOffline`**

Add these methods to the class (near `finish`):

```swift
  private func persistPending(_ record: PendingRecord) {
    guard let defaults = UserDefaults(suiteName: appGroup) else { return }
    var manifest = (try? JSONSerialization.jsonObject(
      with: defaults.data(forKey: "pendingShares") ?? Data()
    )) as? [[String: Any]] ?? []
    let createdAt = Date().timeIntervalSince1970 * 1000

    switch record {
    case .asset(let data, let name, let mimeType):
      guard let container = FileManager.default.containerURL(
        forSecurityApplicationGroupIdentifier: appGroup
      ) else { return }
      let dir = container.appendingPathComponent("pending-shares", isDirectory: true)
      try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
      let filename = UUID().uuidString + fileExtension(for: mimeType)
      guard (try? data.write(to: dir.appendingPathComponent(filename))) != nil else { return }
      manifest.append([
        "kind": "asset", "filename": filename, "name": name,
        "mimeType": mimeType, "createdAt": createdAt,
      ])
    case .url(let value):
      manifest.append(["kind": "url", "value": value, "createdAt": createdAt])
    case .note(let value):
      manifest.append(["kind": "note", "value": value, "createdAt": createdAt])
    }

    // Cap at 20 pending shares, dropping the oldest (and their files).
    if manifest.count > 20 {
      let overflow = manifest.count - 20
      if let container = FileManager.default.containerURL(
        forSecurityApplicationGroupIdentifier: appGroup
      ) {
        for old in manifest.prefix(overflow)
        where (old["kind"] as? String) == "asset" {
          if let fn = old["filename"] as? String {
            try? FileManager.default.removeItem(
              at: container.appendingPathComponent("pending-shares").appendingPathComponent(fn)
            )
          }
        }
      }
      manifest = Array(manifest.suffix(20))
    }

    if let data = try? JSONSerialization.data(withJSONObject: manifest) {
      defaults.set(data, forKey: "pendingShares")
    }
  }

  private func fileExtension(for mime: String) -> String {
    switch mime {
    case "image/png": return ".png"
    case "image/gif": return ".gif"
    case "image/webp": return ".webp"
    default: return ".jpg"
    }
  }

  private func finishOffline() {
    setState(title: "Saved offline", detail: "Will sync when you reopen Openmind.", busy: false)
    titleLabel.textColor = green
    DispatchQueue.main.asyncAfter(deadline: .now() + 1.6) { [weak self] in
      self?.extensionContext?.completeRequest(returningItems: [], completionHandler: nil)
    }
  }
```

- [ ] **Step 5: Build the iOS dev client**

Run (from `apps/mobile`): `npx expo run:ios` (or an EAS dev/preview build). A native change requires a fresh build — Expo Go will not include it.
Expected: build succeeds; the share extension "OpenmindShare" installs.

- [ ] **Step 6: Manual verification**

1. In the app, connect to an instance (mirrors `{instanceUrl, token}` into the App Group).
2. Enable airplane mode.
3. From Photos, share an image to Openmind → the extension shows **"Saved offline — Will sync when you reopen Openmind."**
4. Disable airplane mode, foreground the app → the "N waiting to sync" strip appears and drains; the image lands in Library.
5. Repeat with a shared URL and a shared note to confirm those offline paths drain too.

- [ ] **Step 7: Commit**

```bash
git add targets/share/ShareViewController.swift
git commit -m "feat(mobile): iOS share extension saves offline to the App Group on failure"
```

---

## Self-Review

**Spec coverage:**
- Part A persistence (`expo-file-system`, copy into durable dir) → Task 1 (dep) + Task 2 (`asset-store`).
- Part A queue extension (asset entry, `enqueueAsset`, flush branch, cleanup on every removal path) → Task 3.
- Part A context (`enqueueAsset`, pending count reuse) → Task 5.
- Part A Capture screen (queue remaining on network error; Android inherits via `uploadFiles`) → Task 6.
- Part B Swift (container file + manifest on network failure, cap 20, offline copy) → Task 7.
- Part B app drain (read manifest, resolve container via `appleSharedContainers`, enqueue, clear-after-copy idempotency) → Task 4, wired in Task 5.
- Error taxonomy (network retry / 401 stop / permanent-4xx drop) → reused verbatim in Task 3; asset files deleted on drop/success/evict/remove.
- Testing (table-driven queue + drain, cleanup on each removal, mixed flush, cap eviction, manifest parse/idempotency) → Tasks 2, 3, 4. Swift verified manually → Task 7 Steps 5–6.
- Rollout (Part A ships in current build; Part B needs a fresh build) → Task 6 (JS-only) vs Task 7 (native build steps).

No gaps.

**Placeholder scan:** No TBD/TODO; every code step shows complete code; no "similar to Task N" references.

**Type consistency:** `QueuedCapture.asset` shape `{ filePath, name, type }` is consistent across Tasks 3–6. `enqueueAsset(files: AssetUpload[]) => Promise<{ ids: string[] }>` identical in Tasks 3, 5, 6. `drainSharedPending(): Promise<number>` identical in Tasks 4–5. Manifest record shape (`kind`/`filename`/`name`/`mimeType`/`value`/`createdAt`) matches between the Swift writer (Task 7) and the TS parser (Task 4). `copyIntoQueue(sourceUri, id, mime)` and `deleteQueueFile(uri)` identical in Tasks 2–3.
