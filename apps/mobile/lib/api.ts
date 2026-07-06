// Hand-written API client for the Openmind web app's /api/* proxy routes.
// Those routes honour an `Authorization: Bearer <token>` header. All requests
// read { instanceUrl, token } from secure-store settings. The token is a secret
// and is never logged.
import { getSettings, type Settings } from "./settings";

/** Minimal item shape (subset of the OpenAPI Item schema). */
export type Item = {
  id: string;
  url: string;
  title?: string;
  summary?: string;
  status: string;
  cardType?: string;
  createdAt?: string;
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
