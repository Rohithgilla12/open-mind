// Hand-written API client for the Openmind web app's /api/* proxy routes.
// Those routes honour an `Authorization: Bearer <token>` header. All requests
// read { instanceUrl, token } from secure-store settings. The token is a secret
// and is never logged.
import { getSettings, type Settings } from "./settings";

/** Minimal item shape (subset of the OpenAPI Item schema). */
/** Item plus the full archived body (detail endpoint). */
export type ItemDetail = Item & { body?: string };

export type Item = {
  id: string;
  url: string;
  title?: string;
  summary?: string;
  status: string;
  cardType?: string;
  createdAt?: string;
  palette?: string[];
  leadImageUrl?: string;
  tags?: string[];
  userTags?: string[];
};

async function resolveSettings(override?: Settings): Promise<Settings | null> {
  return override ?? (await getSettings());
}

function authHeaders(token: string, json = false): HeadersInit {
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` };
  if (json) headers["content-type"] = "application/json";
  return headers;
}

/**
 * Validate a token against GET {instanceUrl}/api/auth/check.
 * Returns the HTTP status (200 valid, 401 invalid); 0 on a network error or
 * when no settings are available. Pass `override` to validate typed values
 * before they are persisted.
 */
export async function checkToken(override?: Settings): Promise<number> {
  const settings = await resolveSettings(override);
  if (!settings) return 0;
  try {
    const res = await fetch(`${settings.instanceUrl}/api/auth/check`, {
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
    const res = await fetch(`${settings.instanceUrl}/api/items`, {
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

/**
 * Normalise a device-connect code for submission: trim, uppercase, and
 * reinsert the dash if the user typed it without one (e.g. "abcdefgh" ->
 * "ABCD-EFGH"). Anything else is passed through unchanged so the server can
 * reject malformed input itself.
 */
export function normalizeDeviceCode(input: string): string {
  const upper = input.trim().toUpperCase();
  if (!upper.includes("-") && upper.length === 8) {
    return `${upper.slice(0, 4)}-${upper.slice(4)}`;
  }
  return upper;
}

/**
 * Claim a device-connect code via POST {instanceUrl}/api/device-links/claim.
 * Unauthenticated — the code itself is the credential. On success (201) the
 * response carries a freshly minted API key, which is a secret and is never
 * logged. A wrong/expired/used code and a rate limit both come back as plain
 * HTTP statuses (404 / 429) for the caller to interpret.
 */
export async function claimDeviceCode(
  instanceUrl: string,
  code: string,
  deviceName: string,
): Promise<{ ok: boolean; status: number; key?: string }> {
  const url = instanceUrl.trim().replace(/\/+$/, "");
  if (!url) return { ok: false, status: 0 };
  try {
    const res = await fetch(`${url}/api/device-links/claim`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ code: normalizeDeviceCode(code), deviceName }),
    });
    let key: string | undefined;
    if (res.status === 201) {
      try {
        const data = (await res.json()) as { key?: string };
        key = data.key;
      } catch {
        key = undefined;
      }
    }
    return { ok: res.status === 201 && !!key, status: res.status, key };
  } catch {
    return { ok: false, status: 0 };
  }
}

/**
 * List items via GET {instanceUrl}/api/items?limit=. Returns an array of items
 * (empty on error).
 */
export async function listItems(
  limit = 50,
  override?: Settings,
): Promise<{ ok: boolean; status: number; items: Item[] }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0, items: [] };
  try {
    const res = await fetch(`${settings.instanceUrl}/api/items?limit=${limit}`, {
      method: "GET",
      headers: authHeaders(settings.token),
    });
    let items: Item[] = [];
    if (res.ok) {
      try {
        const data = (await res.json()) as unknown;
        if (Array.isArray(data)) {
          items = data as Item[];
        } else if (data && Array.isArray((data as { items?: Item[] }).items)) {
          items = (data as { items: Item[] }).items;
        }
      } catch {
        items = [];
      }
    }
    return { ok: res.ok, status: res.status, items };
  } catch {
    return { ok: false, status: 0, items: [] };
  }
}

/**
 * Fetch one item's full detail (including the archived body) via
 * GET {instanceUrl}/api/items/{id}.
 */
export async function getItem(
  id: string,
  override?: Settings,
): Promise<{ ok: boolean; status: number; item?: ItemDetail }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0 };
  try {
    const res = await fetch(`${settings.instanceUrl}/api/items/${id}`, {
      method: "GET",
      headers: authHeaders(settings.token),
    });
    let item: ItemDetail | undefined;
    if (res.ok) {
      try {
        item = (await res.json()) as ItemDetail;
      } catch {
        item = undefined;
      }
    }
    return { ok: res.ok, status: res.status, item };
  } catch {
    return { ok: false, status: 0 };
  }
}
