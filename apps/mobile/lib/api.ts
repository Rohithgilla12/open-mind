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
  keptAt?: string | null;
  /** When pinned to the Desk; null/undefined if not pinned. */
  pinnedAt?: string | null;
  /** Feed this item originated from; null if not feed-sourced. */
  feedId?: string | null;
};

/** Thrown by list helpers when the HTTP call fails — status 0 = network. */
export class ApiError extends Error {
  status: number;
  constructor(status: number, message?: string) {
    super(message ?? `Request failed (${status})`);
    this.name = "ApiError";
    this.status = status;
  }
}

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

/** Local file descriptor for multipart upload to POST /api/assets. */
export type AssetUpload = {
  uri: string;
  name: string;
  type: string;
};

/**
 * Upload an image (or PDF) via POST {instanceUrl}/api/assets. The server
 * sniffs content-type, strips image metadata, creates an image card, and
 * queues enrichment. Do not set Content-Type — fetch must supply the
 * multipart boundary. Online-only for now (no offline queue for blobs).
 */
export async function uploadAsset(
  file: AssetUpload,
  override?: Settings,
): Promise<{ ok: boolean; status: number; item?: Item }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0 };
  try {
    const form = new FormData();
    // React Native's FormData accepts { uri, name, type } for file parts.
    form.append("file", {
      uri: file.uri,
      name: file.name,
      type: file.type,
    } as unknown as Blob);
    const res = await fetch(`${settings.instanceUrl}/api/assets`, {
      method: "POST",
      headers: authHeaders(settings.token),
      body: form,
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

/** One page of items, plus the cursor for the next page (absent at the end). */
export type ItemPageResult = { ok: boolean; status: number; items: Item[]; nextCursor?: string };

/**
 * Read a list body in either shape: the ItemPage envelope, or the bare array
 * served by an instance predating it. That compatibility path is deliberate:
 * a self-hosted instance running an older server has no cursor to give, so
 * pagination simply stops after page one instead of the screen breaking.
 *
 * Returns null when the body is neither, so callers report a failure rather
 * than an empty library — the two must not look the same.
 */
export function readItemPage(data: unknown): { items: Item[]; nextCursor?: string } | null {
  if (Array.isArray(data)) return { items: data as Item[], nextCursor: undefined };
  if (data && typeof data === "object") {
    const obj = data as { items?: unknown; nextCursor?: unknown };
    if (Array.isArray(obj.items)) {
      return {
        items: obj.items as Item[],
        // An empty string is treated as absent: TanStack v5's getNextPageParam
        // only checks for null, so a server that ever emitted "" would send
        // mobile into an infinite refetch-page-1-and-append-duplicates loop.
        nextCursor: typeof obj.nextCursor === "string" && obj.nextCursor ? obj.nextCursor : undefined,
      };
    }
  }
  return null;
}

/** List items via GET {instanceUrl}/api/items?limit=&cursor=. */
export async function listItems(
  limit = 50,
  cursor?: string,
  override?: Settings,
): Promise<ItemPageResult> {
  return fetchItemPage("/api/items", { limit, cursor }, override);
}

/** Feed-originated items via GET {instanceUrl}/api/feed?limit=&cursor=, newest first. */
export async function listFeedItems(
  limit = 50,
  cursor?: string,
  override?: Settings,
): Promise<ItemPageResult> {
  return fetchItemPage("/api/feed", { limit, cursor }, override);
}

async function fetchItemPage(
  path: string,
  query: { limit: number; cursor?: string },
  override?: Settings,
): Promise<ItemPageResult> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0, items: [] };
  const params = new URLSearchParams({ limit: String(query.limit) });
  if (query.cursor) params.set("cursor", query.cursor);
  try {
    const res = await fetch(`${settings.instanceUrl}${path}?${params.toString()}`, {
      method: "GET",
      headers: authHeaders(settings.token),
    });
    if (!res.ok) return { ok: false, status: res.status, items: [] };
    try {
      const page = readItemPage(await res.json());
      if (!page) {
        // A 200 we cannot read is a real failure, not an empty list: surface
        // it as an error so the UI shows a failure state instead of silently
        // coercing to an empty, seemingly-healthy list.
        console.error("unrecognised item list body", { path });
        return { ok: false, status: res.status, items: [] };
      }
      return { ok: true, status: res.status, items: page.items, nextCursor: page.nextCursor };
    } catch (err) {
      console.error(err);
      return { ok: false, status: res.status, items: [] };
    }
  } catch (err) {
    console.error(err);
    return { ok: false, status: 0, items: [] };
  }
}

/**
 * Set an item's kept state via PATCH {instanceUrl}/api/items/{id} {kept}.
 * Keeping pins the feed item into the library independent of its feed;
 * passing false unkeeps it again.
 */
export async function setKept(
  itemId: string,
  kept: boolean,
  override?: Settings,
): Promise<{ ok: boolean; status: number; item?: Item }> {
  return patchItem(itemId, { kept }, override);
}

/**
 * Pin or unpin an item on the Desk via PATCH {instanceUrl}/api/items/{id} {pinned}.
 */
export async function setPinned(
  itemId: string,
  pinned: boolean,
  override?: Settings,
): Promise<{ ok: boolean; status: number; item?: Item }> {
  return patchItem(itemId, { pinned }, override);
}

async function patchItem(
  itemId: string,
  body: { kept?: boolean; pinned?: boolean },
  override?: Settings,
): Promise<{ ok: boolean; status: number; item?: Item }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0 };
  try {
    const res = await fetch(`${settings.instanceUrl}/api/items/${itemId}`, {
      method: "PATCH",
      headers: authHeaders(settings.token, true),
      body: JSON.stringify(body),
    });
    let item: Item | undefined;
    if (res.ok) {
      try {
        item = (await res.json()) as Item;
      } catch {
        item = undefined;
      }
    }
    return { ok: res.ok, status: res.status, item };
  } catch (err) {
    console.error(err);
    return { ok: false, status: 0 };
  }
}

/**
 * Desk pins via GET {instanceUrl}/api/desk — newest-pinned first.
 */
export async function listDesk(
  override?: Settings,
): Promise<{ ok: boolean; status: number; items: Item[] }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0, items: [] };
  try {
    const res = await fetch(`${settings.instanceUrl}/api/desk`, {
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

/**
 * Delete an item via DELETE {instanceUrl}/api/items/{id}. Permanent — the
 * server returns 204 on success.
 */
export async function deleteItem(
  id: string,
  override?: Settings,
): Promise<{ ok: boolean; status: number }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0 };
  try {
    const res = await fetch(`${settings.instanceUrl}/api/items/${id}`, {
      method: "DELETE",
      headers: authHeaders(settings.token),
    });
    return { ok: res.status === 204, status: res.status };
  } catch {
    return { ok: false, status: 0 };
  }
}

/**
 * Send an item to the user's Kindle via POST {instanceUrl}/api/items/{id}/kindle.
 * 202 = queued; 409 = Send-to-Kindle not configured (SMTP or Kindle address
 * missing); 422 = item has no archived body to send.
 */
export async function sendItemToKindle(
  id: string,
  override?: Settings,
): Promise<{ ok: boolean; status: number }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0 };
  try {
    const res = await fetch(`${settings.instanceUrl}/api/items/${id}/kindle`, {
      method: "POST",
      headers: authHeaders(settings.token),
    });
    return { ok: res.status === 202, status: res.status };
  } catch {
    return { ok: false, status: 0 };
  }
}

/** A place the pipeline extracted from an item (see GET /items/{id}/places). */
export type Place = {
  id: string;
  name: string;
  hint: string;
  address: string;
  lat?: number;
  lng?: number;
  source: string;
};

export type PlaceWithItem = Place & {
  itemId: string;
  itemTitle: string;
  itemCardType: string;
};

/** Subset of OpenAPI UnderstoodQuery — what parse=true returns alongside hits. */
export type UnderstoodQuery = {
  text?: string;
  color?: string;
  types?: string[];
};

export type SearchHit = { item: Item; score: number };

/**
 * Hybrid search via GET {instanceUrl}/api/search. Pass parse=true to match the
 * web app (NL → text + colour + types when an AI provider is configured).
 */
export async function searchItems(
  params: { q?: string; color?: string; parse?: boolean },
  override?: Settings,
): Promise<{
  ok: boolean;
  status: number;
  results: SearchHit[];
  understood?: UnderstoodQuery;
}> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0, results: [] };
  const qs = new URLSearchParams();
  if (params.q) qs.set("q", params.q);
  if (params.color) qs.set("color", params.color);
  if (params.parse) qs.set("parse", "true");
  try {
    const res = await fetch(`${settings.instanceUrl}/api/search?${qs.toString()}`, {
      method: "GET",
      headers: authHeaders(settings.token),
    });
    let results: SearchHit[] = [];
    let understood: UnderstoodQuery | undefined;
    if (res.ok) {
      try {
        const data = (await res.json()) as {
          results?: SearchHit[];
          understood?: UnderstoodQuery;
        };
        if (Array.isArray(data.results)) results = data.results;
        if (data.understood) understood = data.understood;
      } catch {
        results = [];
      }
    }
    return { ok: res.ok, status: res.status, results, understood };
  } catch {
    return { ok: false, status: 0, results: [] };
  }
}

/**
 * Places extracted from one item via GET {instanceUrl}/api/items/{id}/places.
 */
export async function getItemPlaces(
  id: string,
  override?: Settings,
): Promise<{ ok: boolean; status: number; places: Place[] }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0, places: [] };
  try {
    const res = await fetch(`${settings.instanceUrl}/api/items/${id}/places`, {
      method: "GET",
      headers: authHeaders(settings.token),
    });
    let places: Place[] = [];
    if (res.ok) {
      try {
        const data = (await res.json()) as unknown;
        if (Array.isArray(data)) places = data as Place[];
      } catch {
        places = [];
      }
    }
    return { ok: res.ok, status: res.status, places };
  } catch {
    return { ok: false, status: 0, places: [] };
  }
}

/**
 * Drop one extracted place via DELETE {instanceUrl}/api/items/{id}/places/{placeId}.
 *
 * Only 204 counts as success. A 404 is deliberately NOT folded in: this app
 * ships through the stores while instances are self-hosted, so it routinely
 * runs ahead of the server, and an instance without this endpoint 404s the
 * whole unregistered route. Treating that as success would make removals look
 * like they worked and then silently reappear. `status: 0` means the request
 * never went out — no instance configured, or the fetch threw.
 */
export async function deleteItemPlace(
  id: string,
  placeId: string,
  override?: Settings,
): Promise<{ ok: boolean; status: number }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0 };
  try {
    const res = await fetch(`${settings.instanceUrl}/api/items/${id}/places/${placeId}`, {
      method: "DELETE",
      headers: authHeaders(settings.token),
    });
    return { ok: res.status === 204, status: res.status };
  } catch {
    return { ok: false, status: 0 };
  }
}

/**
 * All of the user's places via GET {instanceUrl}/api/places.
 */
export async function listPlaces(
  override?: Settings,
): Promise<{ ok: boolean; status: number; places: PlaceWithItem[] }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0, places: [] };
  try {
    const res = await fetch(`${settings.instanceUrl}/api/places`, {
      method: "GET",
      headers: authHeaders(settings.token),
    });
    let places: PlaceWithItem[] = [];
    if (res.ok) {
      try {
        const data = (await res.json()) as unknown;
        if (Array.isArray(data)) places = data as PlaceWithItem[];
      } catch {
        places = [];
      }
    }
    return { ok: res.ok, status: res.status, places };
  } catch {
    return { ok: false, status: 0, places: [] };
  }
}

/**
 * Mint a long-lived API key using a Clerk session JWT, via
 * POST {instanceUrl}/api/api-keys. Used right after native Clerk sign-in so
 * the app can store an omk_ key and use it for every subsequent request (no
 * Clerk-session refresh). The returned key is a secret and is never logged.
 */
export async function mintDeviceKey(
  instanceUrl: string,
  clerkToken: string,
  name: string,
): Promise<{ ok: boolean; status: number; key?: string }> {
  const url = instanceUrl.trim().replace(/\/+$/, "");
  if (!url) return { ok: false, status: 0 };
  try {
    const res = await fetch(`${url}/api/api-keys`, {
      method: "POST",
      headers: { Authorization: `Bearer ${clerkToken}`, "content-type": "application/json" },
      body: JSON.stringify({ name }),
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
 * Register (or refresh) an Expo push token for this device, via
 * POST {instanceUrl}/api/push-devices. Idempotent on the token.
 */
export async function registerPushDevice(
  token: string,
  platform: "ios" | "android",
  override?: Settings,
): Promise<{ ok: boolean; status: number }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0 };
  try {
    const res = await fetch(`${settings.instanceUrl}/api/push-devices`, {
      method: "POST",
      headers: authHeaders(settings.token, true),
      body: JSON.stringify({ token, platform }),
    });
    return { ok: res.ok, status: res.status };
  } catch {
    return { ok: false, status: 0 };
  }
}

/**
 * Stop delivering push notifications to a token, via
 * POST {instanceUrl}/api/push-devices/unregister. A POST rather than a
 * DELETE on the token's own path segment — an Expo token contains square
 * brackets, which are an encoding hazard in a URL path.
 */
export async function unregisterPushDevice(
  token: string,
  override?: Settings,
): Promise<{ ok: boolean; status: number }> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0 };
  try {
    const res = await fetch(`${settings.instanceUrl}/api/push-devices/unregister`, {
      method: "POST",
      headers: authHeaders(settings.token, true),
      body: JSON.stringify({ token }),
    });
    return { ok: res.ok, status: res.status };
  } catch {
    return { ok: false, status: 0 };
  }
}
