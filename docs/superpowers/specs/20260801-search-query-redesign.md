# Search & Lens query redesign — structured Query as the spine

Date: 2026-08-01. Approved direction: reimagine search around one structured
`Query` type shared by `/search`, Lenses, MCP, and Kindle digests. Rank soft
signals separately from hard filters. Closes the gaps that make “all my x.com
saves” and “library only” awkward today.

Supersedes the Lens/feed decision in
`docs/superpowers/specs/20260716-feed-river-design.md` § Semantics
(“Lenses … include unkept feed items”) — Lenses default to library scope.

## Problem

Search grew as three bolted-on signals (`q`, `color`, types-via-AI-parse)
with inconsistent surfaces:

| Surface | Types | Scope | Domain |
|---------|-------|-------|--------|
| `GET /search` | only via `ParseQuery` | library-first ranking, still includes feed | none |
| Home `?type=` | client-only post-filter | — | — |
| Lens rule | explicit in JSON | includes feed on purpose | none |

FTS (`search_tsv`) indexes title / summary / tags / body / user_tags — **not
URL**. Typing `x.com` into Query does not find X links. The workaround is the
**Post** card type (`enrich.Classify` maps `x.com` / `twitter.com` /
`mobile.twitter.com` → `tweet`), which conflates “is a tweet-shaped card”
with “URL is on this host”.

## Goals

1. **One Query model** for live search and saved Lenses (same shape, same
   engine path).
2. **Hard filters vs soft rank** — filters never leak through ranking; rank
   never pretends to be a filter.
3. **Domain filter** — match saved items by URL host (e.g. all `x.com` /
   `twitter.com` links), independent of card type.
4. **Scope** — Lenses default to library-only (direct saves + kept feed
   items); home search can still span the river when desired.
5. **noop-safe** — structured filters + FTS work with no AI; `ParseQuery`
   only *fills* the structure from NL when a provider is configured.
6. **Single-binary / Postgres-only** — no new search infra.

## Non-goals (this redesign)

- Optional reranker stage (PRD later; leave a seam, don’t build).
- Date / tag / “similar to” as first-class filters (schema reserved or
  deferred to a follow-up; Phase 1 does not require them).
- Pin/unpin overrides on Lenses (PRD §5.5; separate).
- Replacing hybrid FTS + pgvector + colour RRF — keep the ranker; fix the
  query contract around it.
- Indexing raw URL strings into `search_tsv` as the domain mechanism (flaky
  stemming / tokenisation); use a structured host column instead.

## Design

### Query shape

```text
Query {
  // Rank signals (soft) — fused with RRF when present
  text?:  string      // FTS + optional vector (today’s q)
  color?: string      // palette ΔE (today’s color)

  // Filters (hard) — applied in SQL (preferred) or post-filter before limit
  types?:   string[]  // card types ⊆ schema enum
  domains?: string[]  // registrable / host match (normalised, lowercase)
  scope?:   "library" | "all"   // default depends on surface (below)
}
```

**At least one** of `text`, `color`, `types`, or `domains` must be set (a
query needs something to match). `scope` alone is not enough.

**LensRule** becomes this Query (OpenAPI: extend `LensRule` with `domains`
and `scope`; keep `q` as the text field name for backwards compatibility with
stored jsonb — see Compatibility).

**`GET /search`** accepts the same structured fields as query params:
`q`, `color`, `types` (repeatable or comma-separated), `domains`, `scope`,
`parse`. Today’s `parse=true` path fills missing structured fields from
`ParseQuery`; explicit params win over parsed ones (same precedence as
explicit `color` today).

### Rank vs filter (engine)

```text
NL or form  →  Query  →  SQL hard filters (scope, types, domains)
                              ↓
                     candidate set (user-scoped)
                              ↓
                     RRF: FTS ∪ vector ∪ colour   (only signals present)
                              ↓
                     limit → results
```

Changes vs today (`apps/api/internal/search/search.go`):

1. **Filters before / outside RRF**, not after fusion when possible — so a
   types+domains rule doesn’t waste rank slots on items that will be dropped.
2. **Types-only / domains-only** (no text/colour): list newest matching
   items under filters (no RRF scores), same caps as today (scan/result
   limits), via library-aware list queries — **not** `ListItemsAll`.
3. **`Run` and `RunLensRule` share one entrypoint** that takes `Query`;
   delete the “deliberately span the river” special case.

### Domain matching

- Persist a generated or maintained **`url_host`** (or `domain`) column on
  `items`, lowercased host from `url` (strip leading `www.`).
- Match rule: item host equals any requested domain **or** is a subdomain of
  it (`x.com` matches `x.com` and `mobile.x.com`; `twitter.com` listed
  separately when the user wants both).
- Normalise user input: lowercase, strip scheme/path, strip leading `www.`.
- `ParseQuery` may extract domains from phrases like “from x.com” /
  “tweets on twitter” into `domains` (and may still set `types: [tweet]`
  when the user said “posts/tweets” — complementary, not either/or).

**x.com recipe (product):** Lens with
`domains: ["x.com", "twitter.com"]` (and optionally `types: ["tweet"]` if
the user wants only post-shaped cards). Prefer domains as the primary
“all links from X” answer.

### Scope semantics

| Value | Predicate |
|-------|-----------|
| `library` | `feed_id IS NULL OR kept_at IS NOT NULL` (The Mind) |
| `all` | no feed predicate (today’s search/Lens behaviour) |

**Defaults:**

| Surface | Default `scope` |
|---------|-----------------|
| Lens create / `RunLensRule` / Kindle Lens digest / MCP `run_lens` | `library` |
| `GET /search` (home, Quick Find, MCP search) | `all` (keep feed divider UX on web home) |

Stored Lenses with omitted `scope` read as `library` after this change
(behaviour flip from the 2026-07-16 feed-river decision). Document in
changelog / self-hosting notes if user-facing.

### ParseQuery

Extend `ai.ParsedQuery` with `Domains []string` (and pass through `scope`
only if the model clearly says “in my library” / “including feeds” —
otherwise leave unset so surface defaults apply).

Prompt update: extract domains as bare hosts; do not invent domains.
`noop`: `{ Text: q }` only — unchanged.

Lenses continue to **store the concrete Query**, not the raw NL string — no
re-parse on open (stable live views).

### Web UI (phased)

**Phase 1 — engine + Lenses (ship value fast)**

- `LensForm`: domain input (comma-separated hosts or chips) + scope is
  implicit library (no control required in v1; power users get `scope` via
  API).
- Helper copy: query / colour / type / domain — at least one required.
- Flip tests that asserted river inclusion for Lenses.

**Phase 2 — unify `/search` UI**

- Home search sends `types` / `domains` / `scope` as real API params (retire
  client-only `?type=` post-filter).
- FilterStrip includes Post + Recipe; domain chip when understood or typed.
- “Save as lens” seeds the full Query including domains + scope.

### Compatibility

- Existing Lens jsonb `{ "q", "color", "types" }` remains valid; missing
  `domains` = none; missing `scope` = `library` (new default).
- OpenAPI: extend `LensRule` and `/search` parameters; regenerate client.
- MCP tool schemas updated to match.

## Phasing

| Phase | Deliverable |
|-------|-------------|
| **1** | Query model in OpenAPI + Go; `url_host` migration; engine filters (types, domains, scope); Lenses + Kindle + MCP use library default; LensForm domains; tests flipped/added |
| **2** | `/search` exposes structured params; web home uses them; ParseQuery extracts domains; FilterStrip parity |
| **Later** | date / tags filters, rerank seam, colour SQL approx, Lens pin overrides |

Phase 1 alone answers: “all my x.com saves” and “Lens = my library only”.

## Testing

- Domain: save `https://x.com/a`, `https://twitter.com/b`, `https://example.com/c`;
  rule `domains:["x.com"]` returns only the first; `["x.com","twitter.com"]`
  returns first two; subdomain `mobile.twitter.com` matches `twitter.com`.
- Scope: unkept feed item matching type/domain appears under `scope=all`,
  absent under `scope=library` (and under default Lens path).
- Types+domains conjunction; text+domain still ranks within filtered set.
- noop: domain/type/scope filters work without embeddings or ParseQuery.
- Idempotent migration backfill of `url_host` for existing rows.
- Regenerate + contract tests for OpenAPI.

## Key files

- `openapi.yaml` — `LensRule`, `/search`, `UnderstoodQuery`
- `apps/api/internal/search/search.go` — Query entrypoint, filters, drop
  river-special-case in `RunLensRule`
- `apps/api/internal/store/queries/search.sql` + `items.sql` — host column,
  filtered lists
- `apps/api/internal/store/migrations/` — next free migration
- `apps/api/internal/api/lenses.go`, `server.go` (SearchItems), `ai/ai.go`
- `apps/web/components/LensForm.tsx`, later `SearchContext` / `page.tsx` /
  `FilterStrip`
- `docs/20260801-lens-walkthrough-script.md` — update “Don’t type x.com”
  caveat once domains ship

## Decision log

- **2026-08-01** — Structured Query (rank + filters); domain column not FTS;
  Lenses default `scope: library` (supersedes feed-river Lens inclusion);
  ship Phase 1 then Phase 2; keep Postgres hybrid ranker.
