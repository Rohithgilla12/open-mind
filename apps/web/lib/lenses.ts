import "server-only";
import { apiFetch } from "./api";
import type { Item, Lens, SearchResponse } from "./types";

/** All of the caller's Lenses, newest first. Never throws — [] on failure. */
export async function getLenses(): Promise<Lens[]> {
  try {
    const res = await apiFetch("/lenses");
    if (!res.ok) return [];
    return ((await res.json()) as Lens[]) ?? [];
  } catch {
    return [];
  }
}

/** A single Lens, or null if it does not exist / the API is unreachable. */
export async function getLens(id: string): Promise<Lens | null> {
  try {
    const res = await apiFetch(`/lenses/${id}`);
    if (!res.ok) return null;
    return (await res.json()) as Lens;
  } catch {
    return null;
  }
}

/** The items a Lens currently matches (a live view of its saved rule). */
export async function getLensItems(id: string): Promise<Item[]> {
  try {
    const res = await apiFetch(`/lenses/${id}/items`);
    if (!res.ok) return [];
    const body = (await res.json()) as SearchResponse;
    return (body.results ?? []).map((r) => r.item);
  } catch {
    return [];
  }
}
