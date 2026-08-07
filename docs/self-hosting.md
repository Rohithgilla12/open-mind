# Self-hosting Openmind

Openmind self-hosts as **Postgres + one Go binary**. No Redis, no Python sidecars — `docker compose up` is the whole deployment.

## Quickstart

```bash
git clone <repo> && cd open-mind
cp .env.example .env      # optional: defaults work out of the box
docker compose up -d      # starts Postgres (pgvector) + the API/worker binary
```

This starts three services:

| Service | Bound to | Purpose |
|---|---|---|
| `db` | `127.0.0.1:5433` | Postgres + pgvector (persistent volume) |
| `api` | `127.0.0.1:8080` | Go API + River enrichment worker (one binary) |
| `web` | `127.0.0.1:3000` | Next.js web UI |

The API listens on `http://localhost:8080`; the web UI on `http://localhost:3000`. Migrations run automatically on api start.

Smoke test:

```bash
# Save a URL (returns instantly with status "pending")
curl -s -XPOST localhost:8080/items \
  -d '{"url":"https://paulgraham.com/greatwork.html"}' \
  -H 'content-type: application/json'

# After a few seconds, enrichment finishes (status "enriched")
curl -s localhost:8080/items | python3 -m json.tool

# Find it via full-text search
curl -s 'localhost:8080/search?q=great' | python3 -m json.tool

# Save a plain note instead of a URL (exactly one of url or note per save)
curl -s -XPOST localhost:8080/items \
  -d '{"note":"remember the milk"}' \
  -H 'content-type: application/json'
```

## Authentication

By default the API is **unauthenticated** — convenient for single-user local use, but the binary logs a warning on startup and you must not expose it to a network as-is.

Set `OPENMIND_TOKEN` to a strong secret to require a bearer token on every request (`/healthz` stays exempt for load-balancer probes):

```bash
OPENMIND_TOKEN=$(openssl rand -hex 32) docker compose up -d

curl -s localhost:8080/items -H "Authorization: Bearer $OPENMIND_TOKEN"
```

Requests with a missing or wrong token get `401`. The write and search endpoints (`POST /items`, `GET /search`) are additionally rate-limited per client IP (60 requests/minute, burst 10); over-limit requests get `429`.

### Web UI

The `web` service reaches the API in-network via `API_URL=http://api:8080` and shares the same `OPENMIND_TOKEN` as the api. Log in at `http://localhost:3000/login` with the token; the web app stores it in an httpOnly cookie and injects the bearer header server-side, so the token is never exposed to the browser.

When `OPENMIND_TOKEN` is set, the login page validates the token against the API before accepting it (wrong token → `401`). With no token set, any value is accepted (single-user localhost mode).

### Multi-user mode (Clerk)

**Self-hosting defaults to token mode** (above) — a single shared secret, no third-party dependency. The Docker images and `docker compose up` pin token mode, so a stock self-host deploy needs no Clerk keys. (The app *source* defaults to the hosted `openmind.gilla.fun` instance and its Clerk — that's what the prebuilt mobile/desktop/extension clients target — but your `web` container is pinned to token unless you set the variables below.)

If you want real multi-user accounts (e.g. a small cloud instance you share with friends), switch both services to [Clerk](https://clerk.com):

| Variable | Service | Where it comes from |
|---|---|---|
| `AUTH_MODE=clerk` | `api` | set explicitly (default is `token`) |
| `CLERK_ISSUER` | `api` | Clerk dashboard → **API keys** → **Frontend API URL** (looks like `https://your-app.clerk.accounts.dev`) |
| `NEXT_PUBLIC_AUTH_MODE=clerk` | `web` | set explicitly (default is `token`) |
| `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY` | `web` | Clerk dashboard → **API keys** → **Publishable key** |
| `CLERK_SECRET_KEY` | `web` | Clerk dashboard → **API keys** → **Secret key** — keep this out of git, same as any other secret in this file |

With `AUTH_MODE=clerk`, `/login` renders Clerk's hosted `<SignIn/>` instead of the token form, and the API verifies the Clerk session JWT (via JWKS, matched against `CLERK_ISSUER`) instead of comparing against `OPENMIND_TOKEN`. API keys and device-connect codes (see [API keys & connecting devices](#api-keys--connecting-devices)) work the same in both modes.

**Signup policy** (open / invite-only / allowlist) is a Clerk-dashboard setting, not something this app configures — see Clerk's **User & Authentication** settings.

**Preserving an existing library when you cut over:** if you already have a single-user (token-mode) library and switch that instance to Clerk, your existing items are owned by the auto-provisioned dev user, not your new Clerk identity. Map them with a one-off SQL statement after your first Clerk sign-in (so `users` has a row with your `clerk_user_id`):

```sql
UPDATE users SET clerk_user_id = 'user_…' WHERE clerk_user_id IS NULL AND id = '<dev user id>';
```

Find `<dev user id>` and confirm `user_…` (your Clerk user ID, from the Clerk dashboard's **Users** list or your session's `sub` claim) before running this — it's a manual, one-time step, not an automated migration.

### Exposing to a network

Both `api` and `web` bind to `127.0.0.1` only by default. **Map your public domain / reverse proxy to the `web` service (port 3000) only** — the browser never talks to the API directly, and the API does not need to be publicly reachable. Terminate TLS at your proxy (the login cookie is flagged `Secure` in production, so the UI must be served over HTTPS). Always set a strong `OPENMIND_TOKEN` before exposing anything.

## Configuration

All configuration is via environment variables (see `.env.example`):

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `postgres://openmind:openmind@localhost:5433/openmind` | Postgres connection string. Inside compose the API uses the `db` service host automatically. |
| `TEST_DATABASE_URL` | `postgres://openmind:openmind@localhost:5433/openmind_test` | Connection string used by the Go test suite only. |
| `PORT` | `8080` | HTTP listen port. |
| `OPENMIND_TOKEN` | _(empty)_ | Bearer token guarding the API. Empty = unauthenticated (fine for single-user localhost). Set a strong secret before exposing the API on a network. |
| `TRUSTED_PROXIES` | _(empty)_ | Comma-separated CIDRs/IPs of reverse proxies to trust for `X-Forwarded-For` when resolving the client IP for rate limiting. Empty (default) ignores `X-Forwarded-For` entirely and limits by the direct connection IP. Set it to your reverse proxy's source range so per-client limits work correctly behind a proxy, rather than every request sharing the proxy's own bucket. |
| `ASSETS_DIR` | `/data/assets` | Directory the API writes uploaded image bytes to. In compose this is backed by the named volume `assetsdata` — do not point it at an ephemeral container path in production. |
| `ASSETS_MAX_BYTES` | `10485760` (10 MiB) | Maximum accepted upload size for `POST /assets`; larger uploads are rejected with `413`. |
| `CONTACT_EMAIL` (`web`) | _(empty)_ | Contact address shown on the public `/privacy` and `/terms` pages. Empty falls back to "the operator of this instance" with no mailto link. |

## Image uploads

`POST /assets` (multipart form field `file`, `image/*` content types only, up to `ASSETS_MAX_BYTES`) stores the image on disk under `ASSETS_DIR` and creates an `image` card (`leadImageUrl` pointing at `GET /assets/{id}`). The web app exposes this via `POST /api/assets` and `GET /api/assets/{id}` (cookie-authenticated proxies) plus the `ImageDrop` upload UI; the underlying API routes are bearer-token authenticated like the rest of the API.

In `docker-compose.yml`, the `api` service mounts a named volume (`assetsdata:/data/assets`) so uploaded images survive container recreation — back it up the same way you back up `pgdata` if you rely on saved images. The volume is created automatically the first time you run `docker compose up`; no manual step is required.

**Privacy note:** uploaded JPEG, PNG, WebP, and AVIF images have EXIF/GPS, XMP, and IPTC metadata stripped losslessly on upload (pixel data is untouched — only metadata segments/chunks are removed). GIF images are stored unchanged, including any metadata they carry. A malformed image (recognised type, but its bytes fail to parse) is rejected with `400 could not process image` rather than being stored.

## PDF capture

Save a PDF two ways: upload a file (`POST /assets` with a `application/pdf` file, same endpoint and size cap as images) or save a PDF URL (`POST /items {"url": "..."}`) — either creates a `pdf` card instantly and extracts text asynchronously.

Text extraction runs in-process via `pdfium` compiled to WebAssembly (`apps/api/internal/pdftext`) — no extra service, no Python sidecar, no network call. The extracted body feeds the same FTS/vector search pipeline as any other card, and `pageCount` is recorded on the item.

Scanned or image-only PDFs still save and store fine, but have no searchable body — there is no OCR (by design; keeps the pipeline dependency-free and cheap).

**Privacy note:** unlike images, PDF metadata (author, creation date, producer, etc. in the document info dictionary) is **not** stripped on upload — PDFs are stored as-is.

## Document capture

Upload a Word (`.docx`), OpenDocument Text (`.odt`), Rich Text (`.rtf`), or EPUB (`.epub`) file to `POST /assets` — the same endpoint and size cap as images and PDFs. The card is created instantly and the text is extracted asynchronously. EPUBs become `book` cards; the rest become `article` cards.

Conversion runs in-process via [anydoc](https://github.com/firecrawl/anydoc) compiled to WebAssembly (`apps/api/internal/docmd`) — no extra service, no network call, and no Rust toolchain required to build Openmind, because the `.wasm` artefact is committed. `scripts/build-anydoc-wasm.sh` regenerates it if you bump the pinned version in `tools/anydoc-wasi/Cargo.toml`.

The file type is determined by sniffing the content, never the filename you upload — `.docx`, `.odt`, and `.epub` are all ZIP containers, so they are told apart by their internal structure. Spreadsheets, presentations, and CSV are deliberately **not** accepted: they produce poor cards and noisy embeddings. Uploading one returns `415 unsupported file type`.

The extracted text feeds the same FTS/vector search pipeline as any other card. The structured Markdown is kept alongside it in `items.body_markdown` for future use; nothing reads it today.

**Privacy note:** like PDFs, documents are stored **verbatim** — author, organisation, revision history, and tracked-change metadata embedded in the file are not stripped.

### AI is optional

With no provider configured (or `AI_PROVIDER=noop`), saves are extracted and made searchable via Postgres FTS — no external calls, no API key. Configure one or more providers to add AI summaries, auto-tags, and vector search. Only budget model tiers are ever wired into the enrichment pipeline; a flagship model is never used.

#### Env var reference

| Variable | Default | Description |
|---|---|---|
| `AI_PROVIDERS` | _(empty)_ | Comma-separated ordered fallback chain, e.g. `gemini,openai,noop`. Each entry is tried in order; a rate-limited or failing provider falls over to the next rather than failing the job. **Takes precedence over `AI_PROVIDER` if both are set.** |
| `AI_PROVIDER` | `noop` | Legacy/compat single-provider setting: `noop`, `gemini`, or `openai`. Still supported for existing deployments; prefer `AI_PROVIDERS` for new ones. |
| `GEMINI_API_KEY` | _(empty)_ | Required when `gemini` appears in the chain (or `AI_PROVIDER=gemini`). |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Base URL for any OpenAI-compatible endpoint. Only needs setting for non-default endpoints (a local/self-hosted server such as Ollama). |
| `OPENAI_API_KEY` | _(empty)_ | API key for the OpenAI-compatible endpoint; required (with `OPENAI_MODEL`) when `openai` appears in the chain. Some self-hosted servers accept any non-empty value. |
| `OPENAI_MODEL` | _(empty)_ | Chat/completion model name used for summarise and tag stages; required when `openai` appears in the chain. |
| `OPENAI_EMBED_MODEL` | _(empty)_ | Embedding model name. Must produce 768-dimension vectors — see the pgvector note below. |
| `AI_RPM_<NAME>` | _(empty)_ | Per-provider rate limit in requests per minute, e.g. `AI_RPM_GEMINI=10`, `AI_RPM_OPENAI=60`. `NAME` matches the provider name as it appears in `AI_PROVIDERS`, upper-cased. When the limiter is saturated the chain treats it as a fallover, not a failure. |
| `GEOCODER` | _(empty)_ | Optional geocoder for places extracted from social-video captions (Instagram reels, TikToks). `nominatim` enables OSM Nominatim, `google` enables Google Places Text Search; unset stores places by name without coordinates. Place extraction itself follows the AI provider — with `noop` it is simply off. |
| `NOMINATIM_URL` | `https://nominatim.openstreetmap.org` | Nominatim endpoint. Point at a self-hosted instance for heavy use; the public endpoint is called at most 1 request/second per its usage policy. |
| `NOMINATIM_EMAIL` | _(empty)_ | Contact e-mail sent with public-endpoint requests, per the [Nominatim usage policy](https://operations.osmfoundation.org/policies/nominatim/). Recommended when using the default endpoint. |
| `GOOGLE_PLACES_API_KEY` | _(empty)_ | Required when `GEOCODER=google`. A Google Maps Platform key with the **Places API (New)** enabled. Openmind requests only Pro-SKU fields (address + coordinates), currently 5,000 free calls/month then $32/1,000 — restrict the key to that API. Nominatim is free but its POI coverage (small restaurants, cafés, especially outside Europe/US) is much thinner. Note: Openmind stores resolved coordinates indefinitely; review [Google's Places data caching terms](https://cloud.google.com/maps-platform/terms) before enabling. |

#### Example: Gemini only

```bash
AI_PROVIDER=gemini GEMINI_API_KEY=<your-key> docker compose up -d
```

Single provider, no chain — if Gemini errors, the job fails and River retries; there is no floor provider to fall back to.

#### Example: Gemini → noop chain with a rate limit

```bash
AI_PROVIDERS=gemini,noop
GEMINI_API_KEY=<your-key>
AI_RPM_GEMINI=10
```

Caps Gemini calls at 10 requests/minute; once the limiter saturates, the chain falls over to `noop` instead of failing the job, so saves keep enriching (with basic extraction only) during a quota crunch rather than piling up as retries.

#### Example: Ollama via the OpenAI-compatible endpoint

```bash
OPENAI_BASE_URL=http://host.docker.internal:11434/v1
OPENAI_API_KEY=ollama
OPENAI_MODEL=llama3.2
OPENAI_EMBED_MODEL=nomic-embed-text
AI_PROVIDERS=openai,noop
```

Runs enrichment fully locally against Ollama. Use `host.docker.internal` (not `localhost`) so the `api` container can reach Ollama running on the host.

The `items_embedding` table's vector column is declared as `vector(768)` (`apps/api/internal/store/migrations/0001_init.sql`), and the codebase's `EmbedDims` constant is fixed at `768` (`apps/api/internal/ai/gemini.go`). `nomic-embed-text` produces 768-dimension embeddings, so it matches out of the box — no config change needed. If you pick a different embedding model with a mismatched dimension, the pipeline does not crash or fail the save: the embed stage compares the returned vector length against `EmbedDims` and, on a mismatch, logs a warning (`skipping embedding: unexpected dimension`) and skips saving the embedding row, leaving the item `enriched` with FTS search only (no semantic search) rather than failing the job (`apps/api/internal/enrich/pipeline.go`).

### Reel deep media (optional)

Place extraction reads a reel's caption, an embedded tagged location, and (by
default) its thumbnail. Set `REEL_MEDIA` to control the vision ladder:

| `REEL_MEDIA` | Behaviour |
|---|---|
| `off` | Caption + tagged location only — no vision model calls. |
| `thumbnail` (default) | Adds thumbnail vision (on-screen text). |
| `video` | Adds a deep-media rung: when caption/thumbnail name no place, download the clip and read sampled frames. |

`video` requires user-installed **`yt-dlp`**, **`ffmpeg`**, and **`ffprobe`**
(ffprobe ships with ffmpeg) on `PATH`; if any is missing the server logs a
warning and behaves as `thumbnail`. These binaries are never bundled and never
a `docker compose` requirement.

**Using `REEL_MEDIA=video` with the official API image**: the default
`apps/api/Dockerfile` build does not include yt-dlp/ffmpeg — the image stays
lean for everyone who doesn't need deep media. To get a variant with them
installed, build with the `INSTALL_REEL_MEDIA` build arg:

```bash
docker build --build-arg INSTALL_REEL_MEDIA=true -t openmind-api:video apps/api
```

With `docker compose`, add the build arg under the `api` service:

```yaml
services:
  api:
    build:
      context: apps/api
      args:
        INSTALL_REEL_MEDIA: "true"
```

Then set `REEL_MEDIA=video` in the environment as usual. The opt-in build
installs ffmpeg (ffprobe comes bundled with it) and yt-dlp; leaving the arg
at its default (`false`) reproduces the standard distroless image byte-for-byte.

## Importing

`POST /import` (or the web app's **Import** page) bulk-imports an export file: Netscape bookmark HTML (browsers, Pocket, Raindrop, Pinboard, Instapaper), CSV with a URL column, a plain list of URLs, or an **Omnivore zip export**. Every new URL becomes a pending item and enriches asynchronously; URLs already in your library are skipped, so re-importing is safe.

**Omnivore**: labels are preserved as your tags. Only the saved URLs are imported in the current version — the archived article bodies in the zip are not used yet, so pages whose original URL has since died will import as failed cards.

**Raindrop.io** can also be imported directly, with no export file: in Raindrop open **Settings → Integrations**, create an app, copy its **test token**, and paste it into the Raindrop section of the Import page (or `POST /import/raindrop` with `{"token": "..."}`). The server uses the token for a single export request against the Raindrop API and never stores or logs it. Raindrop tags are preserved as your tags, and each bookmark's collection becomes a tag too (the default "Unsorted" bucket is skipped). The same collections-become-tags rule applies when you upload a Raindrop CSV export by file.

## Exporting your library

A link to a full JSON export is on the web UI's home page (top-level nav). It calls `GET /api/export` (bearer-token or logged-in-cookie authenticated, scoped to your account) and returns every saved item as a JSON array, including each item's extracted text (`body`), title, tags, and metadata — so you always have a portable, lock-in-free copy of everything you've saved.

## Feeds (RSS/Atom subscriptions)

Subscribe to an RSS 2.0 or Atom feed and Openmind keeps saving new entries as normal items — no manual re-import.

- **Subscribe**: the `/feeds` page in the web UI (add-feed form), or `POST /feeds {"url":"<feed url>"}` directly (bearer-token auth like the rest of the API). Returns `201` with the feed's title/site URL and immediately backfills the feed's current entries as pending items (enriched asynchronously, same as any other save).
- **Poll interval**: subscribed feeds are re-polled automatically every 30 minutes by a River periodic job (`poll_feeds`) running inside the same `api` container — no extra service, no cron needed. A fresh subscription is also polled once immediately.
- **Dedup**: entries are matched against your existing saved URLs, so re-polling (or re-adding a feed) never double-saves an item.
- **Unsubscribing**: `DELETE /feeds/{id}` (`204`) stops future polling but does **not** delete items already imported from that feed — they stay in your library like any other save.
- **SSRF-safe**: feed URLs are user-supplied, so fetches go through the same private-IP-blocking, redirect-capped HTTP client used for extracting article content. A feed that can't be fetched or parsed is never persisted (`POST /feeds` returns `502`); a feed that later starts failing on a scheduled poll just records an error status (`last_status` on `GET /feeds`) rather than breaking the poll loop for other feeds.
- **Formats**: RSS 2.0 and Atom via the standard library XML parser only (no new dependency); RSS 1.0/RDF and podcast-specific tags are out of scope.
- **The Feed river**: feed-originated items carry provenance (`feedId`) and live on the **Feed** page rather than The Mind. Press **Keep** on any feed item to promote it into your library (it then also appears in The Mind and Drift). Manually saving a URL that already exists as an unkept feed item promotes it instead of duplicating. **Unsubscribing keeps the feed's items** — they're stamped as kept so nothing silently disappears.
- **Conditional requests**: the poller stores each feed's `ETag`/`Last-Modified` and sends `If-None-Match`/`If-Modified-Since` on the next poll, so an unchanged feed answers with a cheap `304` and no re-parsing happens. Servers that don't support validators simply always return the full feed — behaviour is unchanged.

## Tags

Every item has two independent tag lists:

- **AI tags** (`tags`) — set by the enrichment pipeline when an AI provider is configured; overwritten on every re-enrichment. Read-only in the UI.
- **Your tags** (`userTags`) — set by you (the detail page's tag editor, or `PATCH /items/{id} {"userTags": [...]}` directly) or preserved from an import. Enrichment never touches this list, so your tags survive re-enrichment.

Tags you enter are canonicalised on save: trimmed, lowercased, deduplicated, capped at 30 tags of up to 50 characters each. Both lists feed full-text search (`GET /search?q=...`), and the web UI shows the deduplicated union of AI + your tags on cards and in the detail view.

**Related suggestions.** Once items are embedded (requires an AI provider with embeddings — hidden under `noop`), the detail page's rail shows up to five **Related** items by semantic similarity, each with a one-tap **+ Link** that creates a normal bidirectional link. Agents get the same view via the MCP `related_items` tool.

**Imports keep their tags.** Netscape bookmark exports (`TAGS="a,b"` on an `<A>` element) and CSV exports with a `tags` column (Pocket, Raindrop) are captured as your tags on the created item — they are not lost when enrichment later sets the AI tags.

> Caveat: tag search uses `array_to_tsvector`, which indexes tags as literal lexemes without English stemming. A single-morpheme tag like `mine` matches a search for `mine`; a tag like `favourite` will not match a query for `favourite` if Postgres's English text-search config would otherwise stem it to a different form. Exact tag lookups are unaffected.

## Highlights

Select text in the reader view (`/item/{id}/read`) and click the floating **Highlight** button: the selection is painted in the article and mirrored as a **quote card** — a first-class item, searchable and taggable, linked to the source article. Highlights re-anchor by text (not raw offsets), so they survive re-extraction; if the underlying text changes, the paint disappears but the quote card lives on. Deleting the quote card removes the highlight; `DELETE /highlights/{id}` removes both.

## Desk (pinboard)

Pin whatever you're actively working with to a Desk — a small board separate from your full library, for the handful of items you want one click away right now.

- **Pin/unpin**: the pin toggle on an item's detail page, or `PATCH /items/{id} {"pinned": true}` (and `{"pinned": false}` to unpin) directly. Combine with `userTags` in the same request if you like — both are optional and independent.
- **View**: the `/desk` page in the web UI, also live in the sidebar nav. Backed by `GET /desk`, which returns your pinned items newest-pinned-first (never affects enrichment or search — pinning is purely organisational).
- **Scope**: pinning is per-account like everything else; unpinning never deletes the item, it only removes it from the board.

## Drift (resurfacing)

A calm, once-a-day full-screen mode that resurfaces old saves one at a time so forgotten items don't just pile up.

- **Candidates**: enriched, unpinned items not drifted in the last 30 days, oldest/never-revisited first, in batches of 5. Backed by `GET /drift`, which returns `{items, total}` and never mutates anything.
- **Actions**: for each card, **Let go** (`POST /drift/{id} {"keep": false}`) just marks it drifted so it won't resurface for 30 days; **Keep on my desk** (`POST /drift/{id} {"keep": true}`) does the same and also pins it to the Desk. Both are one-way per session — a kept item drops out of Drift and shows up on `/desk`; a released item drops out of Drift without appearing anywhere else.
- **View**: the `/drift` page in the web UI — an intentionally dark, immersive full-screen canvas (the one screen that departs from the paper/ink theme), reached from the sidebar nav. An empty batch shows a "caught up" state instead of a card.

## Browser extension

The WXT + React browser extension (`apps/extension`) is a thin capture client — it saves the active tab's URL, a selection as a note, or an image, and talks to your instance over the same bearer-token auth as the web UI. Enrichment stays server-side.

Build:

```bash
pnpm --filter extension build           # → apps/extension/.output/chrome-mv3
pnpm --filter extension build:firefox   # → apps/extension/.output/firefox-mv2
```

Load unpacked:

- **Chrome / Edge / Brave**: open `chrome://extensions`, enable **Developer mode**, click **Load unpacked**, select `apps/extension/.output/chrome-mv3`.
- **Firefox**: open `about:debugging#/runtime/this-firefox`, click **Load Temporary Add-on…**, select any file inside `apps/extension/.output/firefox-mv2` (e.g. `manifest.json`). Temporary add-ons are removed on browser restart.

Options page: open the extension's **Settings** (Chrome: right-click the toolbar icon → *Options*), set the **Instance URL** (e.g. `http://localhost:3000` for local testing) and paste the same value you set for `OPENMIND_TOKEN` server-side as the **access token**, then **Validate** and **Save settings**.

For the full manual verification checklist (popup save, context-menu save-selection/save-image, error states), see `apps/extension/README.md` — not duplicated here.

## MCP server

Openmind speaks the [Model Context Protocol](https://modelcontextprotocol.io) at `<instance>/mcp` over Streamable HTTP, so an AI agent (Claude Desktop, Claude Code, or any MCP client) can save into and search your library. It's served by the same API binary — no extra process — and authenticated with the **same `OPENMIND_TOKEN`** you already set, sent as a bearer header. `/mcp` has its own rate bucket, keyed per credential (5 req/s, burst 20), plus a loose per-IP ceiling, so a busy agent session neither starves nor is starved by the rest of the API.

Tools exposed:

- `save_item` — save a URL or a note (returns immediately; enrichment runs async)
- `search_items` — hybrid full-text + semantic search (optional colour, natural-language parsing)
- `list_recent` — the most recently saved items
- `get_item` — full detail of one item, including the archived body
- `related_items` — items semantically nearest a given one (pgvector neighbours)
- `set_user_tags` — replace your own tags on an item (AI tags are separate and untouched)
- `pin_item` — pin an item to the Desk, or unpin it
- `delete_item` — permanently delete an item; refuses without `confirm:true`, so an agent must check with you first
- `get_desk` — the items pinned to your Desk
- `get_drift` — today's Drift resurfacing candidates (read-only: an agent can browse them without consuming your daily Drift)
- `list_lenses` — your saved Lenses (named searches)
- `run_lens` — run a Lens and return what it currently matches
- `create_lens` / `delete_lens` — manage Lenses (delete echoes the removed rule so it can be recreated)

It also serves one **resource template** — `openmind://item/{id}`, the archived body as plain text, so clients can attach an item without a tool call — and one **prompt**, `find_and_summarise(query)`, which walks a client through search → read → summarise.

**Claude Code:**

```bash
claude mcp add --transport http openmind https://openmind.example.com/mcp \
  --header "Authorization: Bearer $OPENMIND_TOKEN"
```

**Claude Desktop** (`claude_desktop_config.json`) — via [`mcp-remote`](https://www.npmjs.com/package/mcp-remote), which forwards the auth header to a remote HTTP server:

```json
{
  "mcpServers": {
    "openmind": {
      "command": "npx",
      "args": [
        "-y", "mcp-remote", "https://openmind.example.com/mcp",
        "--header", "Authorization: Bearer YOUR_TOKEN"
      ]
    }
  }
}
```

### Local (stdio)

If your MCP client runs on the same machine as the Openmind binary (or you can `docker compose exec` into it), you can skip HTTP entirely:

```bash
claude mcp add openmind -- openmind mcp
# or against the compose stack:
claude mcp add openmind -- docker compose exec -T api openmind mcp
```

The stdio transport is **single-user only**: it acts as the auto-provisioned account and refuses to start under `AUTH_MODE=clerk` (use the HTTP transport with a device key there). It reads the same `DATABASE_URL` as the API. Note that stdio does **not** check `OPENMIND_TOKEN` — anyone who can execute the process acts as the single user, so the boundary is process/container access, not the token. Saves made over stdio enqueue enrichment as usual, but the queue is processed by your running `serve`/`work`/`all` process — the stdio process itself runs no workers.

To sanity-check the endpoint by hand, `apps/api/scripts/mcp-e2e.sh` drives `initialize` → `tools/list` → a few `tools/call`s over raw JSON-RPC:

```bash
API=https://openmind.example.com OPENMIND_TOKEN=your-token apps/api/scripts/mcp-e2e.sh
```

## API keys & connecting devices

Instead of sharing your `OPENMIND_TOKEN` across every device, mint **per-device API keys** (`omk_…`, shown once, revocable):

- `POST /api-keys {"name":"laptop"}` → `{key: "omk_…"}` — use it as the Bearer token anywhere a token works today (extension, mobile, dock, MCP, curl).
- `GET /api-keys` lists keys (name, prefix, last used); `DELETE /api-keys/{id}` revokes immediately.
- **Connect a device without copy-pasting secrets:** `POST /device-links` (authenticated) returns a short single-use code (`ABCD-EFGH`, 10-minute expiry). The new device calls `POST /device-links/claim {"code","deviceName"}` — no auth needed, the code is the credential — and receives its own freshly-minted key. Wrong/expired/used codes all return an identical 404, and the claim endpoint is strictly rate-limited (5/min per IP).

The web UI's **Settings → Devices & keys** page (`/settings/devices`) wraps all of the above: list/revoke keys, and a "connect a device" flow that renders the short code plus a QR code for scanning from your phone.

See [Multi-user mode (Clerk)](#multi-user-mode-clerk) above for real multi-user accounts — self-hosted single-user token mode remains the default and is unaffected either way.

> This is the Milestone 0 quickstart. Expanded operational docs (backups, upgrades, reverse proxy, auth) land in Milestone 1.

## Send to Kindle

Any item, or a Lens's current matches as a digest, can be e-mailed to your Kindle as an EPUB: `POST /items/{id}/kindle` (proxied at `/api/items/{id}/kindle`) or `POST /lenses/{id}/kindle` (proxied at `/api/lenses/{id}/kindle`). Both are queued asynchronously — the request returns `202 {"queued":true}` immediately, and delivery happens in the background via a River job. A Lens digest is capped at 25 matching items with a body, one EPUB chapter per item.

The feature is off until you configure outbound SMTP:

| Variable | Required | Description |
|---|---|---|
| `SMTP_HOST` | yes | SMTP server hostname. |
| `SMTP_PORT` | no (default `587`) | SMTP server port. |
| `SMTP_FROM` | yes | Sender address — must be approved in your Amazon account (see below). |
| `SMTP_USERNAME` | no | SMTP auth username, if your server requires it. |
| `SMTP_PASSWORD` | no | SMTP auth password, if your server requires it. Never commit this to source control or an `.env` checked into git. |
| `KINDLE_EMAIL` | no | Instance-wide fallback `@kindle.com` delivery address. Each user can instead set their own address on the **Settings** page (Devices & Keys → Send to Kindle) — the per-user address always wins. |

The SMTP transport (`SMTP_HOST` + `SMTP_FROM`) must be configured, and a recipient must exist — either the user's own Kindle address (set in the web app's Settings) or the `KINDLE_EMAIL` fallback. Otherwise both endpoints return `409`; `docker-compose.yml` passes all six variables through from the host environment (empty by default). Restart the `api` service after changing them.

### Amazon setup

1. Sign in at [amazon.com](https://www.amazon.com) → **Accounts & Lists** → **Content & Devices** → **Preferences** tab → **Personal Document Settings**.
2. Under **Approved Personal Document E-mail List**, add the address you set as `SMTP_FROM` — Amazon only accepts documents from senders you've explicitly approved.
3. Under **Send-to-Kindle E-mail Settings**, find the `@kindle.com` address for the device or app you want delivery to, and use it as `KINDLE_EMAIL`.

### Caveats

- Delivery is fire-and-forget from the API's perspective: a `202` means the job was queued, not that Amazon accepted the e-mail. Check your Kindle library (or the `api` service logs) if a send doesn't arrive.
- A transient SMTP failure is retried by River (up to 5 attempts) rather than dropped. Because retries resend the same EPUB, a retried job can occasionally deliver a duplicate — rare (only on error) and harmless.
- EPUBs open with a cover page and, where an item has a lead image, a hero image at the top of its chapter (fetched at build time; a missing or oversized image simply renders without one).

### Scheduled Lens digests

Any Lens can be sent on a schedule: pick **Daily** or **Weekly** (plus a weekday) in the Lens header. A scheduled digest contains only items that are **new since the last digest** (with an hour's grace for late-finishing enrichment) — nothing new means no e-mail. Scans run hourly from when the worker starts (times are UTC for weekly-day matching; per-user timezones aren't supported yet). The Lens header shows when the digest last went out.

## Notifications

Openmind can notify users about lifecycle events, feed-river digests, and other activity, over mobile push (Expo) and/or e-mail. Delivery is off by default — every notification is still recorded in the outbox and eventually pruned, but nothing is sent — until you set `NOTIFY_CHANNELS`.

| Variable | Required | Description |
|---|---|---|
| `NOTIFY_CHANNELS` | no | Comma-separated list of channels to enable: `expo`, `email`, or both (e.g. `expo,email`). Leaving it empty is a fully supported configuration — the app stays completely functional with notifications recorded but never delivered. |
| `EXPO_ACCESS_TOKEN` | no | Only used when `expo` is listed. Expo Push accepts unauthenticated requests; a token just adds send-security. Create one at [expo.dev](https://expo.dev) under Account Settings → Access Tokens. |

The `email` channel reuses the same outbound SMTP transport as [Send to Kindle](#send-to-kindle) above — `SMTP_HOST` and `SMTP_FROM` must be set (plus `SMTP_PORT`/`SMTP_USERNAME`/`SMTP_PASSWORD` as needed). If `NOTIFY_CHANNELS` includes `email` but SMTP isn't configured, the server logs a warning at startup and leaves e-mail notifications disabled rather than failing to boot.

The `expo` channel delivers to registered mobile devices, but registration only happens from a mobile app build that carries **your own Expo project ID** — the one shipped in this repo's `apps/mobile` points at the upstream project, not yours. If you build and distribute your own binary, you'll need your own Expo project (and, for production push, your own FCM and APNs credentials registered with that project) before device tokens can register at all; `EXPO_ACCESS_TOKEN` alone doesn't provide this.

`NOTIFY_CHANNELS` only decides which channels the *server* can deliver over — it says nothing about which channel a given user's notifications actually go out on. That's controlled separately, per user, by their `notify.digest` / `notify.feed_river` / `notify.lifecycle` preferences (each `off` / `push` / `email` / `both`), which default to `digest = push`, `lifecycle = push`, and `feed_river = off`. A user's preference must point at a channel you've actually enabled: if you set `NOTIFY_CHANNELS=email` (SMTP-only, no Expo project), every user still defaulting to `push` for digest and lifecycle notifications will have those notifications recorded in the outbox and then marked delivered without ever actually being sent — there is no destination to defer for, since push isn't wired up at all. Watch the `api` service logs for `flush_notifications: no live channel for this message; stamping without delivery` if notifications seem to be silently not arriving — it names the category, so you can tell which preference needs to move to `email` or `both` in Settings. (A wired-up channel that a user enabled but has no device or address registered for yet logs separately, as `flush_notifications: no destination for any enabled channel, deferring`, and retries hourly rather than stamping immediately.)

Restart the `api` service after changing either variable.
