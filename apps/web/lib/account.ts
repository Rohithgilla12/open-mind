import "server-only";
import { apiFetch } from "./api";
import type { Account } from "./types";

/**
 * The caller's account facts, or null if the API is unreachable. Never throws:
 * the sidebar renders on every page, so a failure here must degrade to a
 * quieter sidebar rather than take down the whole app.
 */
export async function getAccount(): Promise<Account | null> {
  try {
    const res = await apiFetch("/account");
    if (!res.ok) return null;
    return (await res.json()) as Account;
  } catch {
    return null;
  }
}

/**
 * Human-readable byte size. Uses SI units (kB/MB/GB) to match the decimal
 * divisor, rather than labelling 1024-based values with SI prefixes.
 */
export function formatBytes(bytes: number): string {
  if (bytes <= 0) return "0 kB";
  const units = ["B", "kB", "MB", "GB", "TB"];
  const i = Math.min(
    units.length - 1,
    Math.floor(Math.log(bytes) / Math.log(1000)),
  );
  const value = bytes / 1000 ** i;
  // One decimal place once we're past kB, where the extra digit carries signal.
  const rounded = i >= 2 ? value.toFixed(1) : Math.round(value).toString();
  return `${rounded} ${units[i]}`;
}

// Identity rendering deliberately lives in the account-row components rather
// than here: Clerk mode reads the name and avatar from Clerk's own client hooks
// (so /account's email is only a fallback), and token mode has no identity at
// all. See ClerkAccountRow / TokenAccountRow.
