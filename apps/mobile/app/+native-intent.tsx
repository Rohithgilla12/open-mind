import { getShareExtensionKey } from "expo-share-intent";

// expo-router resolves every incoming native URL to a route. Two URLs arrive
// that don't map to file routes and would otherwise land on Unmatched Route:
//  - the share extension's redirect (openmind://dataUrl=<key>?...) — the
//    shared payload travels via app-group storage, so the URL just needs to
//    land anywhere; ShareIntentGate in _layout picks the payload up and
//    routes to Capture.
//  - the device-connect deep link (openmind://link?...) — scheme-host form,
//    normalised to the /link route with its query intact.
export function redirectSystemPath({ path }: { path: string; initial: boolean }) {
  try {
    if (path.includes(`dataUrl=${getShareExtensionKey()}`)) {
      return "/";
    }
    const url = new URL(path);
    if (url.hostname === "link" || url.pathname === "/link") {
      return `/link${url.search}`;
    }
  } catch {
    // Not a parseable URL — fall through to expo-router's own handling.
  }
  return path;
}
