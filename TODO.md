# TODO

> Lightweight maintainer notes. The real backlog lives in
> [GitHub Issues](../../issues) — file bugs and feature requests there.

## Now
- (empty)

## Next
- (see Issues)

## Later
- Mobile offline photo queue follow-ups (PR #44): verify iOS Part B end-to-end
  on a fresh dev build (extension→app manifest round-trip; foreground drain
  lands the image in Library); add direct unit tests for `queueFileExists`
  catch path + `asset-store` `deleteQueueFile` negative path; RN component test
  for `capture.tsx` offline status branch
- Reel places Phase 4: optional yt-dlp deep media — see
  `docs/superpowers/specs/20260716-reel-places-design.md`
- Reel places Phase 3 leftover: MCP `item_places` tool
- Places map follow-ups: marker clustering,
  consolidate web Place types via api-client `paths[]`, note OSM tile runtime
  dep in self-hosting docs
- Android follow-ups: Play Console internal-testing track (auto-updates vs
  the current sideloaded APK), Android adaptive icon, add Play App Signing
  SHA-1 to the Maps key restriction if we ship via Play
- Dock follow-ups: tray Desk submenu, Win/Linux tab-grab, hotkey rebinding, DMG/notarisation

## Done (recent)
- Mobile: offline photo queue (PR #44) — in-app Capture + Android share intents
  enqueue images on network error and flush via `POST /api/assets`; images
  persisted to a durable `expo-file-system` dir (never lost to an ephemeral
  URI); iOS native share extension persists failed shares to the App Group
  (container file + `pendingShares` manifest), drained into the same queue on
  foreground. jest-expo harness added, 23 tests. iOS Part B needs a dev build
  to verify E2E (2026-07-21)
- Mobile: native Clerk sign-in (Continue with Google + email code) — mints an
  `omk_` device key from the Clerk session, no web-first hop; self-hoster
  builds without a key fall back to manual device-key/QR. Needs Clerk dashboard
  config + `EXPO_PUBLIC_CLERK_PUBLISHABLE_KEY` (EAS preview) + a new build to
  go live (2026-07-19)
- Mobile: save images from phone — Capture Choose/Take photo, Android
  `image/*` share intent, iOS share extension multipart upload to `/api/assets`,
  item detail shows lead image (2026-07-19)
- Android release prep: `app.config.js` injects the Maps Android key from
  `GOOGLE_MAPS_ANDROID_API_KEY` (EAS Preview env, out of git), `preview`
  APK/internal profile; first Android build shipped as a sideload APK for a
  tester (2026-07-19)
- Search puts the library first: unkept feed-river matches rank after every
  library match (API partition), with a "From your feeds" divider on web and
  a section header on mobile; FTS/vector mappers now carry feedId/keptAt
  (2026-07-19)
- Colour-search discovery: clickable palette dots (cards + item rail) run a
  colour search, one-time hint on the search-by-colour strip, colour-aware
  empty state; mobile tap-a-dot Library colour filter (next dev build) —
  issue-free in-app education (2026-07-18)
- Re-allow AVIF uploads with lossless EXIF/XMP stripping (ISOBMFF container
  rewrite, fail-closed, OOM-bounded) — issue #7 (2026-07-18)
- Adaptive feed polling: per-feed intervals (reset on new items, back off to
  24h, Cache-Control floor) — issue #11 (2026-07-18)
- Mobile: Send to Kindle from item detail (2026-07-17)
- Reel/social-video titles: peel Instagram/TikTok OG caption dumps into a
  short "Author: hook" title (caption stays in body); clamp detail/card
  titles to 3 lines on web + mobile for already-saved items (2026-07-17)
- Extract: fall back to first in-article `<img>` when `og:image` is
  missing (simonwillison.net-style posts) (2026-07-17)
- Places map controls: mobile back + current-location FABs (expo-location);
  web MapLibre GeolocateControl (2026-07-17)
- Reel places Phase 2: thumbnail vision (`ExtractPlacesVision` on Gemini,
  caption+vision merge by confidence) (2026-07-17)
- Places map on web + mobile (GET /places, /places MapLibre page, item-rail
  places, react-native-maps screen — needs new dev build) + Google Places
  geocoder (2026-07-17)
- Mobile: Desk pins + TanStack Query cache (no feed/tab reload flash) + richer
  long-press actions (pin/open/copy/share/delete) (2026-07-17)
- Mobile: delete an item (detail screen + long-press in Library) and group Library by card type (2026-07-16)
- Mobile offline capture queue + in-app Library search (2026-07-16)
- Dock v1.1 Desk/Recents home + Launch at login (2026-07-15) — see `docs/superpowers/specs/20260715-dock-desk-autostart-design.md`
