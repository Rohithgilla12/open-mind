# Image Upload Capture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upload a local image → first-class image card, stored on a filesystem volume, served auth-gated through the web proxy. Spec: `docs/superpowers/specs/20260704-image-upload-design.md` — the authority, especially its Security review focus section.

**Architecture:** New `internal/assets` (filesystem blob store) + `assets` table; `POST /assets` (multipart) and `GET /assets/{id}` handlers; pipeline branch for uploaded-image items keyed on `lead_image_url` prefix `/assets/`; web drag-drop upload + `/api/assets/[id]` serving proxy.

**Tech Stack:** stdlib (`mime/multipart`, `net/http`, `os`, `path/filepath`) + existing; no new deps.

## Global Constraints

- Storage on disk under `ASSETS_DIR` (container `/data/assets` via named volume; local `./data/assets`, gitignored). Filenames = parsed-UUID only, never user input. `assets` table every-row `user_id`-scoped.
- Upload: `http.MaxBytesReader` `ASSETS_MAX_BYTES` (default 10 MiB); content-type by `http.DetectContentType` sniff AND allowlist `image/{png,jpeg,gif,webp,avif}` (SVG rejected); mismatch → 415, oversize → 413.
- Serve: user-scoped 404 on cross-tenant/missing; stored Content-Type; headers `X-Content-Type-Options: nosniff`, `Content-Disposition: inline`, `Cache-Control: private, max-age=31536000, immutable`; serve path opens `filepath.Join(ASSETS_DIR, <parsed-uuid>)` — never a raw path param.
- SSRF-safe fetch must never receive an `/assets/...` value (pipeline branch handles uploaded images before any fetch/sniff).
- Bearer auth + rate limiting apply to `/assets` routes. Token stays server-side in web (proxy via apiFetch). Tokens-only web styling. Generated code never hand-edited; errors `%w`; no banner comments; `go test -p 1 ./... && golangci-lint run ./...` + web build/lint green; commit per task.

---

### Task 1: Pipeline branch for uploaded-image items

**Files:**
- Modify: `apps/api/internal/enrich/pipeline.go`
- Test: `apps/api/internal/enrich/pipeline_test.go`

**Interfaces:**
- Consumes: `db.Item` (has `LeadImageUrl`), existing `runNote`/`enrichText`/image-URL branch.
- Produces: `Run` on an item with `Url == "" && strings.HasPrefix(item.LeadImageUrl, "/assets/")` → new `runUploadedImage`: no extraction, no fetch/sniff, `UpdateItemExtraction{Title: <derived>, Body: "", LeadImageUrl: item.LeadImageUrl (unchanged), CardType: "image"}` → `enrichText(title, title)`. Title derived from the item — but the item has no filename yet at pipeline time; Task 2 stores `original_filename` in the asset and sets the item title AT UPLOAD. So: `runUploadedImage` keeps the existing `item.Title` if non-empty, else "image". Idempotent.

- [ ] **Step 1: failing test** `TestPipelineUploadedImageSkipsFetch`: create item via a direct store insert with `Url:""`, `LeadImageUrl:"/assets/<uuid>"`, `Title:"screenshot"` (add a store test helper or use CreateItem then UpdateItemExtraction to set LeadImageUrl+Title — note CreateItem takes UserID/Url/Body; set lead image via a raw `UpdateItemExtraction`). Inject `failingExtractor` and a Pipeline whose `HTTPClient` would error if used (or assert extractor untouched). Assert: after Run, cardType image, status enriched, LeadImageUrl unchanged (`/assets/<uuid>`), title "screenshot", second Run identical.

- [ ] **Step 2: RED → implement the branch.** Ordering in `Run`: uploaded-image check (`Url=="" && HasPrefix(LeadImageUrl,"/assets/")`) BEFORE the `Url==""` note check, so uploads aren't misrouted to notes. → GREEN.

- [ ] **Step 3: full suite + lint + commit** `feat(enrich): pipeline branch for uploaded-image items`.

---

### Task 2: Assets store + upload/serve API + contract

**Files:**
- Create: `apps/api/internal/assets/store.go` (filesystem blob store), `apps/api/internal/assets/store_test.go`
- Modify: `openapi.yaml`, `apps/api/internal/store/queries/*.sql` (+ `assets.sql`), `apps/api/internal/store/migrations/0002_assets.sql`, `apps/api/internal/store/migrate.go` (embed picks up new file automatically), `apps/api/internal/api/server.go`, `apps/api/internal/api/ratelimit.go`, `apps/api/cmd/openmind/main.go` (wire assets dir + store into NewServer), `docker-compose.yml`, `.env.example`
- Generated: gen.go, sqlc db, schema.d.ts
- Test: `apps/api/internal/api/assets_test.go`

**Interfaces:**
- Produces:
  - `assets.NewFSStore(dir string) (*FSStore, error)` (mkdir -p dir); `Put(id uuid.UUID, r io.Reader, max int64) (int64, error)` (writes `dir/<id>`, returns bytes, enforces max via caller's LimitReader too); `Open(id uuid.UUID) (io.ReadCloser, error)`; path built with `filepath.Join(dir, id.String())`.
  - migration `0002_assets.sql`: `assets` table per spec + index `(user_id)`; `items` unchanged.
  - sqlc: `CreateAsset(user_id,item_id,content_type,byte_size,original_filename) :one`, `GetAsset(user_id,id) :one`.
  - `NewServer(store, riverClient, provider, token, assetStore *assets.FSStore, maxBytes int64) http.Handler` — extend signature (update cmd + all test call sites).
  - Handlers `CreateAsset` (multipart), `GetAsset` (stream); `guarded` gains `POST /assets` and `GET /assets/` (prefix match — note limiter guard currently exact-path; extend to prefix for `/assets/`).

- [ ] **Step 1: assets.FSStore TDD** — `store_test.go` using `t.TempDir()`: Put then Open round-trips bytes; Open missing → error; Put respects the byte cap. Implement. 
- [ ] **Step 2: migration + sqlc** — write `0002_assets.sql`, `assets.sql` queries, regenerate; `store.Migrate` embed glob already `migrations/*.sql`.
- [ ] **Step 3: openapi** — add `/assets` POST (requestBody multipart/form-data `{file: string binary}`, 201 Item, 400/413/415) and `/assets/{id}` GET (200 with `content: {image/png: {schema: {type: string, format: binary}}}` etc., 404). Regenerate. Adapt handlers to the generated signatures (multipart handlers in oapi-codegen chi come through as the raw `http.Request` — verify generated method shape; if oapi-codegen doesn't model multipart bodies well, the generated `CreateAsset(w,r)` still gives raw access — read `r.FormFile("file")` directly).
- [ ] **Step 4: handler tests** `assets_test.go` (fresh NewServer per test, `t.TempDir()` asset dir): png upload → 201 + item cardType image + asset row + file exists; 11 MiB → 413; `text/plain` and `image/svg+xml` bodies → 415 (sniff-based); `GET /assets/{id}` returns bytes + `X-Content-Type-Options: nosniff`; cross-tenant GET → 404; `GET /assets/not-a-uuid` → 404/400. RED → implement handlers (`CreateAsset`: MaxBytesReader, FormFile, sniff+allowlist, `assetStore.Put`, `CreateItem` with url="" then `UpdateItemExtraction` setting LeadImageUrl `/assets/<id>`+Title=filename stem+cardType image, `CreateAsset` row, enqueue EnrichArgs; `GetAsset`: parse uuid → GetAsset row (user-scoped) → Open → copy with headers).
- [ ] **Step 5: wire cmd + compose** — `main.go` reads `ASSETS_DIR` (default `/data/assets`) + `ASSETS_MAX_BYTES` (default 10485760), `assets.NewFSStore`, pass to NewServer; compose `api` gets `volumes: [assetsdata:/data/assets]`, env `ASSETS_DIR`/`ASSETS_MAX_BYTES`, top-level `volumes: assetsdata:`. `.env.example` documents. `.gitignore` add `data/`.
- [ ] **Step 6: full suite + lint + commit** `feat(api): image upload store, POST /assets + GET /assets/{id}`.

---

### Task 3: Web upload UI + serving proxy

**Files:**
- Create: `apps/web/app/api/assets/route.ts` (POST multipart proxy), `apps/web/app/api/assets/[id]/route.ts` (GET stream proxy), `apps/web/components/ImageDrop.tsx`
- Modify: `apps/web/components/QuickAdd.tsx` (or page) to include ImageDrop; `apps/web/components/ItemCard.tsx` + `app/item/[id]/page.tsx` (image src: when `leadImageUrl` starts `/assets/`, render `/api/assets/<id>` — strip the `/assets/` prefix to get id — else use it directly)

**Interfaces:**
- Consumes: `apiFetch` (passes cookie→bearer, and must forward multipart bodies + the upstream content-type for streaming back).
- Produces: `POST /api/assets` forwards the multipart body to API `/assets` (do NOT set JSON content-type — pass the incoming `Content-Type` including boundary through; `apiFetch` currently forces JSON when `init.body` present — add a guard so multipart passes through, or bypass apiFetch and build the fetch here reading the cookie). `GET /api/assets/[id]` → `apiFetch("/assets/"+id)` streamed back with upstream content-type + `X-Content-Type-Options: nosniff`.

- [ ] **Step 1: fix apiFetch multipart** — `lib/api.ts`: only set `content-type: application/json` when the body is not `FormData`/`ReadableStream` (guard: `typeof init.body === "string"`). Keeps existing JSON callers unchanged.
- [ ] **Step 2: proxy routes** — POST reads `await req.formData()` (or streams `req.body` with the original content-type header — prefer forwarding `req.body` + `req.headers.get("content-type")` to preserve the multipart boundary) to `/assets`; return the JSON Item + status. GET proxies bytes with upstream `content-type` and `nosniff`.
- [ ] **Step 3: ImageDrop.tsx** (client) — file input + drag-drop zone (accept `image/*`), on drop/select POST each file to `/api/assets` as `FormData` (`fd.append("file", file)`), inline per-file status, `router.refresh()` on success; tokens-styled (dashed `line` border, `cobalt` on hover).
- [ ] **Step 4: image src rewrite** — helper `assetSrc(leadImageUrl)`: if starts `/assets/`, return `/api/assets/` + id; else return as-is. Use in ItemCard image/video branches + detail page.
- [ ] **Step 5: build + lint + commit** `feat(web): image drag-drop upload + authenticated asset serving proxy`.

---

### Task 4: E2E + docs + wrap-up

**Files:** modify `docs/self-hosting.md`, `TODO.md`

- [ ] **Step 1: local docker e2e** — `OPENMIND_TOKEN=devtoken docker compose up -d --build api web` (db up; volume created). Create a small real png (imagemagick `convert` or a committed test fixture or `printf` a minimal png via python in scratchpad). `curl -F file=@shot.png -b <cookie> localhost:3000/api/assets` → 201 image card; `GET /api/assets/<id>` (cookie) → 200 image/png bytes; unauth → redirect/401; oversize → 413. Record evidence. `docker compose stop api web`.
- [ ] **Step 2: docs** — self-hosting.md: image upload + `ASSETS_DIR`/`ASSETS_MAX_BYTES` + volume note + EXIF-not-stripped privacy caveat (follow-up). TODO.md: "Assets table + image upload capture" → Done (dated); add "Strip EXIF/GPS from uploaded images" as a Next security follow-up.
- [ ] **Step 3: commit** `feat: image upload e2e evidence + docs`. Controller merges, pushes, redeploys (compose creates the `assetsdata` volume automatically; no manual step).
