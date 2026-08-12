import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";

export type QueuedCapture = {
  id: string;
  url?: string;
  note?: string;
  createdAt: number;
  attempts: number;
  lastError?: string;
};

export type EnqueueResult = { id: string; deduped: boolean; dropped: number; persisted: boolean };

/** Current queue. Returns [] rather than throwing — the strip is chrome and
 *  must never take the panel down with it. */
export async function listQueue(): Promise<QueuedCapture[]> {
  try {
    return await invoke<QueuedCapture[]>("queue_list");
  } catch {
    return [];
  }
}

export function enqueueCapture(input: { url?: string; note?: string }): Promise<EnqueueResult> {
  return invoke<EnqueueResult>("queue_enqueue", { url: input.url, note: input.note });
}

/** Asks Rust to retry now. Failures are ignored: the periodic drainer will
 *  try again regardless. */
export async function flushQueue(): Promise<void> {
  try {
    await invoke("queue_flush");
  } catch {
    // Ignored by design.
  }
}

export async function removeQueued(id: string): Promise<void> {
  try {
    await invoke("queue_remove", { id });
  } catch {
    // Ignored by design.
  }
}

/** Subscribes to queue mutations emitted by Rust. Resolves to an unlisten fn. */
export function subscribeQueue(
  cb: (items: QueuedCapture[]) => void,
): Promise<() => void> {
  return listen<QueuedCapture[]>("queue-changed", (event) => cb(event.payload));
}
