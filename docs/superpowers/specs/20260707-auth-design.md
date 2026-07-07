# Multi-user Auth: Clerk + API Keys + Device Connect — Design

Date: 2026-07-07 · Status: Approved in discussion (user: third-party auth so they don't maintain it; cross-surface auth is the hard requirement; QR/short-code device connect is a MUST) · Replaces the deferred "real auth" M1 item

## Goal

Real multi-user accounts on the **cloud** instance (share with friends) without building or maintaining an auth system — while the extension, mobile app, dock, and MCP keep working with zero identity-provider code in them, and the **self-hosted** deployment keeps its no-third-party-dependency guarantee.

## The architecture in one paragraph

The identity provider (**Clerk**) lives in the web app only. Every other surface keeps speaking the one credential the whole codebase already uses: a Bearer token to `/api/*` — but tokens become **per-user API keys** minted in web Settings, delivered to devices via a **connect-device flow** (QR / short code) instead of copy-paste. The Go API stays the single enforcement point: it accepts (a) an API key, (b) a Clerk session JWT (verified via JWKS), or (c) the legacy `OPENMIND_TOKEN` in self-host mode. The schema is already multi-tenant; this adds identity mapping + key management, not a data-model change.

## Modes

`AUTH_MODE=token` (default — self-host, exactly today's behaviour: `OPENMIND_TOKEN`, auto-provisioned dev user; principle 3 intact, no new required service) or `AUTH_MODE=clerk` (cloud). Web app renders token-login or Clerk `<SignIn/>` accordingly. API keys + device connect work in **both** modes (in token mode they belong to the single user — still nicer than sharing the master token across devices).

## Schema (migrations)

- `users` gains `clerk_user_id text UNIQUE` (nullable) + `email text` (nullable). First authenticated request with an unseen Clerk id auto-provisions a user row (JIT provisioning — no webhook required for v1; Clerk webhooks for delete/cleanup are a follow-up).
- New `api_keys(id uuid PK, user_id uuid REFERENCES users, name text, key_hash bytea UNIQUE, prefix text, created_at, last_used_at, revoked_at)`. Key format `omk_<43 base64url chars>` (256-bit); stored as SHA-256 hash; the full key is shown/returned **exactly once**. `prefix` = first 8 chars after `omk_` for display. `last_used_at` updated at most once/minute (cheap).
- New `device_links(code_hash bytea PK, user_id, device_hint text, created_at, expires_at, claimed_at)` — short-lived connect codes.

## API auth middleware (replaces `requireBearer` when in clerk mode; extends it in token mode)

Resolution order for `Authorization: Bearer <cred>`:
1. `omk_` prefix → API-key lookup (hash compare) → user; revoked/unknown → 401.
2. In clerk mode: treat as a Clerk session JWT → verify RS256 against Clerk's JWKS (cached, refreshed on unknown-kid), issuer/audience checked → `clerk_user_id` → user (JIT-provision on first sight). Used by the Next.js server layer when proxying for a logged-in browser session.
3. In token mode: constant-time compare with `OPENMIND_TOKEN` → dev user (today's path).
`/healthz` stays open; the device-claim endpoint has its own rule (below). **New Go dependency (justified):** `github.com/golang-jwt/jwt/v5` + JWKS fetch (small hand-rolled JWKS cache with stdlib HTTP — no heavyweight auth SDK server-side). Hand-rolling JWT RS256 verification is the kind of crypto plumbing you don't DIY.

## Contract (openapi.yaml)

- `GET /api-keys` → list (id, name, prefix, createdAt, lastUsedAt) — never the key.
- `POST /api-keys {name}` → 201 `{key: "omk_…", …meta}` — the only time the key is returned.
- `DELETE /api-keys/{id}` → 204 (sets revoked_at).
- `POST /device-links {deviceHint?}` (authenticated, from web Settings) → 201 `{code: "ABCD-EFGH", expiresAt}` — 8-char Crockford-base32 (no 0/O/1/I), TTL 10 min, single-use.
- `POST /device-links/claim {code, deviceName}` — **unauthenticated** (the code is the credential): valid+unclaimed+unexpired → marks claimed, mints an API key named `deviceName`, returns `{key, instanceUrl?}` once → 201. Wrong/expired/used → 404 (uniform). **Heavily rate-limited** (per-IP, stricter than the global bucket) and codes are hashed at rest; 8 chars ≈ 40 bits + 10-min TTL + rate limit ⇒ brute-force infeasible.

## Web

- Clerk integration (`@clerk/nextjs`): middleware guards pages in clerk mode; the existing cookie/token login stays for token mode (env-switched). Server-side proxies (`apiFetch`) attach the Clerk session JWT instead of the om_token cookie in clerk mode.
- **Settings → Devices & keys** page: list keys (name, prefix, last used, revoke); "Connect a device" → calls `POST /device-links`, renders the short code in big type **and a QR code**. QR payload: `openmind://link?code=ABCD-EFGH&url=https://openmind.gilla.fun` — a deep link the phone's native camera opens directly in the mobile app (no in-app scanner needed). QR rendered client-side with a tiny dependency (`qrcode` — canvas/SVG generator, no network) or a pure-SVG implementation in `packages/ui`; countdown to expiry; regenerate button.
- Signup policy is a Clerk-dashboard setting (open/invite-only/allowlist) — not build work.

## Clients (thin changes, no IdP code)

- **Mobile:** Settings gains "Connect with code" (enter `ABCD-EFGH` → claim → store key+url in secure-store) and handles the `openmind://link?code&url` deep link (scheme already registered; `ShareIntentGate` pattern reused for a `LinkClaimGate`) → auto-claims and lands signed-in. Manual URL+token entry stays as fallback.
- **Extension options + dock settings:** add the same "enter connect code" path (they have keyboards; QR adds nothing there) → claim → store key. Existing manual token entry stays.
- **MCP:** header value becomes an API key — no change needed (`Authorization: Bearer omk_…` flows through the same middleware).

## Security notes

Keys hashed (SHA-256) at rest, shown once; codes hashed, single-use, 10-min TTL, uniform 404, tight per-IP rate limit on claim; JWT verification with issuer/audience/exp enforced and JWKS cache keyed by kid; token never logged anywhere (existing discipline); `omk_` prefix makes secret-scanning feasible later. Revocation is immediate (hash lookup checks revoked_at).

## Out of scope (v1)

Clerk webhooks (user deletion sync), org/teams, RBAC/roles, key scopes (all keys are full-access for their user), rotating the legacy OPENMIND_TOKEN path out of token mode, session UI beyond Clerk's components, migrating existing cloud data (single dev user's items stay owned by that user — map YOUR Clerk account to the existing dev user id in a one-off migration so your library survives).

## Verification

Go: middleware table tests (key/JWT/legacy/revoked/expired-code paths; uniform 404s; rate limit), JWKS verify against a locally-generated RSA keypair + fake JWKS server, claim-flow e2e (mint code → claim → key works → code reuse 404). Web: token-mode regression (compose e2e stays green with AUTH_MODE unset), clerk-mode build with test keys, devices page drives (create code → QR renders → claim via curl → key appears in list). Clients: mobile deep-link claim on the sim; extension/dock manual code entry against compose. Cloud cutover last: set AUTH_MODE=clerk + Clerk keys on the box, map your user, verify friends can sign up and see empty libraries (tenant isolation, already tested at the store layer).

## Execution

Decomposed into three plans (each independently shippable):
1. **auth-api** — schema, keys, device-links, middleware (JWT+key+legacy), contract, rate limit, tests. Ships dark: token mode unchanged.
2. **auth-web** — Clerk integration (env-gated), Devices & keys page with QR, proxy JWT pass-through. Cloud cutover at the end.
3. **auth-clients** — mobile deep-link/code claim, extension + dock code entry.
