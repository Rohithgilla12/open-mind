# TODO

> Lightweight maintainer notes. The real backlog lives in
> [GitHub Issues](../../issues) — file bugs and feature requests there.

## Now
- **Pre-scrub history still reachable on the now-public repo.** `refs/pull/1..6/head`
  remain fetchable anonymously; unshallowed they expose 247 of the 298 pre-scrub
  commits, so the squash is largely undone for anyone who fetches them. Verified
  2026-07-31: **no credentials leaked** (gitleaks over all 247 commits found only
  the documented `sk_test_placeholder` false positive; no key material ever
  committed). Residual exposure is low-severity — deploy-box IP + default
  username, and a personal e-mail in the old `/privacy` + `/terms` pages. Chase
  the GitHub Support ticket filed 2026-07-15; no credential rotation needed.
  Details in `.superpowers/launch-checklist.md` (local-only).
- Chrome Web Store submission — **parked by choice; everything in code is
  done** (PR #54, merged + deployed 2026-07-31). Blank default instance,
  optional per-origin host permissions, manifest description, v1.0.0, real
  icons, six 1280×800 store screenshots, and brand fonts bundled rather than
  falling back to system-ui. `CONTACT_EMAIL` is now set in production and
  `/privacy` + `/terms` serve a working mailto.
  **Remaining, all human-only:** register the developer account (US$5), and
  walk the first-run permission prompt by hand — the capture harness bypasses
  it, so no automated run has exercised that flow. Step-by-step playbook lives
  in Notion ("Openmind — Chrome Web Store submission & update playbook");
  listing copy and permission justifications in
  `docs/20260731-chrome-web-store-listing.md`.

## Decided — don't re-raise
- **Mobile and dock keep hard-defaulting to the hosted instance**
  (`apps/mobile/lib/clerk.ts`, `apps/dock/src/lib/settings.ts`). The extension
  was changed to blank + opt-in because a public store distributes it to
  strangers; mobile and dock deliberately keep the convenience default. This is
  a considered maintainer decision as of 2026-07-31, not an oversight — leave it
  alone unless the maintainer reopens it.

## Next
- (see Issues)

## Later
- Mobile offline photo queue follow-ups (PR #44): verify iOS Part B end-to-end
  on a fresh dev build (extension→app manifest round-trip; foreground drain
  lands the image in Library); add direct unit tests for `queueFileExists`
  catch path + `asset-store` `deleteQueueFile` negative path; RN component test
  for `capture.tsx` offline status branch
- Reel places Phase 3 leftover: MCP `item_places` tool
- Mobile overdrive follow-ups (PR #46): on-device pass — confirm the floating
  Liquid Glass tab bar doesn't obscure the last Library/Feed/Desk row (add
  bottom content inset if react-native-screens doesn't auto-adjust); verify
  Android tab icons + reduce-motion morph skip; consider a true image-hero morph
  (fly the lead image + match the 4:3 detail hero height, vs today's gradient
  cross-fade)
- Mobile notification rollout checks (need a fresh dev build — `expo-notifications`
  is a native module): confirm iOS system prompt fires exactly once and denial
  behaves as expected; verify Android channel produces a non-silent notification;
  confirm `getExpoPushTokenAsync` returns a live token against the real EAS project;
  verify a real push tap routes correctly in foreground, background, and killed
  states including the cold-start path; confirm the offline-drain local notification
  actually displays.
- Mobile push-device error messaging: 409 from `POST /push-devices` (device
  already registered to another account) collapses to generic "check your
  connection" error in `apps/mobile/app/(tabs)/settings.tsx` — needs explicit
  conflict messaging.
- Mobile sign-out device handover: unregister-on-sign-out is best-effort; offline
  sign-out, unreachable instance, or crash mid-flight leaves the push-device row
  owned by the previous account, causing next account's registration to 409 with
  no self-service recovery — needs server-side reaper or explicit "claim this
  device" flow.
- Place removal follow-ups: no removal affordance on the aggregate `/places`
  map page (web or mobile) — only item detail. A removed place is a plain row
  delete, so a future re-enrich/re-extract trigger may re-derive it (extraction
  is non-deterministic, so not reliably the same row) — would need a per-item
  suppression list the job filters against. Neither client's optimistic-removal
  path is unit-tested: `apps/web` vitest is node-only over `lib/`, and mobile
  jest only matches `.ts`, so component tests would need a new stack
  (jsdom + testing-library) — maintainer call, not a defect
- Places map follow-ups: note OSM tile runtime dep in self-hosting docs;
  clustering polish — keyboard-accessible web cluster markers (currently
  pointer-only), and (optional) truer mobile zoom mapping using viewport width
  instead of `longitudeDelta` alone
- Android follow-ups: Play Console internal-testing track (auto-updates vs
  the current sideloaded APK), Android adaptive icon, add Play App Signing
  SHA-1 to the Maps key restriction if we ship via Play
- Notifications follow-ups: per-item reminders (`remind me about this save`) —
  deliberately scoped out, needs a spec/table/item-detail UI; `notifyDailyCap` has
  no empty-string escape hatch (integer field), no clear-to-default path;
  notification-substrate polish: `Expo.Receipts` doc promises partial success
  (failed chunk still returns earlier chunks) but caller (`check_receipts`)
  discards the map on error; separately, `ListRecentTickets` `LIMIT 5000`
  without `ORDER BY` makes reconciliation arbitrary above 5000 tickets/hour —
  add `ORDER BY sent_at`.
- Dock follow-ups: tray Desk submenu, Win/Linux tab-grab, hotkey rebinding, DMG/notarisation
- `repo` card type: the reserved-first-segment denylist in
  `apps/api/internal/enrich/classify.go` (and its SQL twin in migration 0021)
  is not exhaustive by construction. When a forge adds a reserved route, URLs
  under it misclassify as repos until someone notices. Revisit if it bites more
  than once; the alternative (repo-root-only matching) was rejected in
  `docs/superpowers/specs/20260801-repo-card-type-design.md` because it splits
  one project's saves across two card types.
- `FilterStrip.tsx` home-page chips omit `recipe`, `tweet`, and `repo` has only
  just been added — the chip list has drifted from the card-type enum and
  should be derived from it or kept in sync deliberately.
- `TestListPushDevicesSkipsFailed` is order-dependent and flaky: `testStore`
  does not truncate `push_devices` between runs. Pre-existing, unrelated to
  the repo card type.

## Done (recent)
- Feed page header/filter spacing — `/feed` reached for a `var(--gutter)` that
  is defined nowhere, so both `padding` shorthands were invalid at computed-value
  time and the header *and* the river rendered with zero padding. Now on the
  house `18px 28px 16px` / `22px 28px 40px` rhythm like Desk and Places. The
  per-feed chips graduated from the content column into a `.feed-strip` chrome
  band flush under the header (same recipe as the Mind's `FilterStrip`) — they
  were previously 48px from the header they belong to and 28px from the first
  row they filter. Also: subscriptions now fetched server-side
  (`lib/feeds.ts`, shared with `/feeds`) so the strip can't appear late and
  shove the river down; broken `role="tablist"` → `role="group"` + `aria-pressed`
  (no tabpanel, no arrow keys ever existed); "+N more" got chip geometry and a
  way back; empty state distinguishes "no subscriptions" from "nothing published
  yet"; `.chip` ellipsises and the strip stacks two-per-row under 640px
  (2026-08-03)
- Web item detail + reader redesign — detail page reframed as "the card,
  opened": grounded panel (warm card shadow + terracotta screen hairline
  instead of the overlay drop-shadow), chrome bar (back link left; pin/Kindle
  separated from delete right), real primary actions under the title (cobalt
  "Read · N min" + outline "Open original"), and long bodies now preview ~3
  paragraphs fading into a "Keep reading →" hand-off instead of duplicating
  the reader; rail reordered (palette → tags → your tags → places → links →
  provenance) with the archive line pinned as a colophon; columns stack under
  920px via `.item-*` classes. Reader gained a terracotta scroll-progress
  hairline, reading-time in the kicker (`lib/reading-time.ts`, tested), a
  select-to-highlight hint, summary markdown rendering (was raw asterisks),
  and a closing colophon (palette dots + back/original/Kindle). Title
  3-line clamp deliberately kept (2026-07-17 decision) (2026-08-01)
- Public `/architecture` deep-dive page (web) — content as typed data in
  `apps/web/lib/architecture.ts` with vitest coverage, page is a pure renderer,
  exempted from auth in middleware, linked from the welcome footer + README.
  Written 2026-07-17, rebased and opened as a PR 2026-08-01 after sitting
  unmerged on a local-only branch (2026-08-01)
- **Deployed to prod 2026-08-01** — PRs #56 (Feed spacing + summary markdown),
  #57 (Raindrop token import), #59 (place removal). Box had been on #54 since
  2026-07-31. Sequential api→web build (load peaked 5.5, well under the <8
  rule), `docker restart cloudflared`, pruned first (77%→82% after, 8.0G free).
  No new env vars needed. Verified by grepping the running api binary for both
  new route strings with a negative control — note the box 401s *before*
  routing, so a 401 on an unknown path proves nothing about deployment
- Remove an extracted place — `DELETE /items/{id}/places/{placeId}` (204; 404
  for an unknown/cross-tenant item, an unknown place, or a place on a different
  item) so a hallucinated venue or a brand name read off a reel can be dropped.
  Web item rail gained a per-place `×`; the leading hairline became a shared
  `Rule` component the client section renders itself, so removing the last place
  takes the rule with it. Mobile gained a `×` with a destructive confirm and a
  single-row optimistic patch (not a list snapshot — that resurrected
  concurrently-removed places). Both clients treat only 204 as success: a 404 is
  as likely to be a missing proxy route or an instance predating the endpoint as
  it is an already-deleted place. Caveat on re-derivation in Later
  (2026-08-01)
- Direct Raindrop.io import (tweet request) — `POST /import/raindrop` takes a
  Raindrop API test token, pulls the account's one-shot CSV export server-side
  (token used for that single request, never stored/logged), and funnels it
  through the existing parse → dedupe → create-and-enrich path, so it's
  idempotent like file imports. Raindrop tags preserved; each bookmark's
  collection (CSV `folder` column) now also becomes a tag ("Unsorted"
  skipped) — for uploaded Raindrop CSVs too. Web Import page gained a
  token form; docs + welcome/README copy updated. Rejected token → 400,
  Raindrop down → 502, oversized export → 413 with upload-a-file fallback
  hint (2026-08-01)
- Notifications (PR #52, merged) — at-least-once delivery (Expo, email,
  noop) via idempotent outbox (migration 0020_notifications.sql, partial unique
  index guard). `internal/notify` adapter + four River workers: scan, per-user
  flush (preferences → coalesce → quiet-hours → cap), receipt reconcile, retention.
  Lens digests untouched; feed-river coalesces to one row per feed per hour;
  enrichment failures only. Daily cap counts outbox rows, independent of device/
  channel. Quiet hours defer (never drop), computed wall-clock in user's IANA zone
  for DST safety. Six `/settings` preferences, `/push-devices` registration, web
  settings UI, mobile permission + tap routing. Config: `NOTIFY_CHANNELS` (unset =
  silent noop), `EXPO_ACCESS_TOKEN`. **`notify.feed_river` off by default; digest +
  lifecycle push by default.** `docker compose up` unchanged (2026-07-27)
- Reel places Phase 4 (PR #49, merged) — deep media + location tag. `REEL_MEDIA=
  off|thumbnail|video` (default thumbnail); `video` shells out to user-installed
  yt-dlp+ffmpeg (new `internal/reelmedia`), samples ≤8 frames (long edge 768) into
  one batched vision call (`ExtractPlacesVisionFrames`), escalating only when
  caption+location+thumbnail found nothing. Opportunistic inline-JSON tagged
  location parsed at capture time → `items.tagged_location` (migration 0019),
  near-certain candidate outranking caption. Generalised `ai.MergePlaces` with a
  precedence table. Built subagent-driven; whole-branch review clean; CI green.
  **`video` is off by default** — enable on the box with `REEL_MEDIA=video` +
  yt-dlp/ffmpeg installed (2026-07-23)
- Default all clients to the hosted instance (PR #48, merged) — mobile, dock,
  extension, and web now point at `openmind.gilla.fun` (+ its Clerk) out of the
  box, each as a fallback on the existing env var so self-host overrides still
  work. Mobile/web bake the public Clerk publishable key (secret key stays
  env-only); web auth mode defaults to clerk with the key threaded through
  ClerkProvider + clerkMiddleware; dock/extension pre-fill the instance URL. The
  web Dockerfile + docker-compose pin `NEXT_PUBLIC_AUTH_MODE=token` so stock
  `docker compose up` still works without Clerk keys. Self-host smoothing is a
  deliberate later follow-up (2026-07-23)
- Mobile overdrive: cinematic card→detail morph + native Liquid Glass tab bar
  (PR #46, merged) — Reanimated 4 spring morphs a card's gradient hero into the
  detail hero (reduce-motion + hero-kinds gated; quote/note navigate plainly);
  tab bar switched to expo-router Native Tabs so iOS 26 adopts UIGlassEffect
  automatically (minimize-on-scroll, cobalt tint, SF Symbols + Ionicon Android
  parity). Calm fade is now the house detail transition. Verified on an iOS 26.5
  sim; needs a fresh native build + on-device follow-ups below (2026-07-22)
- Places marker clustering (PR #45, merged) — web (native MapLibre GeoJSON
  clustering, HTML count markers, no new dep) + mobile (supercluster behind a
  pure tested `lib/cluster.ts`); tap a cluster to zoom-and-expand, single pins
  keep their popup/callout. Also derived the web Places type from the api-client
  contract. Both platforms share supercluster (radius 50, maxZoom 14). A
  comprehensive PR review added review fixes (CI-only GeoJSON type resolution,
  NaN-safe zoom, guarded cluster taps, cluster a11y, typed supercluster union).
  Needs web dev + iOS build for a visual pass (2026-07-21)
- Mobile: offline photo queue (PR #44, merged) — in-app Capture + Android share intents
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
