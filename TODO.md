# TODO

> Lightweight maintainer notes. The real backlog lives in
> [GitHub Issues](../../issues) — file bugs and feature requests there.

## Now
- **`gilla.fun` is served from Singapore, not India — this is the single biggest
  latency cost and it is a billing change, not an engineering one.** Measured
  2026-08-11 from the Hyderabad box and from an Indian client at the same
  moment: `openmind.gilla.fun` and `gilla.fun` both resolve to `colo=SIN`, while
  `shopify.com` gets `HYD`/`MAA` and `cloudflare.com` gets `MAA`/`NAG` — i.e.
  paid zones get Indian colos, this free zone does not. Consequence: every
  request detours India → Singapore → India. Same page, measured three ways:
  **6–16 ms** rendered inside the box, **90–137 ms** from a client over an SSH
  forward (bypassing Cloudflare), **310 ms warm / 880 ms cold-connection /
  1465 ms worst** through the tunnel. Client is 20 ms from its nearest CF edge
  and the box is **1.3 ms** from cloudflared's edge IPs, so the tunnel leg is
  already optimal and **tuning/restarting `cloudflared` cannot help** — don't
  bother, it only interrupts every other service on that tunnel. Real options:
  (a) put the zone on a paid plan → expect ~310 ms to fall to ~60–90 ms;
  (b) unproxied DNS + TLS on the box (Caddy/LE) → the measured ~90–137 ms, but
  exposes the box IP and drops the WAF/DDoS layer; (c) leave it — the web app no
  longer *waits* on this latency (see Done, 2026-08-11).
  Backend is emphatically not the problem: Postgres 0.2 ms on the `/items` list
  (index scan on `items_user_created_id_idx`, 1510 rows), Go API <1 ms, box idle
  at 0.29/4 CPUs with 19 GB free.
- **Tunnel token sits in plaintext in `docker inspect cloudflared`** — it's an
  inline `--token eyJ...` run arg, so anyone with docker access on the box reads
  it. Move to a credentials file or a docker secret and rotate.
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
- Dock follow-ups: Win/Linux tab grab (no AppleScript equivalent — would need
  per-browser platform work or a bridge through the extension); unify the two
  HTTP paths to `POST /api/items` (Rust reqwest at 15s, TS plugin-http at 12s)
  behind one Rust `save_item` command so the queue cannot be bypassed — that
  unification is also the natural place to add the duplicate-recovery guard
  mobile already has (`apps/mobile/app/(tabs)/capture.tsx` checks, after a
  status-0 failure, whether that exact URL was created since the attempt
  began before enqueueing; the dock enqueues unconditionally, and the API
  doesn't dedupe on create, so a POST that times out after the server
  committed produces a duplicate when the queue retries — needs an extra GET
  plus a decision about note saves, so not done here); verify the remaining
  seven newly-added browser bundle ids against real installs (Safari
  Technology Preview, Orion, Vivaldi, Opera, Chrome Beta/Dev/Canary — the
  original five have been in production use since the first dock release;
  Chromium's bundle id was additionally confirmed by direct lookup)
- **Dock: three pre-existing bugs found by running the app on 2026-08-13, all
  shipped in 0.3.0 and none from the offline-queue work.** Recorded because the
  shape of them recurs:
  - **`core:default` grants no mutating window command.** `core:window`'s default
    permission set is 28 read-only commands, so `start_dragging`, `hide`, `show`,
    and `set_focus` each needed granting by name. Consequence: the panel could
    never be dragged (broken since the initial public release) and never hid
    itself — `hide` has five call sites, including Esc, and none worked. These
    fail with **no Rust-side trace at all**: the ACL rejection goes to the webview
    console, so the only symptom is "the button does nothing". A test in `lib.rs`
    now asserts all four by name. **Any new `getCurrentWindow().<verb>()` call
    needs a matching permission — check before assuming it works.**
  - **keyring 3.x cannot persist on macOS 26.** `set_password` returns `Ok` and
    nothing lands in any keychain API. Fixed by bumping to keyring 4 (the `v1`
    feature keeps the API identical). Worth re-testing persistence after any
    future macOS major upgrade.
- Dock queue polish, all deliberately deferred from the 2026-08-12 pass and none
  urgent — a whole-branch review found and triaged each:
  - **`lib.rs`'s `open_item` logs a `tauri_plugin_opener::Error` whose `Display`
    interpolates the URL** (`ForbiddenUrl { url, .. }`), i.e. `{instance_url}/item/{id}`.
    Harmless while it was debug-only, but that pass turned release logging **on**,
    so a self-hosted instance hostname can now reach a user's log file. No token
    or body, so not a constraint breach — still worth making content-free.
  - A revoked token now sets `lastError` on the stalled entry, but `PendingStrip`
    only renders `lastError` when the strip is **expanded**, and `attempts` is
    deliberately not bumped (so `stuck` stays false). A user who never clicks the
    chevron sees a plain gold "N saves waiting to sync" indefinitely. Surfacing
    it on the collapsed row would finish the job.
  - At the 100-entry cap, `enqueue_and_notify` reports "N pending" while cap
    eviction has just silently dropped the oldest capture. Mirrors mobile by
    design, but it is the last capture-loss path on the Rust side.
  - `build_menu` calls `settings_get()` on every tray rebuild purely to test a
    boolean, materialising and discarding the keychain token each time — and the
    rebuild now runs on every panel focus. A `settings_configured()` that reads
    only the URL entry would be faster and better secret hygiene.
  - `insert()` hardcodes `persisted: true` and is correct only because `enqueue`
    overwrites it; a pessimistic `false` default would fail safe, at the cost of
    flipping four test assertions.
  - `spawn_persister` has no force-flush on quit, so quitting within 500 ms of a
    move loses that geometry (reverts to the previously saved position).
    `write_saved` also uses a bare `let _ = fs::write`, so disk errors are silent
    — unlike `queue.rs`, which logs and writes atomically.
  - `subscribeQueue`'s effect can run its cleanup before the listen promise
    resolves, leaving the listener attached. It matches three pre-existing
    effects in `Panel.tsx`; harden all four together or none.
  - Pre-existing: `lib.rs`'s `shortcuts.json` parse warning interpolates `{e}`
    (no secret in that file), and a keyring error string is surfaced raw in a
    user-facing notification.
- `repo` card type: the reserved-first-segment denylist in
  `apps/api/internal/enrich/classify.go` (and its SQL twin in migration 0021)
  is not exhaustive by construction. When a forge adds a reserved route, URLs
  under it misclassify as repos until someone notices. Revisit if it bites more
  than once; the alternative (repo-root-only matching) was rejected in
  `docs/superpowers/specs/20260801-repo-card-type-design.md` because it splits
  one project's saves across two card types.
- Mobile card kinds are still hand-written: `apps/mobile/lib/theme.ts` declares
  its own `CardKind` union rather than deriving it from the contract, so it can
  drift from `openapi.yaml` the way web's chips could. Complete as of
  2026-08-03 (all ten kinds present in `lib/cards.ts` labels + `KNOWN_KINDS`);
  worth the same treatment web just got if it ever bites.
- `TestListPushDevicesSkipsFailed` is order-dependent and flaky: `testStore`
  does not truncate `push_devices` between runs. Pre-existing, unrelated to
  the repo card type.
- Pagination follow-ups: search + Lens results are still capped at 50 fused RRF
  matches and would need a cursor over a materialised fused ranking; `/desk` has
  no limit or cursor at all; `ItemPage` carries no `total`, so the Mind's
  masthead says "50+ gatherings" and mobile's grouped headers drop their counts
  while more pages remain — a true total needs a `COUNT(*)` per request or a
  count endpoint. Revisit the Mind's masonry (JS-distributed fixed columns, no
  seam) only if the per-page seam proves annoying against a real 50-card page.

## Done (recent)
- Dock functional polish (2026-08-12) — durable offline save queue (Rust-owned,
  policy mirrored from mobile's `capture-queue.ts`: cap 100, URL dedupe,
  oldest-first flush, 401 stops the pass, permanent 4xx dropped, transient
  bumps attempts and stops), pending strip in the panel, tray Desk submenu +
  pending count, resizable panel with clamped size/position memory, and eight
  more browsers in the tab grab. Spec:
  `docs/superpowers/specs/20260811-dock-functional-polish-design.md`.
  **⌘⇧S previously discarded a capture outright on a network error.**
- **Deployed to prod 2026-08-11 (third deploy) — card-click navigation.** The
  first deploy fixed *sidebar* navigation and left the app's most travelled
  navigation untouched: clicking a card in the grid. `/item/[id]` sits outside
  the `(app)` group on purpose (no sidebar), so `app/(app)/loading.tsx` never
  covered it — a card click still froze the grid for the whole ~310 ms round trip
  with no feedback, i.e. the original complaint, unfixed, on the most common
  path. Added `app/item/[id]/loading.tsx` (article-shaped: 980px centred card,
  terracotta hairline, 16/9 image well, kicker, two title lines, prose lines at
  uneven widths — also covers `/item/[id]/read` as the parent boundary) and
  `app/drift/loading.tsx` (Drift is likewise outside the group and server-fetches
  its whole batch before `DriftFlow` renders anything).
  Card links now warm **on intent**: `Grid`'s stretched anchor moved to a
  `CardLink` client component that flips `Link`'s `prefetch` on pointer-enter or
  focus. `prefetch={true}` on every card was the obvious move and the wrong one —
  a grid renders up to 50 cards, and fully rendering 50 item pages per view is
  far more traffic than the one page actually opened, competing with the page
  you're looking at over a 310 ms link.
  **Verified in a real browser** (Playwright) against `/spike/grid` — 500
  synthetic cards, auth-free, real `Grid` + real `ItemCard` — built in token mode
  so Clerk's client JS didn't redirect to `/login`: on load, **zero** `/item/`
  requests (no eager flood); after hovering one card, RSC prefetches fired for
  exactly that href and no other. Hovering costs two requests (Next's own hover
  prefetch is partial-only for a dynamic route; the prop flip is what fetches the
  page data), once per card, then `staleTimes.static` holds it 5 min.
  Deploy: image `aae1f6a3`→`c48fee08`, load peaked 2.12, disk 72%. All three
  loading chunks present in the built server output — `(app)`, `item`, `drift` —
  with `om-skel` in exactly 3 chunks, positive control present, negative 0; zero
  error lines in the web log; 11 routes checked correct. Still **not verified**
  behind the Clerk gate.
- **Build depends on network access to Google Fonts.** `lib/fonts.ts` uses
  `next/font/google` for all three families, so they are downloaded at build
  time — a transient network blip fails the whole build with a confusing
  "Warning: Error while requesting resource" / "Turbopack build failed with 9
  errors". Hit once locally 2026-08-11 and passed on immediate retry. Not a Next
  16 regression (same in 15). If a box build ever fails this way, just retry;
  self-hosting fixes would be to vendor the fonts locally.
- **Deployed to prod 2026-08-11 (second deploy of the day) — Next.js 15.5.20 →
  16.3.0.** Done for Turbopack + staying current, **not** for latency, and it
  duly delivered zero latency change: warm reused-connection TTFB 303–315 ms
  against 319–469 ms before (measuring client's RTT back to a normal
  47–54 ms), in-box render 11–27 ms against 6–23 ms. The Cloudflare SIN routing
  remains the floor, exactly as predicted below.
  Migration, all per the official upgrade guide: `middleware.ts` → **`proxy.ts`**
  via the `middleware-to-proxy` codemod (Next deprecated the `middleware`
  convention in 15.6; Clerk documents `proxy.ts` as the Next 16+ convention, so
  this is the blessed path); Next rewrote `tsconfig.json` (`jsx` "preserve" →
  "react-jsx", `.next/dev/types` added to `include`) and `next-env.d.ts`
  (imports + `root-params.d.ts`) — left in Next's generated formatting on
  purpose, since re-compacting it just gets rewritten next build; CI web job
  Node 20 → 22 to match `apps/web/Dockerfile` (Next 16 needs ≥20.9.0, so "20"
  passed only by resolving to the latest 20.x, and this repo has been bitten by
  CI/local skew before); `/architecture` data now says Next.js 16 with
  `LAST_UPDATED` 2026-08-11.
  Not applicable: `next lint` (we typecheck with `tsc`), `unstable_` prefixes
  (none), `experimental.turbopack` config (none set). **`cacheComponents`/PPR
  deliberately NOT enabled** — it needs every dynamic read wrapped in Suspense
  and cannot help our latency anyway.
  **`experimental.staleTimes` survives** — the build lists it under
  "Experiments", so the prefetch work from `96b5c2f` is intact. Verified:
  Turbopack builds clean on arm64 **musl** (the risk point) in 24 s, image
  `e2861f63`→`aae1f6a3`, load peaked 1.60, disk 72%; `tsc` clean, 82/82 tests;
  `/architecture` publicly serves "Next.js 16" ×4 with zero "Next.js 15" and the
  new `2026-08-11` date; `.om-skel` + `om-skel-sweep` still in the Turbopack CSS
  chunk (positive control present, negative 0); build reports
  `ƒ Proxy (Middleware)`; all 14 checked routes correct (public 200, gated 307);
  **zero** error/warn lines in the web log; api container untouched.
  **Not verified:** anything behind the Clerk gate — same gap as the first
  deploy, needs an eyeball.
- **Deployed to prod 2026-08-11** — the web nav perf work below (`96b5c2f` +
  `be9301d`, fast-forwarded onto `main`). Web-only, so `--build web` +
  `docker restart cloudflared`. Load peaked 2.36 (well under the <8 rule), disk
  70%→72% (13 G free). Image ID proves the rebuild took: `976888e2`→`e2861f63`.
  Verified: served CSS chunk hash changed `31fb1fad`→`fc15efbb` and contains
  both `.om-skel` rules plus the `om-skel-sweep` keyframes (positive control
  `.shell-hamburger` present, negative control 0); `om-skel` + `aria-busy`
  present in the built server chunk `6621.js`, so `loading.tsx` compiled in;
  `(app)` group dir present in `.next/server/app`. Routes: `/login` `/`
  `/architecture` `/privacy` `/welcome` 200, and `/desk` `/feed` `/places`
  `/lens/new` `/settings/devices` all 307 to the Clerk gate — i.e. every moved
  route still resolves at its original URL.
  **DEPLOY GOTCHA (now in the deploy skill):** this was a **file-move** change,
  and the house rsync deliberately omits `--delete`, so the old `app/desk`,
  `app/feed`, `app/page.tsx` … would have stayed on the box alongside the new
  `app/(app)/…` copies and collided as duplicate routes. Fixed with a second,
  scoped `rsync -az --delete` over `apps/web/app/` only (nothing server-only
  lives under there).
  **Not verified:** the perceived speed-up itself is behind the Clerk gate, so it
  was not observed end-to-end — needs an eyeball. TTFB is unchanged **by design**
  (310–450 ms warm; the change hides that latency, it does not remove it).
  Absolute numbers taken right after the deploy looked worse (1050–1791 ms)
  purely because the measuring client's own link had degraded — RTT to the box
  went 40–53 ms → 91–223 ms mid-session; the warm reused-connection cost was
  still 319–469 ms, matching the pre-deploy ~310 ms. Not a regression.
- **Upgrading Next.js did NOT fix the latency — checked and then actually done
  2026-08-11 (we are now on 16.3.0); don't re-raise a framework bump as a perf
  fix.** The upgrade was *feasible* (Clerk 7.7.3 peers `^16.0.10 || ^16.1.0-0`, so 16.3.0 satisfies it;
  React `^19` already meets Next 16's peer; no `next` advisories — the
  `pnpm audit` hits are transitive `brace-expansion` under
  `packages/api-client` → `openapi-typescript`) and worth doing on its own merits
  (Turbopack default = faster builds; with PPR, `loading.tsx` stops being a hard
  prefetch cut-off). But it cannot help *this* problem: of the ~350 ms per
  navigation the origin contributes ~15 ms (render 6–16 ms, API <1 ms), so there
  is nothing for a framework to reclaim. The one Next 16 feature that would have
  helped — PPR's **CDN-cached static shell**, where the CDN serves the shell and
  sends a resume request to the origin — requires the CDN to implement Next's PPR
  adapter protocol (`onCacheEntryV2`, postponed state, shell+stream
  concatenation; see Next's "ppr-platform-guide"). Vercel does that; Cloudflare
  in front of a self-hosted tunnel does not. Self-hosted, PPR only buys
  shell-first streaming *from the origin* — ~10 ms of the ~350 ms.
- **Web navigation perf, 2026-08-11 (`96b5c2f`, branch `perf/web-nav-shell-layout`).**
  Navigation stalled 310–900 ms with no feedback
  at all. The app had **zero** loading boundaries and zero `Suspense`, every
  route is dynamic (`apiFetch` reads cookies), and `Shell` was rendered by each
  of the 9 pages rather than a layout — so every click re-rendered the sidebar,
  re-sent it in the page's RSC payload, re-issued `/lenses` + `/account`, and
  showed the old page frozen until the round trip finished. Fixed by moving the
  9 pages into an `app/(app)/` route group whose layout owns `Shell` (URLs
  unchanged — parenthesised group), adding `app/(app)/loading.tsx` (masthead +
  card-grid skeleton in the main column; only possible because Shell is a layout
  now), `prefetch={true}` on the primary nav links (default "auto" only warms the
  shell for dynamic routes, so clicks still paid full latency), and
  `experimental.staleTimes {dynamic:30, static:300}` — Next 15 defaults `dynamic`
  to 0, which was discarding every prefetched payload the instant it landed.
  Active-nav state moved to `ShellNav` via `usePathname()`, which also stops "The
  Mind" highlighting on `/feeds`, `/import`, `/lens/new`, `/settings/devices`.
  Side fixes: `item/[id]` and `lens/[id]` were awaiting two independent fetches
  in sequence (now `Promise.all`), and `apiFetch` resolved a Clerk session token
  on every call only to discard it whenever a bearer header was forwarded.
  Verified: `tsc --noEmit` clean, 82/82 tests, production build succeeds with
  every route at its original path. **Not yet verified in prod** — needs a web
  rebuild + `docker restart cloudflared`, and the perceived win should be
  re-measured on the box afterwards.
- **Deployed to prod 2026-08-08** — PR #67 (document capture), squash-merged as
  `dd6e70e`. Touched both api and web, so sequential `--build api` then
  `--build web` + `docker restart cloudflared`. Load peaked 3.98 on the api
  build, 2.68 on web — both under the <8 rule. Disk 76%→82% (8.2 G free); the
  jump is the committed 5.2 MB wasm plus a fresh web image, worth watching next
  deploy. Image IDs prove both rebuilds took: api `23d4d78`→`1547b6a`, web
  `4df5c92`→`976888e`. **Migration 0023 confirmed applied** — recorded in
  `schema_migrations` at 12:13:33Z, and `information_schema` shows
  `items.body_markdown text NULL`. Verified with passing positive *and*
  negative controls: the public `/architecture` page serves the new
  `anydoc + wazero` stack row and "documents via anydoc" pipeline note, carries
  `2026-08-07` with the old `2026-08-03` gone; the api binary contains
  `docmd: converted output exceeds limit`, `compiling anydoc wasm`, and the
  embedded `anydoc` module (negative control 0). Routes: `/login` `/`
  `/architecture` 200, `/feed` `/desk` 307 to the Clerk gate. No errors in
  either container's log post-deploy.
  **Not exercised in prod:** no document has actually been uploaded through the
  live UI — the first real `.docx` will be the first time the wasm module
  compiles on the box (~3 s, one-off, lazily on first use).
- **Document capture (2026-08-07)** — upload `.docx`/`.odt`/`.rtf`/`.epub` as
  first-class cards. anydoc compiled to `wasm32-wasip1` and run under wazero
  (`internal/docmd`), mirroring how pdfium already works; the 5.2 MB artefact is
  committed and `go:embed`-ed, so no Rust toolchain is needed to build Openmind.
  Spec: `docs/superpowers/specs/20260807-document-capture-design.md`.
  **Not yet deployed, and iOS share-extension changes are unverified on device**
  — they need a fresh dev build (`expo-share-intent` and the native extension
  both changed). Follow-ups worth considering: URL routing for documents (a link
  to a `.docx` still falls through to the article extractor); nothing reads
  `items.body_markdown` yet, so a Markdown-aware reader mode or a
  structure-preserving EPUB export is the natural payoff; spreadsheets and
  presentations remain deliberately unsupported.
- **Deployed to prod 2026-08-04** — PR #66 (cursor pagination + infinite scroll),
  squash-merged as `1c1af2f`. Touched both api and web, so sequential
  `--build api` then `--build web` + `docker restart cloudflared`. Pruned first
  (79%→76%, 82% after). Load peaked 5.29 on the api build, 3.21 on web — both
  under the <8 rule. Image IDs prove both rebuilds took: api `2764dc`→`23d4d78`,
  web `cf14c3`→`4df5c92`. **Migration 0022 confirmed applied** — recorded in
  `schema_migrations`, and `pg_indexes` shows the swap completed exactly
  (`items_user_created_id_idx` present, `items_user_created_idx` dropped).
  Verified with passing positive *and* negative controls: web bundle carries
  "Load more saves", "Couldn't load more", and the new aria-live announcement;
  api binary carries "invalid cursor" and `created_at DESC, id DESC` ×3.
  `ListItemsAll` correctly still shows the old un-tiebroken ORDER BY ×1.
  `/login` `/` `/architecture` 200; `/feed` `/desk` 307 to the Clerk gate.
  No errors in the api log since boot.
  **NOT verified — needs a signed-in human:** that saving via QuickAdd/ImageDrop
  on `/` makes the new card appear. That was a Critical the whole-branch review
  caught (`router.refresh()` preserves client state, so `ItemRiver` discarded the
  fresh page 1); the fix is deployed but unobserved, because the Mind sits behind
  Clerk and no automated session can reach it. Also unverified: whether the
  IntersectionObserver auto-loads without a click, and the mobile tabs (no dev
  build exists — mobile also hard-defaults to this instance, so it will exercise
  the envelope for real once rebuilt)
- Infinite scrolling on web and mobile — `/items` and `/feed` now return an
  `ItemPage` envelope (`{items, nextCursor}`) with keyset pagination on
  `(created_at DESC, id DESC)`; migration 0022 adds the matching index and drops
  `items_user_created_idx`, which was a strict prefix of it. Handlers over-fetch
  by one so `nextCursor` is precise and the client never ends on an empty
  request; an undecodable cursor is a 400, never a silent page 1. Web appends one
  `.mind-col` block per page — appending into the shared container rebalanced the
  columns and moved 8 of 12 already-visible cards, measured. The load control is
  a real button with an IntersectionObserver pressing it early, so it stays
  keyboard-reachable. Mobile moved to `useInfiniteQuery`; pull-to-refresh, focus
  refetch, and mutation invalidation all trim to the first page first, because
  TanStack v5 refetches every loaded page. Also fixed a pre-existing silent
  failure: the extension and dock turned an unrecognised list body into
  `ok: true` with an empty list. Search, Lens, Desk and Places deliberately
  unchanged (2026-08-04)
- **Deployed to prod 2026-08-03 (second deploy of the day)** — PR #65 (card-type
  chips derived from the contract). Shipped `origin/main` @ `c876ebb`; web-only,
  so `--build web` + `docker restart cloudflared`, no api rebuild. Load 1.41,
  prune took / 78%→76% (79% after). Web image `d858c6a9`→`cf14c394` proves the
  rebuild took. `/login` `/` `/architecture` 200; `/feed` `/desk` 307 to the
  Clerk gate; #64's `.feed-strip` still in the served CSS and `var(--gutter)`
  still absent, so no regression. **Caveat: #65 is a behaviour-preserving
  refactor behind the auth gate, so its own effect was NOT observed live** —
  `typeFilters`/`chipLabel` are minified out of `.next/server` (verified with a
  passing positive control and a negative control), and the rendered chip labels
  are byte-identical by design. Its vitest guard is the real check
- Home-page filter chips can no longer drift from the card-type enum — `CardKind`
  in `apps/web/lib/cards.ts` is now derived from the OpenAPI contract
  (`NonNullable<Item["cardType"]>`) instead of a re-typed union, so adding a card
  type and regenerating fails `tsc` until every `Record<CardKind, …>` accounts for
  it (gradient, accent, label, and the new plural `chipLabel`). The chip list moved
  out of `FilterStrip.tsx` into tested data as `typeFilters`, leaving the component
  a pure renderer; a vitest guard asserts the curated chip order covers every kind,
  which is the half types can't catch. Rendered labels and order are byte-identical
  to before. Both halves verified by deliberately breaking them. Note the old TODO
  claim that the chips omitted `recipe`/`tweet` was stale — all ten were present;
  the real defect was that nothing enforced it (2026-08-03)
- **Deployed to prod 2026-08-03** — PR #64 (Feed header/filter spacing).
  Web-only change, so `docker compose up -d --build web` alone (no api rebuild,
  no migration) + `docker restart cloudflared`. Load 0.09 before building;
  `docker builder prune -f` first took / from 78%→76%, back to 78% after.
  Verified by pulling the served `/_next/static/css` chunk and grepping it:
  `.feed-strip`/`.feed-strip-more`/`.chip:focus-visible` present, `var(--gutter)`
  and the old `.feed-chips` gone. `/login` + `/` 200, `/feed` 307 to the Clerk
  gate as expected. Procedure now lives in the personal `deploy-openmind` skill
  (host details stay out of this public repo). **NB #65 landed after this deploy
  and is NOT on the box yet**
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
