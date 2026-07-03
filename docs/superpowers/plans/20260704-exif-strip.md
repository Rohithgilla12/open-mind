# Strip Metadata From Uploaded Images — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]` tracking.

**Goal:** Losslessly strip EXIF/XMP/IPTC/text metadata from uploaded images at upload time (no re-encode, no quality loss), so a publicly-mapped instance never serves GPS-bearing photo originals.

**Architecture:** New `internal/assets/strip.go` `StripMetadata(contentType, data) ([]byte, error)` — format-specific surgical segment/chunk removal, stdlib only. `CreateAsset` reads the full (size-capped) upload into memory, sniffs, allowlist-checks, strips, then stores the stripped bytes. AVIF removed from the allowlist (can't be stripped losslessly with stdlib; safe-by-default reject rather than store metadata).

## Global Constraints

- Lossless: never decode/re-encode pixel data — walk the container and drop metadata segments only. No new dependency (stdlib only; justify otherwise).
- Formats: JPEG (drop APP1=EXIF/XMP, APP13=IPTC/Photoshop, COM comments; keep APP0=JFIF, APP2=ICC, all scan data); PNG (drop tEXt/zTXt/iTXt/eXIf chunks; keep the rest incl. IHDR/IDAT/IEND); WebP/RIFF (drop `EXIF` and `XMP ` chunks, fix RIFF size, clear the VP8X EXIF/XMP flag bits if a VP8X chunk is present); GIF (no EXIF → return unchanged); AVIF removed from allowlist → 415.
- Parse failure / truncated / not-actually-that-format → `StripMetadata` returns an error → handler rejects 400 (never store an image we couldn't sanitize).
- Security invariants from the upload feature stay intact: sniff+allowlist (never client type), UUID-only on-disk names, size cap → 413, user-scoped, `nosniff` on serve. errors `%w`; no banner comments; `go test -p 1 ./... && golangci-lint run ./...` + web build green.

---

### Task 1: StripMetadata + CreateAsset integration

**Files:**
- Create: `apps/api/internal/assets/strip.go`, `apps/api/internal/assets/strip_test.go`
- Modify: `apps/api/internal/api/assets.go` (allowlist − avif; read-all → sniff → allowlist → strip → store), `apps/api/internal/api/assets_test.go` (avif now 415; strip-on-upload assertion)

**Interfaces:**
- Produces: `assets.StripMetadata(contentType string, data []byte) ([]byte, error)` — returns metadata-stripped bytes; error on unparseable/truncated input for the strippable formats; GIF returns `data` (copy) unchanged.

- [ ] **Step 1: strip_test.go (TDD, the core).** Build real fixtures in-test:
  - JPEG: `jpeg.Encode` a 2×2 image to a buffer → inject an APP1 EXIF segment (`FFE1`, len, `"Exif\x00\x00"` + a few TIFF/GPS-ish bytes) immediately after SOI (`FFD8`) → this is the "photo with EXIF". Assert: `StripMetadata("image/jpeg", withExif)` output contains NO `FFE1` marker and NO `"Exif\x00\x00"` bytes, still `jpeg.Decode`s to a 2×2 image, and the scan data is byte-identical to the original no-EXIF encoding. Also inject+assert-removed an APP13 (`FFED`) and a COM (`FFFE`); assert APP0 (JFIF) survives.
  - PNG: `png.Encode` a 2×2 image → inject a `tEXt` chunk (and an `eXIf` chunk) after IHDR → assert `StripMetadata("image/png", …)` removes them, still `png.Decode`s, IHDR/IDAT/IEND present.
  - WebP: hand-craft a minimal `RIFF....WEBP` with a `VP8 ` chunk + an `EXIF` chunk + an `XMP ` chunk (no stdlib webp encoder — structural craft) → assert output has no `EXIF`/`XMP ` FourCCs, the `VP8 ` chunk is preserved byte-identical, and the RIFF size field equals `len(out)-8`.
  - GIF: `gif.Encode` → `StripMetadata` returns equal bytes.
  - Corrupt: truncated JPEG (SOI + partial APP1 length) → error; random non-image bytes for a claimed jpeg → error.
  - No-metadata: a freshly `jpeg.Encode`d image with no APP1 → decodes fine, no error.

- [ ] **Step 2: RED** — `go test ./internal/assets/ -run TestStrip` fails (undefined).

- [ ] **Step 3: implement strip.go.** One dispatcher on contentType → `stripJPEG`/`stripPNG`/`stripWebP`/gif-passthrough. Algorithms:
  - **JPEG**: require `FFD8`. Emit SOI. Walk `FF <marker>`: skip standalone markers (`D0–D7`, `01`) by copying; for segment markers read 2-byte BE length; if marker ∈ {`E1`,`ED`,`FE`} drop (skip length+payload), else copy marker+length+payload; on `DA` (SOS) copy it and the entire remainder verbatim and stop. Guard all slice indexing (truncation → error).
  - **PNG**: require the 8-byte signature. Walk chunks (len BE + type + data + crc); drop type ∈ {`tEXt`,`zTXt`,`iTXt`,`eXIf`}; copy others verbatim; stop after `IEND`. Bounds-check → error on truncation.
  - **WebP**: require `RIFF`....`WEBP`. Walk FourCC+LE-size(+pad) chunks; drop `EXIF`/`XMP `; if a `VP8X` chunk is copied, clear its EXIF (0x08) and XMP (0x04) flag bits in byte 0 of its payload; copy others; rewrite the RIFF size (`len(out)-8`, LE). Bounds-check → error.
  - **GIF**: return `append([]byte(nil), data...)`.

- [ ] **Step 4: integrate in CreateAsset.** Remove `"image/avif"` from `allowedImageTypes` (keep `detectAVIF` so AVIF is recognised and cleanly 415'd, not mis-sniffed). Refactor the handler: `data, err := io.ReadAll(file)` (body already `MaxBytesReader`-wrapped → `*http.MaxBytesError` → 413); sniff `detectImageType(data[:min(sniffLen,len)])`; allowlist-check (415); `stripped, err := assets.StripMetadata(contentType, data)` → on error `writeError(400, "could not process image")`; THEN create item + asset row and `assetStore.Put(asset.ID, bytes.NewReader(stripped), s.assetMaxByte)`; `byte_size = len(stripped)`. (Doing strip before row creation means a bad image never creates orphan rows; the existing blob-write-failure cleanup stays for disk errors.)

- [ ] **Step 5: handler tests** — update `assets_test.go`: AVIF fixture → 415; upload a JPEG-with-EXIF → 201, then `GET /assets/{id}` bytes contain no `"Exif\x00\x00"`. Keep the existing 413/415(svg,text)/nosniff/cross-tenant/bad-uuid tests green.

- [ ] **Step 6: `go test -p 1 ./... && golangci-lint run ./...` green; commit** `feat(assets): strip exif/metadata from uploads (lossless), drop avif`.

---

### Task 2: E2E + docs

**Files:** modify `docs/self-hosting.md`, `TODO.md`, `docs/superpowers/specs/20260704-image-upload-design.md` (note the shipped strip + avif removal)

- [ ] **Step 1: box e2e.** Fresh build on the server. Build a JPEG-with-EXIF in a scratch script (Go: encode + inject APP1 `Exif\0\0` + fake GPS bytes; write `/tmp/geo.jpg`). Login (cookie), `POST /api/assets` `-F file=@/tmp/geo.jpg` → 201; `GET /api/assets/<id>` → download bytes → assert `grep -c $'Exif\0\0'` == 0 and the file still opens as a valid JPEG (`python3 -c "from struct... "` or just check `FFD8...FFD9` and that it's servable). AVIF: craft a tiny avif-branded ISOBMFF → `POST /api/assets` → 415. Record outputs. Stop api/web (leave db + volume).
- [ ] **Step 2: docs.** self-hosting.md: replace the "EXIF not stripped" caveat with "uploaded images have EXIF/XMP/IPTC stripped losslessly on upload; AVIF uploads are rejected (415) pending lossless AVIF support". TODO.md: EXIF follow-up → Done (dated, evidence); note AVIF-support as a small Next item. Update the image-upload spec's "Out of scope"/security notes to reflect the shipped state.
- [ ] **Step 3: commit** `feat(assets): exif-strip e2e evidence + docs`. Controller merges, pushes, redeploys api+web, re-verifies on the box.
