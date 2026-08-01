# Search Query Redesign (Phase 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lenses (and their Kindle/MCP paths) run a structured Query with hard filters for `domains` + `scope: library` by default, so “all my x.com saves” and “library only” work without feed-river leakage.

**Architecture:** Add a generated `items.url_host` column; introduce `search.Query` (text/color rank + types/domains/scope filters); push filters into SQL before RRF; extend `LensRule` in OpenAPI; default omitted Lens scope to `library`. Phase 2 (unify `/search` UI + ParseQuery domains) is **out of scope** — file a follow-up plan after Phase 1 lands.

**Tech Stack:** Postgres generated column, sqlc, oapi-codegen, Go search package, Next.js `LensForm`.

**Spec:** `docs/superpowers/specs/20260801-search-query-redesign.md`

## Global Constraints

- Single-binary / Postgres-only — no Redis, Elasticsearch, or sidecars.
- noop-safe: domain/type/scope filters must work with `ai.NewNoop()` / Fake without embeddings.
- Contract-first: edit `openapi.yaml`, then `task generate` — never hand-edit `packages/api-client` or oapi/sqlc output.
- Mind / library predicate: `(feed_id IS NULL OR kept_at IS NOT NULL)`.
- Lens default scope = `library` (supersedes feed-river design’s “Lenses include unkept feed”). Home `GET /search` stays `scope=all` in Phase 1 (no API param yet).
- At least one of `q` / `color` / `types` / `domains` required; `scope` alone is invalid.
- Keep field name `q` in LensRule JSON (not `text`) for stored-rule compatibility.
- Migration number: **0021** (verify no newer migration landed; next free after `0020_notifications.sql`).
- Go tests from `apps/api` with `TEST_DATABASE_URL`; prefer `-p 1` when DB-backed suites race. UK English. No banner comments.

## File map

| File | Role |
|------|------|
| `apps/api/internal/store/migrations/0021_url_host.sql` | `items_url_host()` + generated `url_host` + index |
| `apps/api/internal/store/queries/search.sql` | Filtered FTS/vector/palette + filter-only list |
| `apps/api/internal/search/query.go` | `Query` type, `NormalizeDomain`, `NormalizeDomains` |
| `apps/api/internal/search/search.go` | `RunQuery` entrypoint; retire river-special `RunLensRule` path |
| `openapi.yaml` | `LensRule.domains`, `LensRule.scope` |
| `apps/api/internal/api/lenses.go` | parse/marshal domains+scope; default library |
| `apps/api/internal/jobs/kindle.go` | Pass full rule into `RunLensRule` / `RunQuery` |
| `apps/api/internal/mcp/tools.go` | create_lens description + rule fields |
| `apps/web/components/LensForm.tsx` | Domains field; validation copy |
| `apps/web/app/lens/new/page.tsx` | Seed `domains` from searchParams |
| `docs/20260801-lens-walkthrough-script.md` | Domain recipe replaces “don’t type x.com” |

---

### Task 1: `url_host` migration + filtered SQL queries

**Files:**
- Create: `apps/api/internal/store/migrations/0021_url_host.sql`
- Modify: `apps/api/internal/store/queries/search.sql`
- Run: `task generate` (sqlc only is fine; OpenAPI unchanged this task)

**Interfaces → Produces:**
- Column `items.url_host` (nullable text, generated)
- sqlc params on search queries: `LibraryOnly bool`, `Types []string` (nil = no type filter), `Domains []string` (nil = no domain filter)
- New query `ListItemsMatching` for filter-only (no text/colour) paths

- [x] **Step 1: Write migration**

```sql
-- Extract lowercased host from a URL-ish string; strip leading www.
-- Empty / unparseable → NULL so domain filters never false-match notes/quotes.
CREATE OR REPLACE FUNCTION items_url_host(u text) RETURNS text AS $$
  SELECT NULLIF(
    regexp_replace(
      lower(
        substring(
          u
          from '(?i)^(?:[a-z][a-z0-9+.-]*:)?(?://)?(?:[^@/\s]+@)?([^:/?\s#]+)'
        )
      ),
      '^www\.',
      ''
    ),
    ''
  );
$$ LANGUAGE SQL IMMUTABLE PARALLEL SAFE;

ALTER TABLE items
  ADD COLUMN url_host text
  GENERATED ALWAYS AS (items_url_host(url)) STORED;

CREATE INDEX items_url_host_idx ON items (user_id, url_host)
  WHERE url_host IS NOT NULL;
```

Generated column backfills existing rows automatically — no separate UPDATE.

- [x] **Step 2: Replace / extend `search.sql`**

Shared filter fragment (document in comments; paste into each query):

```sql
-- library_only: when true, Mind predicate
-- types: NULL or '{}' means no type filter; else card_type = ANY(types)
-- domains: NULL or '{}' means no domain filter; else host equals or is subdomain
```

Update `SearchFTS`:

```sql
-- name: SearchFTS :many
SELECT *, ts_rank(search_tsv, websearch_to_tsquery('english', $2))::float8 AS rank
FROM items
WHERE user_id = $1
  AND search_tsv @@ websearch_to_tsquery('english', $2)
  AND (
    NOT sqlc.arg(library_only)::bool
    OR feed_id IS NULL
    OR kept_at IS NOT NULL
  )
  AND (
    sqlc.narg(filter_types)::text[] IS NULL
    OR cardinality(sqlc.narg(filter_types)::text[]) = 0
    OR card_type = ANY (sqlc.narg(filter_types)::text[])
  )
  AND (
    sqlc.narg(filter_domains)::text[] IS NULL
    OR cardinality(sqlc.narg(filter_domains)::text[]) = 0
    OR EXISTS (
      SELECT 1
      FROM unnest(sqlc.narg(filter_domains)::text[]) AS d(domain)
      WHERE url_host = d.domain
         OR url_host LIKE d.domain || '.%'
    )
  )
ORDER BY rank DESC
LIMIT $3;
```

Mirror the same three AND blocks on `SearchVector` (join path — filter on `i.`), `ListItemsWithPalette`, and add:

```sql
-- name: ListItemsMatching :many
-- Filter-only listing (types and/or domains; optional library scope).
-- Newest first. Used when a Query has no text/colour rank signal.
SELECT *
FROM items
WHERE user_id = sqlc.arg(user_id)
  AND (
    NOT sqlc.arg(library_only)::bool
    OR feed_id IS NULL
    OR kept_at IS NOT NULL
  )
  AND (
    sqlc.narg(filter_types)::text[] IS NULL
    OR cardinality(sqlc.narg(filter_types)::text[]) = 0
    OR card_type = ANY (sqlc.narg(filter_types)::text[])
  )
  AND (
    sqlc.narg(filter_domains)::text[] IS NULL
    OR cardinality(sqlc.narg(filter_domains)::text[]) = 0
    OR EXISTS (
      SELECT 1
      FROM unnest(sqlc.narg(filter_domains)::text[]) AS d(domain)
      WHERE url_host = d.domain
         OR url_host LIKE d.domain || '.%'
    )
  )
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count);
```

If sqlc rejects `narg` + `arg` naming, adjust to match existing `items.sql` narg style (`sqlc.narg(filter_feed_id)`). Prefer nullable slices: pass `nil` from Go when filter unused.

- [x] **Step 3: Generate**

```bash
task generate
```

Expected: `db.Item` gains `UrlHost`; search query params gain filter fields. Fix any sqlc name clashes before continuing.

- [x] **Step 4: Smoke-check host extraction** (optional psql against test DB after migrate):

```sql
SELECT items_url_host('https://www.x.com/foo'), items_url_host('https://mobile.twitter.com/a'), items_url_host('');
-- expect: x.com | mobile.twitter.com | NULL
```

- [x] **Step 5: Commit**

```bash
git add apps/api/internal/store/migrations/0021_url_host.sql \
  apps/api/internal/store/queries/search.sql \
  apps/api/internal/store/db/
git commit -m "$(cat <<'EOF'
feat(search): add url_host column and filtered search SQL

EOF
)"
```

---

### Task 2: OpenAPI `LensRule` domains + scope

**Files:**
- Modify: `openapi.yaml` (`LensRule` schema; optionally bump `/lenses/{id}/items` description)
- Run: `task generate`

**Interfaces → Produces:** generated Go `LensRule` with `Domains *[]string`, `Scope *LensRuleScope` (or string enum `library` | `all`).

- [x] **Step 1: Extend schema**

Replace `LensRule` description/properties with:

```yaml
    LensRule:
      type: object
      description: >
        A saved search Query. At least one of q, color, types, or domains must be set.
        Rank signals: q (FTS+vector), color (palette proximity). Hard filters: types,
        domains (URL host / subdomain), scope (library = Mind only; all = include feed river).
        Omitted scope defaults to library when the rule is run as a Lens.
      properties:
        q: { type: string, description: "Free-text query (rank)." }
        color: { type: string, description: "Hex (#RRGGBB) or named colour (e.g. cobalt)." }
        types:
          type: array
          description: "Card types to include (filter)."
          items: { type: string, enum: [article, product, book, recipe, video, tweet, image, note, quote] }
        domains:
          type: array
          description: "URL hosts to include (filter). Normalised lowercase; www stripped. Subdomains match (x.com matches mobile.x.com)."
          items: { type: string, minLength: 1 }
        scope:
          type: string
          description: "library = saved/kept only; all = include unkept feed items. Lens runs default to library when omitted."
          enum: [library, all]
```

Update `GET /lenses/{id}/items` description to say matches are library-scoped by default.

Do **not** add `/search` query params in this task (Phase 2).

- [x] **Step 2: `task generate`**

- [x] **Step 3: Commit**

```bash
git add openapi.yaml apps/api/internal/api/gen.go packages/api-client/
git commit -m "$(cat <<'EOF'
feat(api): extend LensRule with domains and scope

EOF
)"
```

---

### Task 3: `search.Query` + domain normalisation (unit tests first)

**Files:**
- Create: `apps/api/internal/search/query.go`
- Create: `apps/api/internal/search/query_test.go`

**Interfaces → Produces:**

```go
package search

type Scope string

const (
	ScopeLibrary Scope = "library"
	ScopeAll     Scope = "all"
)

// Query is the structured search/Lens request: soft rank signals + hard filters.
type Query struct {
	Text    string
	Color   string
	Types   []string
	Domains []string // already normalised hosts
	Scope   Scope    // empty means caller must set default before RunQuery
}

func NormalizeDomain(raw string) (string, bool) // ok=false if empty/invalid
func NormalizeDomains(raw []string) []string    // dedupe, skip invalid
func (q Query) HasMatchSignal() bool            // text|color|types|domains
func (q Query) LibraryOnly() bool               // Scope != ScopeAll
```

- [x] **Step 1: Write failing tests** in `query_test.go`

```go
func TestNormalizeDomain(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"x.com", "x.com", true},
		{"https://www.X.com/foo", "x.com", true},
		{"HTTP://Twitter.com", "twitter.com", true},
		{"  www.example.co.uk ", "example.co.uk", true},
		{"", "", false},
		{"not a host", "", false}, // spaces / garbage
		{"http://", "", false},
	}
	for _, tc := range cases {
		got, ok := NormalizeDomain(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("NormalizeDomain(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestNormalizeDomainsDedupe(t *testing.T) {
	got := NormalizeDomains([]string{"x.com", "https://www.x.com/a", "twitter.com", ""})
	// expect ["x.com","twitter.com"] order-preserving first-seen
}
```

- [x] **Step 2: Run tests — expect FAIL** (undefined)

```bash
cd apps/api && go test ./internal/search/ -run TestNormalize -count=1
```

- [x] **Step 3: Implement `query.go`**

Use `net/url.Parse` after ensuring a scheme (`https://` prefix if missing `://`). Take `Hostname()`, `strings.ToLower`, trim `www.` prefix. Reject empty host or hosts containing spaces.

- [x] **Step 4: Run tests — expect PASS**

- [x] **Step 5: Commit**

```bash
git add apps/api/internal/search/query.go apps/api/internal/search/query_test.go
git commit -m "$(cat <<'EOF'
feat(search): add Query type and domain normalisation

EOF
)"
```

---

### Task 4: Engine — `RunQuery` with SQL filters; library-default Lens path

**Files:**
- Modify: `apps/api/internal/search/search.go`
- Modify: `apps/api/internal/search/search_test.go`
- Modify: callers — `apps/api/internal/api/lenses.go` (`runLensRule`), `apps/api/internal/api/server.go` (`SearchItems` → keep wrapping `Run` with `ScopeAll`), `apps/api/internal/jobs/kindle.go`, `apps/api/internal/api/mcp.go` if it calls `RunLensRule`

**Interfaces:**

```go
// RunQuery executes q. Caller must set Scope (Lens: library; /search: all).
// Returns ErrBadColor for invalid colour; empty results if HasMatchSignal is false.
func RunQuery(ctx context.Context, s *store.Store, p ai.Provider, userID uuid.UUID, q Query, limit int) ([]Result, error)

// Prefer updating RunLensRule to take Query:
func RunLensRule(ctx context.Context, s *store.Store, p ai.Provider, userID uuid.UUID, q Query) ([]Result, error)
// which sets Scope=library when q.Scope=="" and calls RunQuery(..., ruleResultLimit).
```

Prefer **updating `RunLensRule` to take `Query`** and fix all call sites in this task. Keep `Run` / `Hybrid` as thin wrappers:

```go
func Run(ctx ..., q, color string, types []string, limit int) ([]Result, error) {
	return RunQuery(ctx, s, p, userID, Query{
		Text: q, Color: color, Types: types, Scope: ScopeAll,
	}, limit)
}
```

- [x] **Step 1: Write failing DB tests** in `search_test.go`

`TestDomainFilterMatchesHostAndSubdomain` — create items with URLs `https://x.com/a`, `https://mobile.twitter.com/b`, `https://example.com/c` (use CreateItem + set card_type); `RunQuery` with `Domains:["x.com"]` → only first; `Domains:["twitter.com"]` → second (subdomain); both domains → first two.

`TestLibraryScopeExcludesUnkeptFeed` — seed unkept feed item (copy pattern from `api/feedriver_test.go` CreateFeedItem) with matching type; `ScopeLibrary` + `Types:["article"]` → absent; `ScopeAll` → present.

`TestFilterOnlyDomainsUsesListPath` — domains only, no text/color → results unscored / newest; still respects library scope.

- [x] **Step 2: Run tests — expect FAIL**

```bash
cd apps/api && go test ./internal/search/ -run 'TestDomain|TestLibraryScope|TestFilterOnly' -count=1
```

- [x] **Step 3: Implement `RunQuery`**

Logic sketch:

```go
func RunQuery(...) ([]Result, error) {
	if !q.HasMatchSignal() {
		return nil, nil
	}
	libraryOnly := q.LibraryOnly()
	types := q.Types
	domains := q.Domains
	// nil vs empty: pass nil to sqlc when len==0 so narg skips filter

	if q.Text == "" && q.Color == "" {
		rows, err := s.Queries.ListItemsMatching(ctx, db.ListItemsMatchingParams{
			UserID: userID, LibraryOnly: libraryOnly,
			FilterTypes: typesOrNil(types), FilterDomains: domainsOrNil(domains),
			LimitCount: int32(limit),
		})
		// map to Result{Item: row}
		return ...
	}

	// else: existing RRF path but pass LibraryOnly/FilterTypes/FilterDomains
	// into SearchFTS, SearchVector, ListItemsWithPalette
	// REMOVE post-fusion types filter (now in SQL)
	// KEEP library-first sort among results when ScopeAll (feed still ranks after library)
}
```

Delete `ListItemsAll` usage from `RunLensRule`. Update `RunLensRule` comment: Lenses default to library scope.

- [x] **Step 4: Fix call sites**

`lenses.go` `runLensRule`: build `search.Query` from `normalisedRule`, `Scope: search.ScopeLibrary` unless rule says `all`.

`kindle.go`: build `Query` from stored LensRule JSON (including domains/scope once Task 5 lands; for now types/q/color + library default).

`SearchItems`: continue calling `Run` / `RunQuery` with `ScopeAll`.

- [x] **Step 5: Run full search + lens packages**

```bash
cd apps/api && go test ./internal/search/ ./internal/api/ -count=1 -p 1
```

Expect `TestLensTypesOnlyIncludesUnkeptFeedItem` to FAIL until Task 5 flips it — that red test is expected; do not skip. Task 5 flips the assertion.

- [x] **Step 6: Commit**

```bash
git add apps/api/internal/search/ apps/api/internal/api/lenses.go \
  apps/api/internal/jobs/kindle.go apps/api/internal/api/mcp.go \
  apps/api/internal/api/server.go
git commit -m "$(cat <<'EOF'
feat(search): RunQuery with domain and library scope filters

EOF
)"
```

---

### Task 5: API rule parse/marshal + flip Lens feed test

**Files:**
- Modify: `apps/api/internal/api/lenses.go` (`normalisedRule`, `parseRule`, `marshalRule`, `decodeStoredRule`)
- Modify: `apps/api/internal/api/lenses_internal_test.go`
- Modify: `apps/api/internal/api/lenses_test.go` (`TestLensTypesOnlyIncludesUnkeptFeedItem` → excludes)
- Modify: `apps/api/internal/mcp/tools.go` (create_lens description + input struct if rule is typed)
- Modify: `docs/superpowers/specs/20260716-feed-river-design.md` — one-line note that Lens inclusion was superseded 2026-08-01

**Interfaces → Produces:**

```go
type normalisedRule struct {
	q, color string
	types, domains []string
	scope search.Scope // "" in storage means library at run time
}
```

- [x] **Step 1: Extend `TestParseRule`**

Add cases:
- `domains only` → ok
- `domains+types`
- `bad domain` rejected (`NormalizeDomain` fails)
- `scope library` / `scope all` / `invalid scope` rejected
- `domains-only` satisfies HasMatchSignal (empty q/color/types ok)
- empty still rejected

- [x] **Step 2: Implement parse/marshal**

```go
// parseRule: normalise domains via search.NormalizeDomains; if any raw domain
// produced zero normalised and raw was non-empty → error "rule.domains contains an invalid host"
// scope: only "", "library", "all"; store as search.Scope
// validity: q=="" && color=="" && len(types)==0 && len(domains)==0 → error
//   "... at least one of q, color, types, or domains"
```

`marshalRule`: omit empty scope (so old clients / DB stay compact); omit empty domains.

`runLensRule`:

```go
q := search.Query{
	Text: rule.q, Color: rule.color, Types: rule.types, Domains: rule.domains,
	Scope: rule.scope,
}
if q.Scope == "" {
	q.Scope = search.ScopeLibrary
}
return search.RunLensRule(ctx, s.store, s.provider, uid, q)
```

- [x] **Step 3: Flip integration test**

Rename to `TestLensTypesOnlyExcludesUnkeptFeedItem`. Assert unkept feed item is **absent** from `GET /lenses/{id}/items`. Keep assertion that `GET /items` also excludes it. Add sibling `TestLensScopeAllIncludesUnkeptFeedItem` posting `{"rule":{"types":["article"],"scope":"all"}}` and asserting present.

- [x] **Step 4: Add `TestLensDomainFilter`** — create Mind items on x.com + example.com; lens `domains:["x.com"]`; only x.com returned.

- [x] **Step 5: Run**

```bash
cd apps/api && go test ./internal/api/ -run Lens -count=1
```

- [x] **Step 6: Commit**

```bash
git add apps/api/internal/api/lenses.go apps/api/internal/api/lenses_internal_test.go \
  apps/api/internal/api/lenses_test.go apps/api/internal/mcp/tools.go \
  docs/superpowers/specs/20260716-feed-river-design.md
git commit -m "$(cat <<'EOF'
feat(lenses): validate domains/scope; default library-only matches

EOF
)"
```

---

### Task 6: Web `LensForm` — domains field

**Files:**
- Modify: `apps/web/components/LensForm.tsx`
- Modify: `apps/web/app/lens/new/page.tsx` (accept `domains` searchParam)
- Modify: `docs/20260801-lens-walkthrough-script.md` (domain recipe)

**Interfaces:** POST body `rule: { q?, color?, types?, domains? }` — omit scope (server defaults library).

- [x] **Step 1: Form state**

Add `domains` string state (comma-separated input is fine for v1), seeded from `initialDomains?: string[]` joined by `, `.

`hasRule` = query OR colour OR types OR normalised domains length > 0.

Helper copy:

> Add a query, a colour, a domain, or at least one card type — a lens needs something to match.

- [x] **Step 2: Submit**

```ts
const domainList = domains
  .split(/[\s,]+/)
  .map((d) => d.trim())
  .filter(Boolean);
if (domainList.length) rule.domains = domainList;
```

Server normalises/validates; show API error string on 400.

- [x] **Step 3: UI block** (place after Query, before Colour)

Label `DOMAINS`, placeholder `x.com, twitter.com`, mono hint under field: `Host only — subdomains match`.

- [x] **Step 4: `lens/new/page.tsx`**

```ts
searchParams: Promise<{ q?: string; color?: string; types?: string; domains?: string }>
// initialDomains = domains?.split(",").filter(Boolean) ?? []
```

- [x] **Step 5: Update walkthrough doc**

Replace “Don’t type x.com into the Query field” with: use Domains `x.com, twitter.com` (and optional Post type). Update reference JSON example to include domains. Scene 4 can stay types-only as an alternate demo.

- [x] **Step 6: Typecheck**

```bash
pnpm --filter web exec tsc --noEmit
```

- [x] **Step 7: Commit**

```bash
git add apps/web/components/LensForm.tsx apps/web/app/lens/new/page.tsx \
  docs/20260801-lens-walkthrough-script.md
git commit -m "$(cat <<'EOF'
feat(web): domain field on new Lens form

EOF
)"
```

---

### Task 7: Phase 1 verification checklist

- [x] **Step 1: Migrate + API tests**

```bash
export DATABASE_URL=postgres://openmind:openmind@localhost:5433/openmind
export TEST_DATABASE_URL=postgres://openmind:openmind@localhost:5433/openmind_test
cd apps/api && go test ./internal/search/ ./internal/api/ ./internal/jobs/ -count=1 -p 1
```

Expected: PASS (including flipped Lens feed test + domain tests).

- [x] **Step 2: Manual smoke** (dev stack up)

1. Save `https://x.com/some/status/1` and an article URL; wait for enrich (or noop classify still sets tweet for x.com).
2. New Lens → Domains `x.com` → Save → only X URL appears.
3. Subscribe a feed, leave item unkept with type article; Lens types `article` → unkept feed item absent.
4. API: `POST /lenses` with `"scope":"all"` → unkept feed item appears.

- [x] **Step 3: Note Phase 2 follow-ups** in PR description (not code): `/search?types&domains&scope`, ParseQuery domains, FilterStrip Post/Recipe, Save-as-lens seeds domains, retire client `?type=` filter.

---

## Spec coverage (self-review)

| Spec requirement | Task |
|------------------|------|
| `url_host` column + subdomain match | 1, 4 |
| Query rank vs filter | 3, 4 |
| LensRule domains + scope | 2, 5 |
| Lens default library | 4, 5 |
| Types/domains-only via list not ListItemsAll | 4 |
| LensForm domains | 6 |
| noop-safe filters | 4 tests with Fake/Noop |
| Flip feed-river Lens decision | 5 |
| Walkthrough doc | 6 |
| Phase 2 `/search` + ParseQuery | deferred (explicit) |

## Out of scope (do not implement in this plan)

- `GET /search` structured params / web home FilterStrip / ParseQuery `Domains`
- Date/tag filters, reranker, Lens pin overrides
- Scope toggle in LensForm UI (API-only `scope: all` is enough for Phase 1)
