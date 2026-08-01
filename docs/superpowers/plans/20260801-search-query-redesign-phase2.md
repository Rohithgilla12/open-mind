# Search Query Redesign (Phase 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unify `/search` with the structured Query model — expose `types`/`domains`/`scope` as API params, teach ParseQuery to extract domains, and wire the web home search UI to send filters server-side (retire client-only `?type=` post-filter).

**Architecture:** Extend OpenAPI `/search` + `UnderstoodQuery`; `SearchItems` merges explicit params with `ParseQuery` (explicit wins); web `getSearch` forwards `type`→`types`, optional `domains`, default search `scope=all`; FilterStrip gains Post + Recipe; Save-as-lens seeds domains.

**Tech Stack:** openapi + oapi-codegen, Go AI ParseQuery, Next.js home page.

**Spec:** `docs/superpowers/specs/20260801-search-query-redesign.md` § Phase 2

## Global Constraints

- Explicit query params win over parsed values (same as colour today).
- `/search` default `scope` = `all` (keep feed divider UX). Omitted scope = all.
- At least one of `q`, `color`, `types`, `domains` required (not scope alone).
- noop: ParseQuery still returns `{Text: q}` only; structured params work without AI.
- Contract-first; `task generate`; UK English; no banner comments.
- Do not change Lens library default (Phase 1).

---

### Task 1: OpenAPI `/search` params + UnderstoodQuery.domains

**Files:** `openapi.yaml`; `task generate`

- [x] **Step 1:** Update `GET /search`:
  - Description: structured filters + parse extracts text/colour/types/domains
  - Params: `types` (array of card-type enum, style: form, explode: true OR comma — prefer explode true for oapi), `domains` (array of string), `scope` (enum library|all)
  - At least one of q, color, types, domains required (document; enforce in handler)

- [x] **Step 2:** `UnderstoodQuery` gains `domains: string[]`

- [x] **Step 3:** `task generate` + commit `feat(api): expose types, domains, scope on /search`

---

### Task 2: SearchItems handler + ParseQuery Domains

**Files:**
- `apps/api/internal/ai/ai.go` — `ParsedQuery.Domains`, prompt, `sanitiseDomains` using `search.NormalizeDomain`
- `apps/api/internal/ai/gemini.go`, `openai.go` — decode domains from JSON
- `apps/api/internal/ai/noop.go`, `fake.go` — unchanged behaviour (empty domains)
- `apps/api/internal/api/server.go` — SearchItems merge + RunQuery
- `apps/api/internal/api/mcp.go` / `mcp/tools.go` — pass through if Search helper exists
- Tests: `server_test.go`, `ai` parse tests, new search param tests

**Handler logic:**

```go
// Collect explicit types/domains/scope from params (normalise domains).
// If parse && q: ParseQuery → fill text; fill color/types/domains only where explicit empty.
// Require HasMatchSignal on final Query (text|color|types|domains).
// Scope: explicit or ScopeAll.
// understood echoes what was actually searched (incl domains).
// search.RunQuery(..., q, defaultListLimit)
```

Prompt update — four parts JSON:
`{"text","color","types","domains"}`  
Example: `"posts from x.com about shoes"` → text≈shoes, domains=["x.com"], types may include tweet.

- [x] **Step 1:** Failing tests for explicit `types`/`domains`, parse domains, explicit wins over parse
- [x] **Step 2:** Implement AI + handler
- [x] **Step 3:** `go test ./internal/ai/ ./internal/api/ -run 'Search|Parse' -count=1`
- [x] **Step 4:** Commit `feat(search): structured /search params and ParseQuery domains`

---

### Task 3: Web home — server-side filters + FilterStrip + Save-as-lens

**Files:**
- `apps/web/app/page.tsx` — getSearch(q, color, types, domains); no client filter
- `apps/web/components/FilterStrip.tsx` — add Posts (tweet), Recipes (recipe); preserve domains in links
- `apps/web/components/SearchContext.tsx` — domain chips in understood echo; seed lensQuery.domains; preserve type/domains in colour hrefs
- `apps/web/app/api/search/route.ts` — proxy query string as-is (verify)

URL params stay `?type=` (singular) for FilterStrip UX; map to API `types=tweet`. Optional `?domains=x.com,twitter.com` for future/manual.

- [x] **Step 1:** Wire getSearch to pass types/domains; remove `fetched.filter(cardKind)`
- [x] **Step 2:** FilterStrip Post + Recipe; keep `type` in links with q/color/domains
- [x] **Step 3:** SearchContext domains echo + save-as-lens
- [x] **Step 4:** `pnpm --filter web exec tsc --noEmit`
- [x] **Step 5:** Commit `feat(web): server-side search filters and domain-aware Save as lens`

---

### Task 4: Verify

```bash
export TEST_DATABASE_URL=postgres://openmind:openmind@localhost:5433/openmind_test
cd apps/api && go test ./internal/search/ ./internal/api/ ./internal/ai/ ./internal/jobs/ -count=1 -p 1
pnpm --filter web exec tsc --noEmit
```

Push + update PR.

## Out of scope

- LensForm scope toggle
- Date/tag filters, reranker
- Changing search default scope away from `all`
