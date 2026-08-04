// Hand-written API client for the Openmind web app's /api/* proxy routes.
// Those routes honour an `Authorization: Bearer <token>` header. All requests
// read { instanceUrl, token } from keychain-backed settings unless an
// override is passed (used to validate typed values before they persist).
// The token is a secret and is never logged.
import { getSettings, type Settings } from "./settings";
// Plugin fetch tunnels through Rust — the webview's CORS policy would block
// plain fetch() from the tauri:// origin to any instance URL.
import { fetch } from "@tauri-apps/plugin-http";

/** Cap hung requests so the panel never sits on "Searching…" / "Checking…" forever. */
const REQUEST_TIMEOUT_MS = 12_000;

/** Minimal item shape (subset of the OpenAPI Item schema). */
export type Item = {
  id: string;
  url: string;
  title?: string;
  summary?: string;
  status: string;
  cardType?: string;
  tags?: string[];
  userTags?: string[];
};

export type SearchResult = {
  item: Item;
  score: number;
};

/** Matches OpenAPI UnderstoodQuery — only `text` is shown in the dock panel. */
export type UnderstoodQuery = {
  text?: string;
  color?: string;
  types?: string[];
};

async function resolveSettings(override?: Settings): Promise<Settings | null> {
  return override ?? (await getSettings());
}

function authHeaders(token: string, json = false): HeadersInit {
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` };
  if (json) headers["content-type"] = "application/json";
  return headers;
}

function apiUrl(instanceUrl: string, path: string): string {
  return `${instanceUrl.replace(/\/+$/, "")}${path}`;
}

/**
 * Plugin fetch with a hard deadline. Prefer AbortSignal so in-flight work can
 * be cancelled (search races); fall back to connectTimeout for the Rust client.
 */
async function timedFetch(
  input: string,
  init: RequestInit & { connectTimeout?: number } = {},
  signal?: AbortSignal,
): Promise<Response> {
  const controller = new AbortController();
  const onAbort = () => controller.abort();
  if (signal) {
    if (signal.aborted) {
      throw new DOMException("Aborted", "AbortError");
    }
    signal.addEventListener("abort", onAbort, { once: true });
  }
  const timer = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);
  try {
    return await fetch(input, {
      ...init,
      signal: controller.signal,
      connectTimeout: REQUEST_TIMEOUT_MS,
    });
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener("abort", onAbort);
  }
}

/**
 * Validate a token against GET {instanceUrl}/api/auth/check.
 * Returns the HTTP status (200 valid, 401 invalid); 0 on a network error or
 * when no settings are available.
 */
export async function checkToken(override?: Settings): Promise<number> {
  const settings = await resolveSettings(override);
  if (!settings) return 0;
  try {
    const res = await timedFetch(apiUrl(settings.instanceUrl, "/api/auth/check"), {
      method: "GET",
      headers: authHeaders(settings.token),
    });
    return res.status;
  } catch {
    return 0;
  }
}

/**
 * Save an item via POST {instanceUrl}/api/items. Exactly one of url/note
 * should be provided. A 201 response body is parsed into `item`.
 */
export async function saveItem(
  input: { url?: string; note?: string },
  override?: Settings,
): Promise<{ ok: boolean; status: number; item?: Item }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0 };
  try {
    const res = await timedFetch(apiUrl(settings.instanceUrl, "/api/items"), {
      method: "POST",
      headers: authHeaders(settings.token, true),
      body: JSON.stringify(input),
    });
    let item: Item | undefined;
    if (res.status === 201) {
      try {
        item = (await res.json()) as Item;
      } catch {
        item = undefined;
      }
    }
    return { ok: res.ok, status: res.status, item };
  } catch {
    return { ok: false, status: 0 };
  }
}

/** Trim, uppercase, and reinsert the dash so "abcd efgh" -> "ABCD-EFGH". */
function normaliseDeviceCode(code: string): string {
  const cleaned = code.trim().toUpperCase().replace(/[^A-Z0-9]/g, "");
  if (cleaned.length !== 8) return cleaned;
  return `${cleaned.slice(0, 4)}-${cleaned.slice(4)}`;
}

export type ClaimDeviceCodeResult =
  | { ok: true; status: number; key: string; name: string }
  | { ok: false; status: number };

/**
 * Redeem a device-connect code for a fresh API key via
 * POST {instanceUrl}/api/device-links/claim. Unauthenticated — the code
 * itself is the credential, so no Bearer header is sent. Network failures
 * surface as status 0. The returned key is never logged.
 */
export async function claimDeviceCode(
  instanceUrl: string,
  code: string,
  deviceName: string,
): Promise<ClaimDeviceCodeResult> {
  try {
    const res = await timedFetch(apiUrl(instanceUrl, "/api/device-links/claim"), {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ code: normaliseDeviceCode(code), deviceName }),
    });
    if (res.status === 201) {
      try {
        const data = (await res.json()) as { key: string; name: string };
        return { ok: true, status: res.status, key: data.key, name: data.name };
      } catch {
        return { ok: false, status: res.status };
      }
    }
    return { ok: false, status: res.status };
  } catch {
    return { ok: false, status: 0 };
  }
}

function parseUnderstood(raw: unknown): string | undefined {
  if (raw == null) return undefined;
  // Contract is UnderstoodQuery { text?, color?, types? }. Older/buggy
  // responses may still send a bare string — accept both, never return an object.
  if (typeof raw === "string") {
    const t = raw.trim();
    return t || undefined;
  }
  if (typeof raw === "object" && "text" in raw) {
    const text = (raw as UnderstoodQuery).text;
    if (typeof text === "string") {
      const t = text.trim();
      return t || undefined;
    }
  }
  return undefined;
}

/**
 * Search via GET {instanceUrl}/api/search?q=<q>&parse=true. Returns ranked
 * results and, when the query parser rewrote the input, an `understood`
 * text echo. Pass `signal` so the panel can cancel superseded searches.
 * Network failures / timeouts / aborts surface as status 0 (aborts rethrow
 * so callers can ignore them).
 */
export async function searchItems(
  q: string,
  override?: Settings,
  signal?: AbortSignal,
): Promise<{ ok: boolean; status: number; results: SearchResult[]; understood?: string }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0, results: [] };
  try {
    const res = await timedFetch(
      apiUrl(settings.instanceUrl, `/api/search?q=${encodeURIComponent(q)}&parse=true`),
      {
        method: "GET",
        headers: authHeaders(settings.token),
      },
      signal,
    );
    if (!res.ok) {
      return { ok: false, status: res.status, results: [] };
    }
    try {
      const data = (await res.json()) as { results?: SearchResult[]; understood?: unknown };
      return {
        ok: true,
        status: res.status,
        results: Array.isArray(data.results) ? data.results : [],
        understood: parseUnderstood(data.understood),
      };
    } catch {
      return { ok: true, status: res.status, results: [] };
    }
  } catch (err) {
    // Caller cancelled this search — don't paint an error for a stale request.
    if (signal?.aborted) {
      throw err instanceof Error ? err : new DOMException("Aborted", "AbortError");
    }
    // Own deadline / network failure → unreachable.
    return { ok: false, status: 0, results: [] };
  }
}

/**
 * Read an item list out of either shape: the ItemPage envelope, or the bare
 * array served by an instance predating it. Returns null when the body is
 * neither, so callers can report a failure instead of an empty list — the two
 * used to be indistinguishable.
 */
function readItemList(data: unknown): Item[] | null {
  if (Array.isArray(data)) return data as Item[];
  if (data && typeof data === "object" && Array.isArray((data as { items?: unknown }).items)) {
    return (data as { items: Item[] }).items;
  }
  return null;
}

async function listItemsFrom(
  path: string,
  override?: Settings,
  signal?: AbortSignal,
): Promise<{ ok: boolean; status: number; items: Item[] }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0, items: [] };
  try {
    const res = await timedFetch(apiUrl(settings.instanceUrl, path), {
      method: "GET",
      headers: authHeaders(settings.token),
    }, signal);
    if (!res.ok) {
      return { ok: false, status: res.status, items: [] };
    }
    try {
      const items = readItemList(await res.json());
      if (items === null) {
        console.error("unrecognised item list body", { path });
        return { ok: false, status: res.status, items: [] };
      }
      return { ok: true, status: res.status, items };
    } catch {
      return { ok: false, status: res.status, items: [] };
    }
  } catch (err) {
    if (signal?.aborted) {
      throw err instanceof Error ? err : new DOMException("Aborted", "AbortError");
    }
    return { ok: false, status: 0, items: [] };
  }
}

/**
 * Set an item's user tags via PATCH {instanceUrl}/api/items/{id}. Throws on
 * a missing instance/token or a non-2xx response — callers surface the
 * failure inline (e.g. the confirm strip keeps the tag input on error).
 */
export async function setUserTags(itemId: string, tags: string[], override?: Settings): Promise<void> {
  const settings = await resolveSettings(override);
  if (!settings) throw new Error("No settings configured");
  const res = await timedFetch(apiUrl(settings.instanceUrl, `/api/items/${itemId}`), {
    method: "PATCH",
    headers: authHeaders(settings.token, true),
    body: JSON.stringify({ userTags: tags }),
  });
  if (!res.ok) {
    throw new Error(`Request failed (${res.status})`);
  }
}

/** Desk pins via GET {instanceUrl}/api/desk (newest-pinned first). */
export function listDesk(
  override?: Settings,
  signal?: AbortSignal,
): Promise<{ ok: boolean; status: number; items: Item[] }> {
  return listItemsFrom("/api/desk", override, signal);
}

/** Recent library items via GET {instanceUrl}/api/items?limit=. */
export function listRecent(
  limit: number,
  override?: Settings,
  signal?: AbortSignal,
): Promise<{ ok: boolean; status: number; items: Item[] }> {
  const capped = Number.isFinite(limit) && limit > 0 ? Math.floor(limit) : 8;
  return listItemsFrom(`/api/items?limit=${capped}`, override, signal);
}
