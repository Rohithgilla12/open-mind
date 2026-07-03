/**
 * Rewrites an item's leadImageUrl into a browser-loadable src.
 *
 * Uploaded assets have a leadImageUrl of "/assets/<id>" (bearer-auth'd on the
 * API). The browser can't send that bearer, so route it through the same-origin
 * web proxy at "/api/assets/<id>", which carries the cookie and adds the bearer
 * server-side. External (http...) URLs are returned unchanged.
 */
export function assetSrc(leadImageUrl: string | undefined): string | undefined {
  if (leadImageUrl?.startsWith("/assets/")) {
    return "/api/assets/" + leadImageUrl.slice("/assets/".length);
  }
  return leadImageUrl;
}
