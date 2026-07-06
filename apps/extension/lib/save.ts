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
      return { ok: true, status: res.status, items: Array.isArray(parsed) ? (parsed as Item[]) : [] };
    } catch {
      return { ok: true, status: res.status, items: [] };
    }
  } catch {
    return { ok: false, status: 0, items: [] };
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
