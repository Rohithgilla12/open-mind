import { getSettings } from "./storage";

export interface SaveBody {
  url?: string;
  note?: string;
}

export interface SaveResult {
  ok: boolean;
  status: number;
}

function normaliseUrl(instanceUrl: string): string {
  return instanceUrl.replace(/\/+$/, "");
}

/**
 * POST a new item to the configured instance. Returns the HTTP status and
 * whether the request succeeded. Network failures surface as status 0.
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
    return { ok: res.ok, status: res.status };
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
