import "server-only";
import { apiFetch } from "./api";
import type { DriftResponse } from "./types";

/**
 * Today's Drift batch: up to 5 resurfacing candidates plus the total candidate
 * count (for the "n of total today" line). Never throws — an empty batch on
 * failure keeps the screen calm rather than erroring.
 */
export async function getDrift(): Promise<DriftResponse> {
  try {
    const res = await apiFetch("/drift");
    if (!res.ok) return { items: [], total: 0 };
    return ((await res.json()) as DriftResponse) ?? { items: [], total: 0 };
  } catch {
    return { items: [], total: 0 };
  }
}
