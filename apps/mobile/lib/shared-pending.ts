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
      if (typeof rec.createdAt !== "number") return false;
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

let draining: Promise<number> | null = null;

/**
 * Move every pending share into the JS queue, oldest-first. Each record is
 * cleared from the manifest only after its content is safely enqueued (or dropped with a warning if its container file is missing), so a
 * crash mid-drain leaves un-drained records intact rather than double-saving.
 * Returns the number of records drained. Concurrent calls coalesce onto the
 * same in-flight run so overlapping foreground events cannot double-enqueue.
 */
export function drainSharedPending(): Promise<number> {
  if (draining) return draining;
  draining = doDrain().finally(() => {
    draining = null;
  });
  return draining;
}

async function doDrain(): Promise<number> {
  const storage = extensionStorage();
  if (!storage) return 0;
  const records = parseManifest(storage.get(MANIFEST_KEY));
  if (records.length === 0) return 0;

  const container = containerDir();
  let remaining = [...records];
  let drained = 0;

  for (const rec of [...records].sort((a, b) => a.createdAt - b.createdAt)) {
    try {
      let enqueued = true;
      if (rec.kind === "asset") {
        if (!container) continue; // unresolved container, retry this record next foreground
        const src = new File(container, CONTAINER_SUBDIR, rec.filename);
        if (src.exists) {
          await enqueueAsset([{ uri: src.uri, name: rec.name, type: rec.mimeType }]);
          src.delete();
        } else {
          console.warn(`[shared-pending] container file missing, dropping record: ${rec.filename}`);
          enqueued = false;
        }
      } else {
        await enqueue(rec.kind === "url" ? { url: rec.value } : { note: rec.value });
      }
      remaining = remaining.filter((r) => r !== rec);
      storage.set(MANIFEST_KEY, remaining.length ? remaining : undefined);
      if (enqueued) drained += 1;
    } catch (err) {
      console.warn("[shared-pending] drain stopped", err);
      break;
    }
  }
  return drained;
}
