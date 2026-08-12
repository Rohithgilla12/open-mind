import type { QueuedCapture } from "./queue";
import { host } from "./url";

/** Attempts at which an entry stops reading as "pending" and starts reading
 *  as "stuck" — the strip switches to the danger colour here. */
export const STUCK_ATTEMPTS = 5;

const NOTE_EXCERPT = 48;

export function pendingSummary(items: QueuedCapture[]): { label: string; stuck: boolean } {
  const label =
    items.length === 1 ? "1 save waiting to sync" : `${items.length} saves waiting to sync`;
  return { label, stuck: items.some((q) => q.attempts >= STUCK_ATTEMPTS) };
}

export function entryLabel(entry: QueuedCapture): string {
  if (entry.url) return host(entry.url);
  const note = entry.note?.trim();
  if (!note) return "Untitled save";
  return note.length > NOTE_EXCERPT ? `${note.slice(0, NOTE_EXCERPT)}…` : note;
}

export function relativeAge(createdAt: number, now: number): string {
  const ms = Math.max(0, now - createdAt);
  const minutes = Math.floor(ms / 60_000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}
