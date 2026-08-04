import { getSettings } from "./storage";

export interface SaveBody {
  url?: string;
  note?: string;
}

/** Minimal shape of an item returned by the API. */
export interface Item {
  id: string;
  url: string;
  title?: string;
  summary?: string;
  status: string;
  userTags?: string[];
  createdAt?: string;
}

export interface SaveResult {
  ok: boolean;
  status: number;
  item?: Item;
}

export interface PatchResult {
  ok: boolean;
  status: number;
}

export interface RecentResult {
  ok: boolean;
  status: number;
  items: Item[];
}

function normaliseUrl(instanceUrl: string): string {
  return instanceUrl.replace(/\/+$/, "");
}

/**
 * POST a new item to the configured instance. On 201 the response body is
 * parsed into `item`. Returns the HTTP status and whether the request
 * succeeded. Network failures surface as status 0.
 */
export async function saveItem(body: SaveBody): Promise<SaveResult> {
  const { instanceUrl, token } = await getSettings();
  try {
    const res = await fetch(`${normaliseUrl(instanceUrl)}/api/items`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(body),
    });
    if (res.status === 201) {
      try {
        const item = (await res.json()) as Item;
        return { ok: res.ok, status: res.status, item };
      } catch {
        return { ok: res.ok, status: res.status };
      }
    }
    return { ok: res.ok, status: res.status };
  } catch {
    return { ok: false, status: 0 };
  }
}

/**
 * PATCH the full user-tag list of an item. Callers pass the complete desired
 * list each time (server replaces, not merges). Network failures surface as
 * status 0.
 */
export async function patchUserTags(
  id: string,
  userTags: string[],
): Promise<PatchResult> {
  const { instanceUrl, token } = await getSettings();
  try {
    const res = await fetch(
      `${normaliseUrl(instanceUrl)}/api/items/${id}`,
      {
        method: "PATCH",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ userTags }),
      },
    );
    return { ok: res.ok, status: res.status };
  } catch {
    return { ok: false, status: 0 };
  }
}

/**
 * GET the most recent items (up to `limit`). Network failures surface as
 * status 0 with an empty list.
 */
export async function recentItems(limit: number): Promise<RecentResult> {
  const { instanceUrl, token } = await getSettings();
  try {
    const res = await fetch(
      `${normaliseUrl(instanceUrl)}/api/items?limit=${limit}`,
      {
        method: "GET",
        headers: {
          Authorization: `Bearer ${token}`,
        },
      },
    );
    if (!res.ok) {
      return { ok: false, status: res.status, items: [] };
    }
    try {
      const parsed = (await res.json()) as unknown;
      const items = Array.isArray(parsed)
        ? (parsed as Item[])
        : parsed && typeof parsed === "object" && Array.isArray((parsed as { items?: unknown }).items)
          ? ((parsed as { items: Item[] }).items)
          : null;
      if (items === null) {
        console.error("unrecognised item list body");
        return { ok: false, status: res.status, items: [] };
      }
      return { ok: true, status: res.status, items };
    } catch {
      // A 200 with unparseable JSON is a real failure, not an empty library
      // (see apps/mobile/lib/api.ts's listFeedItems for the same reasoning).
      return { ok: false, status: res.status, items: [] };
    }
  } catch {
    return { ok: false, status: 0, items: [] };
  }
}

export interface ClaimResult {
  ok: boolean;
  status: number;
  key?: string;
  name?: string;
}

/** Trim, uppercase, and reinsert the dash so "abcd efgh" -> "ABCD-EFGH". */
function normaliseDeviceCode(code: string): string {
  const cleaned = code.trim().toUpperCase().replace(/[^A-Z0-9]/g, "");
  if (cleaned.length !== 8) return cleaned;
  return `${cleaned.slice(0, 4)}-${cleaned.slice(4)}`;
}

/**
 * Redeem a device-connect code for a fresh API key via
 * POST {instanceUrl}/api/device-links/claim. Unauthenticated — the code
 * itself is the credential. Network failures surface as status 0. The
 * returned key is never logged.
 */
export async function claimDeviceCode(
  instanceUrl: string,
  code: string,
  deviceName: string,
): Promise<ClaimResult> {
  try {
    const res = await fetch(
      `${normaliseUrl(instanceUrl)}/api/device-links/claim`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          code: normaliseDeviceCode(code),
          deviceName,
        }),
      },
    );
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
 * Validate the configured token against the instance. Returns the HTTP status
 * of GET /api/auth/check, or 0 when the instance is unreachable.
 */
export async function checkToken(): Promise<number> {
  const { instanceUrl, token } = await getSettings();
  try {
    const res = await fetch(`${normaliseUrl(instanceUrl)}/api/auth/check`, {
      method: "GET",
      headers: {
        Authorization: `Bearer ${token}`,
      },
    });
    return res.status;
  } catch {
    return 0;
  }
}
