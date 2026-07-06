import "server-only";
import { apiFetch } from "./api";
import type { Item } from "./types";

/** The caller's Desk: pinned items ordered newest-pinned first. Never throws — [] on failure. */
export async function getDesk(): Promise<Item[]> {
  try {
    const res = await apiFetch("/desk");
    if (!res.ok) return [];
    return ((await res.json()) as Item[]) ?? [];
  } catch {
    return [];
  }
}
