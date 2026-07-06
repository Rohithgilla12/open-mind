# Send to Kindle — Design

Date: 2026-07-07 · Status: Designed autonomously (user asleep; standing "milestones over milestones" directive) · Milestone 4's last item (PRD §10: "Send to Kindle (single item + Lens digest)")

## Goal

Read your saved articles on a Kindle: send one item, or a whole Lens's current matches as a digest, to your Kindle address. Amazon ingests **EPUB via email**; Openmind already archives full text, so this is packaging + delivery.

## Principles honoured

- **No new required service, no new dependency.** EPUB is a zip of XHTML — built with `archive/zip` + `html/template` (stdlib). Email via `net/smtp` (stdlib; STARTTLS + PLAIN auth). SMTP is **optional config**: unconfigured instances return a clear error and everything else keeps working.
- **Async, capture-sacred-adjacent:** the endpoint enqueues a River job (`send_kindle`) and returns **202** immediately; rendering + SMTP happen in the worker. Job payload is IDs only; state fetched fresh in the job.
- Single-user v1: the Kindle address + SMTP come from env. When real multi-user auth lands, the Kindle address moves to a per-user setting (documented).

## Config (env, documented in docs/self-hosting.md)

`SMTP_HOST`, `SMTP_PORT` (587 STARTTLS default; 465 implicit TLS; 25/plain allowed for local relays), `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM` (must be a Kindle-approved sender), `KINDLE_EMAIL` (the user's @kindle.com address). Feature is "configured" iff HOST+FROM+KINDLE_EMAIL are set (auth optional — some relays are IP-allowed).

## Contract (openapi.yaml)

- `POST /items/{id}/kindle` → **202** `{queued: true}`; **404** unknown/cross-tenant; **409** `{"error":"kindle is not configured — set SMTP_HOST, SMTP_FROM and KINDLE_EMAIL"}` when unconfigured; **422** when the item has no body to send (empty archive, e.g. failed extraction or image card).
- `POST /lenses/{id}/kindle` → **202**; **404**; **409** unconfigured; **422** when the Lens currently matches nothing with a body.
- operationIds `sendItemToKindle`, `sendLensToKindle`. Go + TS regenerated.

## Packages

- **`internal/epub`** (new, stdlib-only): `Build(w io.Writer, doc Document) error` where `Document{Title, Author, Chapters []Chapter{Title, HTMLBody}}`. Emits a minimal valid EPUB 3: `mimetype` (stored, uncompressed, first entry), `META-INF/container.xml`, `OEBPS/content.opf` (dc:title/creator/language, manifest, spine), `OEBPS/chapter-N.xhtml` (escaped/templated body paragraphs), `OEBPS/nav.xhtml`. Archived bodies are plain text paragraphs (extraction output) — rendered as `<p>` blocks with html escaping; no remote images.
- **`internal/mailer`** (new, stdlib-only): `Send(cfg SMTPConfig, msg Message{To, Subject, BodyText, Attachment{Filename, ContentType, Data}}) error` — builds the MIME multipart (base64 attachment) with `mime/multipart` + `encoding/base64`, dials per port (465 `tls.Dial`, else `smtp.Dial` + STARTTLS when the server offers it), PLAIN auth when configured. Interface `Mailer` so the job is testable with a fake; the real one is used in `cmd` wiring.
- **`internal/jobs`**: `SendKindleArgs{UserID, ItemID *uuid, LensID *uuid}` (exactly one set) + worker: fetch item(s) fresh (Lens → `runLensRule`-equivalent via a small server-side seam → items with non-empty bodies, cap 25 chapters), build EPUB (item: title = item title; digest: "Openmind digest — <lens name> — YYYY-MM-DD"), `Mailer.Send` to `KINDLE_EMAIL`. Errors → River retry (transient SMTP) with max attempts 5; a permanently empty document never enqueues (guarded at the handler).

## Handlers

`internal/api/kindle.go`: ownership check via user-scoped Get (404), config check (409), emptiness check (422 — item body empty; lens matches have no bodies), enqueue → 202. The handler needs the SMTP config presence only (not the secrets — the worker reads full config), so `Server` gains a `kindleConfigured bool` + the lens-run seam already exists (`runLensRule`).

## Web

- Detail page action row: **"Send to Kindle"** button (posts, then shows "Sent to your Kindle ✓ (arrives shortly)" on 202; specific copy for 409 "Not configured — see self-hosting docs" and 422). Cookie-proxy route `apps/web/app/api/items/[id]/kindle/route.ts`.
- Lens view header: **"Send digest"** button, same states; proxy `apps/web/app/api/lenses/[id]/kindle/route.ts`.

## Testing

- `internal/epub`: build a two-chapter doc → unzip in-test → assert `mimetype` first + stored + exact content, container.xml points at the opf, opf lists every chapter, each xhtml parses (`encoding/xml` well-formedness) and contains the escaped body (`<script>` input arrives escaped).
- `internal/mailer`: an in-process TCP fake SMTP server (goroutine speaking just enough SMTP: greeting, EHLO, MAIL/RCPT/DATA, 250s) captures the DATA payload → assert headers, boundary structure, base64 attachment decodes byte-identical, no STARTTLS on the plain path. (No network, no container.)
- Jobs: fake `Mailer` — item job sends one EPUB with the item title; lens job caps chapters and skips bodyless items; unconfigured never reaches Send (handler guard tested at the API layer: 409/422/404/202 table).
- Compose e2e: 202 path with a **Mailpit container run ad hoc during the test only** (dev tooling, not part of the deployment) OR — simpler and chosen — set SMTP_* to the host's in-test fake? Compose can't reach a host fake portably; instead e2e asserts the 409 unconfigured path + the 202 path with `SMTP_HOST=mailpit` via a temporary `--profile test` service documented as dev-only. If that drags, the Go-level fake-SMTP integration test is the send-path authority and compose e2e covers 409/404/422 only — acceptable, noted honestly.

## Out of scope

Per-user Kindle addresses (needs real auth), scheduled/automatic digests, EPUB cover images, images inside articles (text-only v1), MOBI (dead), send history/UI log.

## Execution

Subagent-driven, 3 tasks: (1) `internal/epub` + `internal/mailer` + tests (pure stdlib, no wiring); (2) contract + job + handlers + config wiring + API tests; (3) web buttons + proxies + compose e2e + docs (self-hosting SMTP section) + TODO. Deploy batched with the VPS-recovery queue.
