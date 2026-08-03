import "server-only";
import { apiFetch } from "./api";
import type { Feed } from "./types";

/** The caller's subscriptions. Never throws — [] on failure, so a page that
 *  only needs them for chrome (the Feed river's filter strip) still renders. */
export async function getFeeds(): Promise<Feed[]> {
  try {
    const res = await apiFetch("/feeds");
    if (!res.ok) return [];
    return ((await res.json()) as Feed[]) ?? [];
  } catch {
    return [];
  }
}
