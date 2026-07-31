import { browser } from "wxt/browser";

/**
 * Host access is requested per instance origin at runtime rather than declared
 * up front, so a store install asks for nothing beyond the one server the user
 * configures. See `wxt.config.ts` for the optional-permission declaration.
 */

/**
 * Build the match pattern covering an instance origin, e.g.
 * "https://openmind.example.com/sub" -> "https://openmind.example.com/*".
 * Returns null when the URL is unparseable or not http(s).
 */
export function originPattern(instanceUrl: string): string | null {
  const trimmed = instanceUrl.trim();
  if (!trimmed) return null;
  try {
    const { protocol, host } = new URL(trimmed);
    if (protocol !== "https:" && protocol !== "http:") return null;
    return `${protocol}//${host}/*`;
  } catch {
    return null;
  }
}

/** Whether host access to the instance origin has already been granted. */
export async function hasOriginAccess(instanceUrl: string): Promise<boolean> {
  const pattern = originPattern(instanceUrl);
  if (!pattern) return false;
  return browser.permissions.contains({ origins: [pattern] });
}

/**
 * Prompt for host access to the instance origin. Chrome only shows the prompt
 * from a user gesture, so call this directly from a click handler — never from
 * the background service worker, which has no gesture to attach to.
 *
 * Resolves false when the URL is invalid or the user declines.
 */
export async function requestOriginAccess(
  instanceUrl: string,
): Promise<boolean> {
  const pattern = originPattern(instanceUrl);
  if (!pattern) return false;
  try {
    return await browser.permissions.request({ origins: [pattern] });
  } catch {
    return false;
  }
}

/**
 * Drop host access for an origin the extension no longer points at, so a user
 * who switches instances doesn't leave the old grant behind. Best-effort:
 * failures are ignored because the grant is harmless if it lingers.
 */
export async function revokeOriginAccess(instanceUrl: string): Promise<void> {
  const pattern = originPattern(instanceUrl);
  if (!pattern) return;
  try {
    await browser.permissions.remove({ origins: [pattern] });
  } catch {
    // Ignored — Chrome refuses to revoke patterns it never granted.
  }
}
