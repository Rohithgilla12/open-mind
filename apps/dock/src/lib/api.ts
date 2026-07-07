// Hand-written API client for the Openmind web app's /api/* proxy routes.
// Those routes honour an `Authorization: Bearer <token>` header. All requests
// read { instanceUrl, token } from keychain-backed settings unless an
// override is passed (used to validate typed values before they persist).
// The token is a secret and is never logged.
import { getSettings, type Settings } from "./settings";

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
 * Validate a token against GET {instanceUrl}/api/auth/check.
 * Returns the HTTP status (200 valid, 401 invalid); 0 on a network error or
 * when no settings are available.
 */
export async function checkToken(override?: Settings): Promise<number> {
  const settings = await resolveSettings(override);
  if (!settings) return 0;
  try {
    const res = await fetch(apiUrl(settings.instanceUrl, "/api/auth/check"), {
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
    const res = await fetch(apiUrl(settings.instanceUrl, "/api/items"), {
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
    const res = await fetch(apiUrl(instanceUrl, "/api/device-links/claim"), {
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

/**
 * Search via GET {instanceUrl}/api/search?q=<q>&parse=true. Returns ranked
 * results and, when the query parser rewrote the input, an `understood`
 * echo. Network failures surface as status 0 with an empty result list.
 */
export async function searchItems(
  q: string,
  override?: Settings,
): Promise<{ ok: boolean; status: number; results: SearchResult[]; understood?: string }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0, results: [] };
  try {
    const res = await fetch(
      apiUrl(settings.instanceUrl, `/api/search?q=${encodeURIComponent(q)}&parse=true`),
      {
        method: "GET",
        headers: authHeaders(settings.token),
      },
    );
    if (!res.ok) {
      return { ok: false, status: res.status, results: [] };
    }
    try {
      const data = (await res.json()) as { results?: SearchResult[]; understood?: string };
      return {
        ok: true,
        status: res.status,
        results: Array.isArray(data.results) ? data.results : [],
        understood: data.understood,
      };
    } catch {
      return { ok: true, status: res.status, results: [] };
    }
  } catch {
    return { ok: false, status: 0, results: [] };
  }
}
