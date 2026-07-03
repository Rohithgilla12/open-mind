# Image Upload Capture — Design

Date: 2026-07-04 · Status: Designed autonomously (authorised overnight run; storage decision made per handoff recommendation — filesystem volume) · Advances "capture anything"

## Goal

Save a local image (drag-drop or file picker) as a first-class image card. Fills the gap between "save an image URL" (already works) and "upload the screenshot on my clipboard/disk". Storage stays Postgres-only-for-required-infra: blobs live on a mounted filesystem volume, metadata in an `assets` table.

## Storage decision (locked)

- Files on disk under `ASSETS_DIR` (env; container default `/data/assets`, compose mounts a named volume `assetsdata`; local dev default `./data/assets`, gitignored). Filename = `<asset uuid>` + extension derived from validated content-type — never from user input (no path traversal).
- `assets` table (every row `user_id`-scoped): `id uuid pk`, `user_id uuid`, `item_id uuid null`, `content_type text`, `byte_size bigint`, `original_filename text`, `created_at`.
- Optional S3/R2 adapter is explicitly deferred (documented as future config-gated integration).

## API

- `POST /assets` (multipart/form-data, field `file`): `http.MaxBytesReader` cap `ASSETS_MAX_BYTES` (default 10 MiB). Validate content-type is `image/{png,jpeg,gif,webp}` by sniffing the leading bytes (`http.DetectContentType`) AND matching an allowlist — reject otherwise (415). **AVIF is not in the allowlist** (removed pending lossless AVIF metadata stripping — see below) and is rejected 415 even if content-sniffed correctly. Before writing to disk, run the bytes through `internal/assets.StripMetadata` (jpeg: drop APP1/APP13/COM segments; png: drop `tEXt`/`zTXt`/`iTXt`/`eXIf` chunks; webp: drop `EXIF`/`XMP ` RIFF chunks and clear the corresponding `VP8X` flag bits; gif: passthrough, no metadata segments to strip) — lossless, pixel data untouched, corrupt/unparseable input is rejected (400) rather than silently passed through. Write stripped file, insert asset row, create an item (`cardType: image`, `url: ""`, `body: ""`, `lead_image_url: "/assets/<id>"`), link `asset.item_id`, enqueue enrichment. Return `201 Item`. Rate-limited (guarded set) and bearer-auth'd like all routes.
- `GET /assets/{id}`: user-scoped lookup (404 cross-tenant/missing), stream the file from disk with stored `Content-Type`, `Content-Disposition: inline`, `X-Content-Type-Options: nosniff`, `Cache-Control: private, max-age=31536000, immutable`. No directory listing; open by `filepath.Join(ASSETS_DIR, sanitized-id)` where id is a parsed UUID (defence in depth against traversal). Never serves anything but the allowlisted image types (stored type is already constrained at upload).
- openapi.yaml: `POST /assets` (multipart request body, 201 Item / 400 / 413 / 415), `GET /assets/{id}` (200 image/* binary / 404). Bearer applies.

## Pipeline (uploaded images)

`Pipeline.Run` on an item whose `url == ""` but that has a linked asset (or `lead_image_url` starting `/assets/`): skip extraction AND the image-URL sniff (no external fetch — the "url" is internal), set `cardType image`, title = asset `original_filename` stem, run `enrichText(title, title)` (summarise/tag/embed over the filename; noop stores verbatim, real providers get little to work with — acceptable, OCR is out of scope). Idempotent. Guard: the SSRF-safe fetch path must never be handed an `/assets/...` value.

## Web

- Quick-add area gains a file input + drag-drop zone (client component): on file selected/dropped, POST multipart to a web proxy `POST /api/assets` (passes cookie/bearer via `apiFetch` with the multipart body), then `router.refresh()`. Accept only `image/*`; show per-file progress/error inline.
- Image serving through the browser: `ItemCard`/detail `<img src>` for uploaded images points at `/api/assets/<id>` — a web proxy route (`GET /api/assets/[id]`) that calls `apiFetch("/assets/"+id)` (cookie → bearer, server-side) and streams the bytes back with the upstream content-type. Keeps the API token server-side; same-origin `<img>` works with the login cookie. Image-URL cards (external `leadImageUrl`) keep pointing directly at the external URL as today.
- Tokens-only styling; drag-drop zone uses `line`/`cobalt`/`surface`.

## Security review focus (call out explicitly to the reviewer)

Upload content-type spoofing (sniff + allowlist, not the client-supplied type), path traversal (UUID-only filenames + parsed-UUID serve path), stored-XSS via SVG (SVG NOT in the allowlist — reject), size DoS (MaxBytesReader), serving auth (every `/assets` + `/api/assets` path bearer/cookie-gated and user-scoped), `nosniff` header, no execution of stored files, EXIF/GPS/XMP/IPTC metadata leakage (stripped losslessly on upload, shipped — see Pipeline/API sections).

## Testing

- Go: upload happy path (png → 201, asset row, item image card, file on disk), oversize → 413, non-image (text/svg) → 415, cross-tenant `GET /assets/{id}` → 404, path-traversal id (`../`) → 404/400, pipeline test for uploaded-image item (skips fetch, title from filename, idempotent). Use `t.TempDir()` for `ASSETS_DIR` in tests.
- Web: build + lint; local docker e2e — upload a real png via `POST /api/assets` (cookie), see it as an image card, fetch it back via `/api/assets/<id>` (200 image/png), cross-check unauth → redirect/401.

## Out of scope

OCR, thumbnail/resize generation, lossless AVIF metadata stripping (AVIF re-allow tracked as a follow-up in `TODO.md`), clipboard-paste in the extension, video/file (non-image) uploads, S3 adapter.
