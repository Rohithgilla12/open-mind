# Document capture — design

**Date:** 2026-08-07
**Status:** approved

Save a `.docx`, `.odt`, `.rtf`, or `.epub` as a first-class card. The file
converts to Markdown via the [anydoc](https://github.com/firecrawl/anydoc) Rust
crate compiled to `wasm32-wasip1` and run under wazero, then feeds the existing
classify → summarise → embed pipeline unchanged.

Today `internal/enrich` handles HTML (readability/trafilatura/Jina) and PDF
(`internal/pdftext`, pdfium-on-wazero). Documents are the gap: `/assets`
accepts images and PDFs only.

## Decisions

| Question | Decision |
|---|---|
| Formats | `.docx`, `.odt`, `.rtf`, `.epub`. No spreadsheets, presentations, or CSV. |
| Wasm artefact | Built by a pinned script, committed, `go:embed`-ed. No Rust toolchain needed to build Openmind. |
| `items.body` | Flattened plain text. Raw Markdown additionally stored in a new `items.body_markdown` column. |
| Card type | `.epub` → `book`; the rest → `article`. No contract change. |
| URL routing | Upload-only. A URL serving a `.docx` keeps falling through to the article extractor. |
| Clients | API + web drop zone + mobile share sheet. Extension unchanged. |
| Wasm boundary | WASI stdio module, fresh instance per call. |

### Why `body_markdown` rather than Markdown in `body`

`item.body` is the canonical plain text of an item, consumed by ten places,
none of which render Markdown:

| Consumer | Location |
|---|---|
| FTS index | `0009_tag_stemming.sql:20` — `to_tsvector('english', left(body, 100000))` |
| Embeddings | `enrich/pipeline.go:231` |
| AI summarise + tag | `enrich/pipeline.go:216,220` |
| Send to Kindle | `api/kindle.go:83`, `epub/epub.go:15` — plain text, **HTML-escaped** on render |
| MCP server | `mcp/tools.go:158`, `mcp/resources.go:36` |
| Highlights | `0014_highlights.sql` — anchored by `exact`/`prefix`/`suffix` + `offsetHint` |
| Related | `api/related.go:27` |
| Reader mode | `item/[id]/read/page.tsx:59` — `body.split(/\n\n+/)` |
| Item detail / QuoteReader | `item/[id]/page.tsx:194,406` |
| Reading time | `lib/text` `readingMinutes` |

Markdown in `body` would put literal `##` and `|---|` into Kindle EPUBs and pull
syntax characters into highlight quotes. Flattening keeps all ten working
untouched.

`body_markdown` is a convenience, not a recoverability mechanism: the original
file is retained as an asset permanently, so Markdown can always be regenerated
by re-running the job. It exists so a future Markdown-aware reader mode or a
structure-preserving EPUB export needs no re-extraction pass.

## Spike results (2026-08-07)

Both headline risks were retired before any Openmind code was written.

- **anydoc builds to `wasm32-wasip1` unmodified** — 84 crates, no patching, on
  rustc 1.89.0.
- **Artefact is 5.2 MB** uncompressed, below the 10 MB threshold at which
  committing a binary would have been reconsidered in favour of a separate Go
  module.
- **Round-trip verified through wazero** for all four formats.

| Format | Conversion |
|---|---|
| `.docx` | 2.7 ms |
| `.odt` | 1.2 ms |
| `.rtf` | 0.5 ms |
| `.epub` | 1.5 ms |

- **Module compilation costs 3.08 s.** This is the one measurement that shapes
  the design: `docmd` must compile lazily on first use behind a `sync.Once`, not
  eagerly at worker boot. Most worker instances never see a document and must
  not pay it.

anydoc's public API is `to_markdown_bytes(bytes, format) -> Result<String>`,
with `Format::from_extension` and content-based `Format::from_bytes`. It is
in-memory, so the wrapper needs no WASI filesystem access.

## Components

### New

| Path | Purpose |
|---|---|
| `tools/anydoc-wasi/` | Rust wrapper crate: stdin bytes + format arg → Markdown on stdout. `anydoc` pinned exactly. |
| `scripts/build-anydoc-wasm.sh` | `cargo build --target wasm32-wasip1 --release` → `apps/api/internal/docmd/anydoc.wasm`. Documented, not part of `task build`. |
| `apps/api/internal/docmd/` | `docmd.go` (`Converter`, `New`, `Convert`), `anydoc.wasm` (`go:embed`), `flatten.go`, tests + fixtures. |
| `apps/api/internal/enrich/doc.go` | `runUploadedDoc`, `persistDoc`, `failDoc` — mirrors `pdf.go`. |
| `apps/api/internal/store/migrations/0023_body_markdown.sql` | `ALTER TABLE items ADD COLUMN body_markdown text` (nullable). |

### Modified

- `internal/api/assets.go` — document allowlist, `detectDocType`, routing.
- `internal/enrich/pipeline.go` — dispatch the `/assets/` branch on asset content type.
- `internal/store/queries/items.sql` — `SetItemBodyMarkdown`.
- `openapi.yaml` — `createAsset` description only. `body_markdown` is **not**
  exposed; nothing reads it yet, and adding it later is a normal contract change.
- `apps/web/components/ImageDrop.tsx` — accept list and copy.
- `apps/mobile` share extension — document UTIs / intent filters.
- `docs/self-hosting.md` — documents are stored verbatim, metadata included.
- `apps/web/app/architecture/page.tsx` — new pipeline input; bump `LAST_UPDATED`.

`docmd` is the only package that knows anydoc exists. Callers see
`Result{Title, Markdown, Text}`, mirroring how `pdftext` isolates pdfium.

## Format detection

`.docx`, `.odt`, and `.epub` are all ZIP containers, so
`http.DetectContentType` returns `application/zip` for each.
`detectDocType` disambiguates without decompressing:

- **RTF** — magic prefix `{\rtf`.
- **ODT / EPUB** — both store an uncompressed `mimetype` member first; read it
  directly (`application/vnd.oasis.opendocument.text`, `application/epub+zip`).
- **DOCX** — no mimetype member; open the central directory with `archive/zip`
  and require a `word/document.xml` entry.

Only entry *names* are read while sniffing, never decompressed content, so the
sniff itself cannot be zip-bombed. `MaxBytesReader` still bounds the request.

Go's detection serves the allowlist and card-type mapping; the detected format
is then passed explicitly to anydoc rather than letting it re-detect, so a file
that sniffs as `docx` in Go can never be parsed as a spreadsheet we chose not to
support.

## Data flow

**Capture — must stay instant.** `CreateAsset` → sniff → allowlist → store blob
verbatim → `CreateItem` → asset row → `SetItemURL("/assets/<uuid>")` →
`UpdateItemExtraction(title = filename stem, cardType = book|article)` → enqueue
`EnrichArgs` → `201`. No conversion in the request path.

**Enrich — worker.** `pipeline.Run` sees the `/assets/` prefix, loads the asset
through the user-scoped `GetAssetByItem`, and switches on `ContentType`:
`application/pdf` → existing `runUploadedPDF`; document types → `runUploadedDoc`.
This is the only behavioural change to an existing path and preserves current
PDF behaviour exactly.

`runUploadedDoc`: read blob → `docmd.Convert` → flattened text to `body`, raw
Markdown to `body_markdown`, title = first H1 if present else the filename stem
→ `enrichText` (summarise → tag → embed → status), untouched.

**Idempotency.** Conversion and flattening are pure functions of the stored
bytes, so a re-run reproduces byte-identical state. `page_count` stays null —
documents have no meaningful page count.

## Error handling

`failDoc` mirrors `failPDF`: flip status to `failed`, wrap with stage context,
leave the item and asset row intact. Corrupt or unconvertible input fails the
item; River retries transient errors.

Three bounds mirror `pdftext`: a 30 s ceiling per document, a 10 MB cap on
accumulated text, and `WithCloseOnContextDone(true)` so a wedged conversion is
interruptible. One property the pooled PDF path cannot offer as cleanly: the
wasm instance carries an explicit memory limit, so a decompression bomb traps
inside the sandbox and returns an error rather than pressuring the host. The
instance is discarded either way — there is no pool, because conversion is
stateless and has no document handles to keep alive.

Uploaded documents are stored **verbatim**, like PDFs. They carry author and
organisation metadata that is not stripped; this needs the same note in
`docs/self-hosting.md` that PDFs already have.

## Testing

- `docmd`: real fixtures per format asserting extracted text; corrupt input
  errors; an empty document yields empty text and no error; context
  cancellation honoured; an oversized archive errors rather than OOMs.
- `flatten`: table-driven over headings, tables, lists, emphasis, code blocks.
- `enrich`: a fake `DocConverter` so tests never boot wasm, exactly as
  `PDFTexter` is faked today — including the mandatory run-twice idempotency
  test.
- `api`: each accepted type → 201 with the right card type; a non-document ZIP
  → 415; PDF and image paths unregressed.
- `store`: `SetItemBodyMarkdown` against real Postgres.

## Not in scope

URL routing for documents; spreadsheets, presentations, and CSV; exposing
`bodyMarkdown` in the OpenAPI contract; Markdown-aware reader mode; extension
changes.

## Remaining risks

- **anydoc is v0.1.7** — early software, pinned exactly, upgrades deliberate.
- **Mobile cannot be verified without a fresh dev build**, consistent with
  existing notes on native-module testing. The share-extension change ships
  unverified on device until then.
