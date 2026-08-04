# Infinite Scroll Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add keyset (cursor) pagination to `GET /items` and `GET /feed`, and consume it as infinite scroll on the web Mind, the web Feed river, the mobile Library tab, and the mobile Feed tab.

**Architecture:** The 200 body of both endpoints becomes an `ItemPage` envelope — `{ items, nextCursor }` — where `nextCursor` is an opaque base64url token encoding the last row's `(created_at, id)`. Queries seek with a row-value comparison against that key rather than counting with `OFFSET`, so a save landing at the head between pages cannot re-serve a row the client already holds. Handlers over-fetch by one to make `nextCursor` precise. Web appends one CSS-multi-column block per page so already-rendered cards never reflow; mobile moves to `useInfiniteQuery`.

**Tech Stack:** Go 1.x (chi, sqlc, pgx v5, oapi-codegen), Postgres, Next.js 15 App Router (React server + client components), Expo / React Native with TanStack Query v5.101.2, vitest (web/dock), jest-expo (mobile).

**Spec:** `docs/superpowers/specs/20260804-infinite-scroll-design.md`

## Global Constraints

- `openapi.yaml` is the contract and comes first: edit it, run `task generate`, then implement. Never hand-write API types in TS; never add a Go route absent from the spec.
- Never hand-edit generated code: `packages/api-client/src/schema.d.ts`, `apps/api/internal/store/db/*.sql.go`, `apps/api/internal/api/gen.go`.
- All SQL goes through sqlc in `internal/store/queries/`. Every store method takes `ctx` and is scoped by `user_id`.
- Wrap Go errors with `fmt.Errorf("doing x: %w", err)`.
- Migrations are applied lexicographically by filename inside a transaction (`internal/store/migrate.go`). **Do not use `CREATE INDEX CONCURRENTLY`** — it cannot run inside a transaction. Next free number is `0022` (note `0021` is already used twice: `0021_repo_card_type.sql` and `0021_url_host.sql`).
- Page size stays `defaultListLimit = 50`, ceiling `maxListLimit = 200` (`internal/api/server.go:35`).
- Web vitest is **node-only** and matches `lib/**/*.test.ts` — no component mounting. Mobile jest matches `.ts` only, so no component tests there either. Put logic worth testing in pure `lib/` modules.
- Go tests run against real Postgres, serialised: `cd apps/api && go test -p 1 ./...`. Default test DSN `postgres://openmind:openmind@localhost:5433/openmind_test`, overridable via `TEST_DATABASE_URL`.
- Design tokens come from `packages/ui` — never hardcode colours in apps.
- No new required infrastructure. No AI calls in a save's request path.
- UK English in user-facing copy.
- Do not use decorative banner comments (`// ==== Section ====`). Comments explain *why*, not *what*.

---

### Task 1: Cursor codec

Pure encode/decode for the keyset position. No dependencies on other tasks, so it goes first.

**Files:**
- Create: `apps/api/internal/api/cursor.go`
- Test: `apps/api/internal/api/cursor_internal_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type pageCursor struct { CreatedAt time.Time; ID uuid.UUID }`
  - `func encodeCursor(c pageCursor) string`
  - `func decodeCursor(s *string) (*pageCursor, error)` — returns `(nil, nil)` when `s` is nil or empty (meaning "first page"); returns an error wrapping `errInvalidCursor` for anything undecodable.
  - `var errInvalidCursor error`

The test file is `_internal_test.go` and declares `package api` (not `api_test`), matching `ratelimit_internal_test.go` and `lenses_internal_test.go`, because these identifiers are unexported.

- [ ] **Step 1: Write the failing test**

Create `apps/api/internal/api/cursor_internal_test.go`:

```go
package api

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCursorRoundTrip(t *testing.T) {
	// Microsecond precision: Postgres stores microseconds, so a cursor built
	// from a row read back out must survive the round trip exactly.
	want := pageCursor{
		CreatedAt: time.Date(2026, 8, 4, 12, 34, 56, 123456000, time.UTC),
		ID:        uuid.MustParse("11111111-2222-3333-4444-555555555555"),
	}
	tok := encodeCursor(want)
	if tok == "" {
		t.Fatal("encodeCursor returned empty string")
	}
	for _, c := range tok {
		if c == '=' || c == '+' || c == '/' {
			t.Errorf("token contains %q; must be URL-safe and unpadded: %s", c, tok)
		}
	}

	got, err := decodeCursor(&tok)
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if got == nil {
		t.Fatal("decodeCursor returned nil for a valid token")
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, want.CreatedAt)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %v, want %v", got.ID, want.ID)
	}
}

func TestDecodeCursorAbsent(t *testing.T) {
	empty := ""
	for name, in := range map[string]*string{"nil": nil, "empty": &empty} {
		t.Run(name, func(t *testing.T) {
			got, err := decodeCursor(in)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if got != nil {
				t.Errorf("got %+v, want nil (meaning first page)", got)
			}
		})
	}
}

func TestDecodeCursorInvalid(t *testing.T) {
	cases := map[string]string{
		"not base64":     "!!!not-base64!!!",
		"no separator":   "MjAyNi0wOC0wNA",
		"bad time":       encodeRaw("not-a-time|11111111-2222-3333-4444-555555555555"),
		"bad uuid":       encodeRaw("2026-08-04T12:34:56Z|not-a-uuid"),
		"empty both":     encodeRaw("|"),
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			tok := tok
			got, err := decodeCursor(&tok)
			if !errors.Is(err, errInvalidCursor) {
				t.Errorf("err = %v, want errInvalidCursor", err)
			}
			if got != nil {
				t.Errorf("got %+v, want nil on error", got)
			}
		})
	}
}
```

Add this helper at the bottom of the same test file so the invalid-input cases can build tokens whose *contents* are wrong rather than whose encoding is wrong:

```go
func encodeRaw(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}
```

and add `"encoding/base64"` to that file's imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/api && go test ./internal/api/ -run TestCursor -run TestDecodeCursor -v`

Expected: FAIL — compile error, `undefined: pageCursor`, `undefined: encodeCursor`, `undefined: decodeCursor`, `undefined: errInvalidCursor`.

- [ ] **Step 3: Write the implementation**

Create `apps/api/internal/api/cursor.go`:

```go
package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// errInvalidCursor marks a cursor we cannot decode. Handlers turn it into a
// 400 rather than falling back to page 1: silently serving the top of the list
// to a client that believes it paged forward would hide the bug and duplicate
// rows in its list.
var errInvalidCursor = errors.New("invalid cursor")

// pageCursor is the keyset position of the last row of a page — the sort key of
// ORDER BY created_at DESC, id DESC. id breaks ties because created_at is not
// unique.
type pageCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// encodeCursor renders a keyset position as an opaque token. The timestamp is
// formatted from the value read out of the row, so it round-trips exactly
// against Postgres's microsecond storage. RawURLEncoding keeps the token free
// of '=' and '+', so it needs no escaping in a query string.
func encodeCursor(c pageCursor) string {
	raw := c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor parses a token produced by encodeCursor. A nil cursor with a nil
// error means no cursor was supplied, i.e. the caller wants the first page.
func decodeCursor(s *string) (*pageCursor, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*s)
	if err != nil {
		return nil, fmt.Errorf("decoding cursor: %w", errInvalidCursor)
	}
	// RFC3339Nano contains no '|', so a left split of two is unambiguous.
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("cursor missing separator: %w", errInvalidCursor)
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, fmt.Errorf("parsing cursor timestamp: %w", errInvalidCursor)
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, fmt.Errorf("parsing cursor id: %w", errInvalidCursor)
	}
	return &pageCursor{CreatedAt: ts, ID: id}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/api && go test ./internal/api/ -run 'TestCursorRoundTrip|TestDecodeCursor' -v`

Expected: PASS (3 tests, 7 subtests).

- [ ] **Step 5: Commit**

```bash
git add apps/api/internal/api/cursor.go apps/api/internal/api/cursor_internal_test.go
git commit -m "feat(api): opaque keyset cursor codec"
```

---

### Task 2: Contract — `ItemPage` envelope and `cursor` params

Changes the contract and regenerates. Ends with the whole monorepo compiling; the handlers still emit bare arrays (fixed in Task 4), which is called out in the commit message so the intermediate state is not mistaken for done.

**Files:**
- Modify: `openapi.yaml` (the `/items` `get`, the `/feed` `get`, and `components.schemas`)
- Modify: `apps/web/lib/types.ts:3-4`
- Regenerated (do not hand-edit): `apps/api/internal/api/gen.go`, `packages/api-client/src/schema.d.ts`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - Go: `type ItemPage struct { Items []Item; NextCursor *string }`
  - Go: `ListItemsParams` gains `Cursor *string`; `GetFeedItemsParams` gains `Cursor *string`
  - TS: `paths["/items"]["get"]["responses"]["200"]["content"]["application/json"]` is now the envelope, so `Item` is reached via `["items"][number]`

- [ ] **Step 1: Add `cursor` to `GET /items` and switch its 200 to `ItemPage`**

In `openapi.yaml`, replace the `/items` `get` block (currently lines 33-43):

```yaml
    get:
      operationId: listItems
      parameters:
        - { name: limit, in: query, schema: { type: integer, default: 50 } }
        - { name: cursor, in: query, schema: { type: string }, description: "Opaque cursor from a previous page's nextCursor. Omit for the first page." }
      responses:
        "200":
          content:
            application/json:
              schema: { $ref: "#/components/schemas/ItemPage" }
        "400": { description: invalid cursor }
```

- [ ] **Step 2: Add `cursor` to `GET /feed` and switch its 200 to `ItemPage`**

Replace the `/feed` `get` block (currently lines 485-495):

```yaml
    get:
      operationId: getFeedItems
      parameters:
        - { name: limit, in: query, schema: { type: integer } }
        - { name: feedId, in: query, schema: { type: string, format: uuid } }
        - { name: cursor, in: query, schema: { type: string }, description: "Opaque cursor from a previous page's nextCursor. Omit for the first page." }
      responses:
        "200":
          description: feed-originated items, newest first
          content:
            application/json:
              schema: { $ref: "#/components/schemas/ItemPage" }
        "400": { description: invalid cursor }
```

- [ ] **Step 3: Add the `ItemPage` schema**

Under `components.schemas`, immediately after the `Item` schema, add:

```yaml
    ItemPage:
      type: object
      description: >
        One page of items, newest first. nextCursor is opaque — pass it back as
        ?cursor= to fetch the following page. It is absent when there are no
        more items, so its presence is the only "has more" signal a client needs.
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/Item" }
        nextCursor: { type: string }
```

- [ ] **Step 4: Regenerate**

Run: `task generate`

Expected: succeeds. `apps/api/internal/api/gen.go` now declares `ItemPage`, and both param structs carry `Cursor *string`. Verify:

```bash
grep -n "type ItemPage struct" -A 4 apps/api/internal/api/gen.go
grep -n "type ListItemsParams struct" -A 4 apps/api/internal/api/gen.go
grep -n "type GetFeedItemsParams struct" -A 5 apps/api/internal/api/gen.go
```

- [ ] **Step 5: Confirm the web type derivation now fails**

Run: `cd apps/web && pnpm exec tsc --noEmit`

Expected: FAIL in `lib/types.ts` — the 200 body is an object, so indexing it with `[number]` is invalid. This is the intended loud failure.

- [ ] **Step 6: Repoint the web `Item` type**

In `apps/web/lib/types.ts`, replace lines 3-4:

```ts
export type Item =
  paths["/items"]["get"]["responses"]["200"]["content"]["application/json"]["items"][number];

export type ItemPage =
  paths["/items"]["get"]["responses"]["200"]["content"]["application/json"];
```

- [ ] **Step 7: Verify the monorepo compiles and existing tests still pass**

Run:
```bash
cd apps/web && pnpm exec tsc --noEmit
cd ../api && go build ./...
cd ../.. && pnpm turbo run test
```

Expected: `tsc` clean, `go build` clean, JS tests pass.

- [ ] **Step 8: Commit**

```bash
git add openapi.yaml apps/api/internal/api/gen.go packages/api-client/src/schema.d.ts apps/web/lib/types.ts
git commit -m "feat(api): ItemPage envelope + cursor param on /items and /feed

Contract and generated clients only. Handlers still emit bare arrays and are
brought into conformance in the following commit."
```

---

### Task 3: Keyset store queries and the index

**Files:**
- Modify: `apps/api/internal/store/queries/items.sql` (`ListItems` at :10-13, `ListFeedItems` at :27-31)
- Create: `apps/api/internal/store/migrations/0022_items_keyset_index.sql`
- Test: `apps/api/internal/store/items_page_test.go`
- Regenerated (do not hand-edit): `apps/api/internal/store/db/items.sql.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `db.ListItemsParams{ UserID uuid.UUID; CursorCreatedAt pgtype.Timestamptz; CursorID pgtype.UUID; LimitCount int32 }`
  - `db.ListFeedItemsParams{ UserID uuid.UUID; FilterFeedID pgtype.UUID; CursorCreatedAt pgtype.Timestamptz; CursorID pgtype.UUID; LimitCount int32 }`
  - Both return `[]db.Item` ordered `created_at DESC, id DESC`.

Note `ListItems`'s limit field is renamed `Limit` → `LimitCount` by the switch to `sqlc.arg(limit_count)`, which keeps it consistent with `ListFeedItems`. Task 4 updates the callers.

- [ ] **Step 1: Write the failing store test**

Create `apps/api/internal/store/items_page_test.go`:

```go
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

func TestListItemsKeysetPagesWholeSet(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	base := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	const total = 7
	for i := 0; i < total; i++ {
		it, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
			UserID: userID, Url: "https://example.com/" + string(rune('a'+i)), Body: "",
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		// Spread created_at so ordering is unambiguous, newest last-created.
		if _, err := s.Pool().Exec(ctx,
			`UPDATE items SET created_at = $2 WHERE id = $1`,
			it.ID, base.Add(time.Duration(i)*time.Minute),
		); err != nil {
			t.Fatalf("backdate %d: %v", i, err)
		}
	}

	// Page through in 3s and assert every row is seen exactly once.
	seen := map[uuid.UUID]int{}
	var cursorAt pgtype.Timestamptz
	var cursorID pgtype.UUID
	for page := 0; page < 10; page++ {
		rows, err := s.Queries.ListItems(ctx, db.ListItemsParams{
			UserID: userID, CursorCreatedAt: cursorAt, CursorID: cursorID, LimitCount: 3,
		})
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			seen[r.ID]++
		}
		last := rows[len(rows)-1]
		cursorAt = last.CreatedAt
		cursorID = pgtype.UUID{Bytes: last.ID, Valid: true}
	}

	if len(seen) != total {
		t.Errorf("saw %d distinct items, want %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("item %s served %d times, want exactly 1", id, n)
		}
	}
}

func TestListItemsKeysetBreaksTiesOnID(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	// Two items sharing created_at exactly, straddling a page boundary of 1.
	same := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	for _, u := range []string{"https://example.com/x", "https://example.com/y"} {
		it, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: u, Body: ""})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := s.Pool().Exec(ctx, `UPDATE items SET created_at = $2 WHERE id = $1`, it.ID, same); err != nil {
			t.Fatalf("backdate: %v", err)
		}
	}

	first, err := s.Queries.ListItems(ctx, db.ListItemsParams{UserID: userID, LimitCount: 1})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first page had %d rows, want 1", len(first))
	}
	second, err := s.Queries.ListItems(ctx, db.ListItemsParams{
		UserID:          userID,
		CursorCreatedAt: first[0].CreatedAt,
		CursorID:        pgtype.UUID{Bytes: first[0].ID, Valid: true},
		LimitCount:      1,
	})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second page had %d rows, want 1 (identical created_at must not swallow a row)", len(second))
	}
	if second[0].ID == first[0].ID {
		t.Error("second page repeated the first page's row")
	}
}

func TestListItemsKeysetStableWhenHeadGrows(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	base := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		it, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
			UserID: userID, Url: "https://example.com/old" + string(rune('a'+i)), Body: "",
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := s.Pool().Exec(ctx, `UPDATE items SET created_at = $2 WHERE id = $1`,
			it.ID, base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("backdate: %v", err)
		}
	}

	first, err := s.Queries.ListItems(ctx, db.ListItemsParams{UserID: userID, LimitCount: 2})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}

	// A capture lands at the head between requests. Under OFFSET 2 this would
	// shift the window and re-serve first[1]; keyset must be immune.
	if _, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
		UserID: userID, Url: "https://example.com/brand-new", Body: "",
	}); err != nil {
		t.Fatalf("create new: %v", err)
	}

	second, err := s.Queries.ListItems(ctx, db.ListItemsParams{
		UserID:          userID,
		CursorCreatedAt: first[len(first)-1].CreatedAt,
		CursorID:        pgtype.UUID{Bytes: first[len(first)-1].ID, Valid: true},
		LimitCount:      2,
	})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	for _, a := range first {
		for _, b := range second {
			if a.ID == b.ID {
				t.Errorf("item %s appeared on both pages after a head insert", a.ID)
			}
		}
	}
}
```

These tests need raw pool access to set an explicit `created_at` — no production query writes that column, and relying on insert timing would make the ordering and tie-break assertions flaky. Step 2 adds the accessor.

- [ ] **Step 2: Expose the pool on `Store` for tests that need raw SQL**

`apps/api/internal/store/store.go` currently only holds `Queries`. Read it, then add:

```go
// Pool exposes the underlying connection pool. Tests use it to set up rows that
// no query needs to write in production — notably an explicit created_at, so
// ordering and tie-breaking are deterministic rather than timing-dependent.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }
```

Match the existing field name for the pool; if `Store` stores the pool under a different name, use that. Add the `pgxpool` import if absent.

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd apps/api && go test ./internal/store/ -run TestListItemsKeyset -v`

Expected: FAIL — compile error, `unknown field CursorCreatedAt in struct literal of type db.ListItemsParams`.

- [ ] **Step 4: Rewrite the two queries**

In `apps/api/internal/store/queries/items.sql`, replace `ListItems` (lines 10-13):

```sql
-- name: ListItems :many
-- Keyset (not OFFSET) pagination: the seek is anchored to a row, so a capture
-- landing at the head between two requests cannot shift a window and re-serve
-- a row the client already holds. id breaks created_at ties.
SELECT * FROM items
WHERE user_id = $1 AND (feed_id IS NULL OR kept_at IS NOT NULL)
  AND (
    sqlc.narg(cursor_created_at)::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);
```

and replace `ListFeedItems` (lines 27-31):

```sql
-- name: ListFeedItems :many
SELECT * FROM items
WHERE user_id = $1 AND feed_id IS NOT NULL
  AND (sqlc.narg(filter_feed_id)::uuid IS NULL OR feed_id = sqlc.narg(filter_feed_id))
  AND (
    sqlc.narg(cursor_created_at)::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);
```

- [ ] **Step 5: Add the index migration**

Create `apps/api/internal/store/migrations/0022_items_keyset_index.sql`:

```sql
-- The list sort key gained an id tiebreaker, so the index has to carry it for
-- the keyset seek to stay index-only.
--
-- CONCURRENTLY is deliberately absent: migrate.go runs each file inside a
-- transaction, and CREATE INDEX CONCURRENTLY cannot run in one.
CREATE INDEX items_user_created_id_idx ON items (user_id, created_at DESC, id DESC);

-- items_user_created_idx (0001_init.sql) is a strict prefix of the index above,
-- so every query it served is still served. Keeping both would pay the write
-- cost twice for nothing.
DROP INDEX items_user_created_idx;
```

- [ ] **Step 6: Regenerate sqlc and run the tests**

Run:
```bash
task generate
cd apps/api && go test ./internal/store/ -run TestListItemsKeyset -v
```

Expected: the three tests PASS. If `go build ./...` fails in `internal/api` because `ListItems` callers still pass `Limit:`, that is expected and fixed in Task 4 — but the store package's own tests compile and pass.

- [ ] **Step 7: Verify the index is actually used**

Run:
```bash
cd apps/api && psql "${TEST_DATABASE_URL:-postgres://openmind:openmind@localhost:5433/openmind_test}" \
  -c "EXPLAIN SELECT * FROM items WHERE user_id = gen_random_uuid() ORDER BY created_at DESC, id DESC LIMIT 50;"
```

Expected: an `Index Scan` (or `Index Only Scan`) using `items_user_created_id_idx`, not a `Seq Scan` with a `Sort`. On an empty table Postgres may prefer a seq scan; if so, note it and move on — the plan shape is what matters here.

- [ ] **Step 8: Commit**

```bash
git add apps/api/internal/store/queries/items.sql \
        apps/api/internal/store/migrations/0022_items_keyset_index.sql \
        apps/api/internal/store/db/items.sql.go \
        apps/api/internal/store/store.go \
        apps/api/internal/store/items_page_test.go
git commit -m "feat(api): keyset pagination on the item list queries

Sort key becomes (created_at DESC, id DESC) so pages cannot drop or repeat a
row when created_at ties. Replaces the (user_id, created_at DESC) index with
one carrying the tiebreaker; the old one was a strict prefix of the new."
```

---

### Task 4: Handlers emit `ItemPage`

**Files:**
- Modify: `apps/api/internal/api/server.go:261-286` (`ListItems`)
- Modify: `apps/api/internal/api/feedriver.go:15-47` (`GetFeedItems`)
- Modify: `apps/api/internal/api/mcp.go:73` (pass a nil cursor)
- Modify: `apps/api/internal/api/server_test.go:258-283` (`TestListItems` now decodes an envelope)
- Modify: `apps/api/internal/api/feedriver_test.go:236` (`ListItems` field rename)
- Test: `apps/api/internal/api/pagination_test.go`

**Interfaces:**
- Consumes: `pageCursor`, `encodeCursor`, `decodeCursor`, `errInvalidCursor` (Task 1); `db.ListItemsParams`, `db.ListFeedItemsParams` (Task 3); `ItemPage` (Task 2).
- Produces:
  - `func (s *Server) listPage(ctx context.Context, params ...) (ItemPage, error)` is **not** introduced — the two handlers each build their own page, because their queries take different params and a shared wrapper would need a callback for no gain.
  - Both endpoints return `ItemPage`, and 400 `invalid cursor` on an undecodable cursor.

- [ ] **Step 1: Write the failing handler tests**

Create `apps/api/internal/api/pagination_test.go`:

```go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// itemPage mirrors the ItemPage envelope for assertions.
type itemPage struct {
	Items      []map[string]any `json:"items"`
	NextCursor *string          `json:"nextCursor"`
}

func getPage(t *testing.T, url string) itemPage {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d for %s, want 200", resp.StatusCode, url)
	}
	var page itemPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return page
}

func TestListItemsPagesWithoutDuplicatesOrGaps(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	const total = 5
	for i := 0; i < total; i++ {
		postJSON(t, srv.URL+"/items", `{"url":"https://example.com/`+string(rune('a'+i))+`"}`).Body.Close()
		// Distinct created_at values keep the assertion about ordering honest.
		time.Sleep(2 * time.Millisecond)
	}

	seen := map[string]int{}
	url := srv.URL + "/items?limit=2"
	for i := 0; i < 10; i++ {
		page := getPage(t, url)
		for _, it := range page.Items {
			seen[it["id"].(string)]++
		}
		if page.NextCursor == nil {
			// Last page must not be empty: the limit+1 lookahead means a
			// nextCursor is only emitted when a further row really exists.
			if len(page.Items) == 0 && i > 0 {
				t.Error("final request returned an empty page; lookahead should have withheld the cursor")
			}
			break
		}
		url = srv.URL + "/items?limit=2&cursor=" + *page.NextCursor
	}

	if len(seen) != total {
		t.Errorf("saw %d distinct items, want %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("item %s served %d times, want 1", id, n)
		}
	}
}

func TestListItemsRejectsMalformedCursor(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/items?cursor=!!!not-a-cursor!!!")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (a bad cursor must not silently serve page 1)", resp.StatusCode)
	}
}

func TestFeedItemsPaginate(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	page := getPage(t, srv.URL+"/feed?limit=2")
	// No feed items seeded, so this asserts the shape rather than the contents:
	// an empty list must be [] with no cursor, never null.
	if page.Items == nil {
		t.Error("items was null; want an empty array")
	}
	if page.NextCursor != nil {
		t.Errorf("nextCursor = %q on an empty feed, want absent", *page.NextCursor)
	}

	resp, err := http.Get(srv.URL + "/feed?cursor=!!!bad!!!")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/api && go test ./internal/api/ -run 'TestListItemsPages|TestListItemsRejects|TestFeedItemsPaginate' -v`

Expected: FAIL — the body is still a bare array, so `Items` decodes as nil and `?cursor=` returns 200 rather than 400. (`go build` may also fail on the `Limit`→`LimitCount` rename; fix that in Step 3.)

- [ ] **Step 3: Rewrite `ListItems`**

Replace `apps/api/internal/api/server.go:261-286` with:

```go
// ListItems returns one page of the caller's items, newest first. Pagination is
// keyset: nextCursor encodes the last row's (created_at, id).
func (s *Server) ListItems(w http.ResponseWriter, r *http.Request, params ListItemsParams) {
	limit := listLimit(params.Limit)
	cur, err := decodeCursor(params.Cursor)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}

	ctx := r.Context()
	// Over-fetch by one so nextCursor is only emitted when a further row really
	// exists — otherwise the client always ends on a wasted empty request.
	rows, err := s.store.Queries.ListItems(ctx, db.ListItemsParams{
		UserID:          userID(ctx),
		CursorCreatedAt: cursorTimestamp(cur),
		CursorID:        cursorUUID(cur),
		LimitCount:      int32(limit + 1),
	})
	if err != nil {
		slog.Error("listing items", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list items")
		return
	}
	writeJSON(w, http.StatusOK, toItemPage(rows, limit))
}
```

- [ ] **Step 4: Add the shared helpers**

Append to `apps/api/internal/api/cursor.go`:

```go
// listLimit clamps a client-supplied limit to the house range.
func listLimit(v *int) int {
	limit := defaultListLimit
	if v != nil {
		limit = *v
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	return limit
}

// cursorTimestamp and cursorUUID render a decoded cursor as the nullable query
// args. A nil cursor yields invalid (NULL) values, which the queries read as
// "start at the newest row".
func cursorTimestamp(c *pageCursor) pgtype.Timestamptz {
	if c == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: c.CreatedAt, Valid: true}
}

func cursorUUID(c *pageCursor) pgtype.UUID {
	if c == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: c.ID, Valid: true}
}

// toItemPage trims an over-fetched row set to limit and derives nextCursor from
// the last row actually returned, so the token round-trips exactly.
func toItemPage(rows []db.Item, limit int) ItemPage {
	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		tok := encodeCursor(pageCursor{CreatedAt: last.CreatedAt.Time, ID: last.ID})
		next = &tok
	}
	items := make([]Item, 0, len(rows))
	for _, it := range rows {
		items = append(items, toAPIItem(it))
	}
	return ItemPage{Items: items, NextCursor: next}
}
```

Add `"github.com/jackc/pgx/v5/pgtype"` and `"github.com/rohithgilla12/openmind/api/internal/store/db"` to `cursor.go`'s imports.

- [ ] **Step 5: Rewrite `GetFeedItems`**

Replace the body of `GetFeedItems` in `apps/api/internal/api/feedriver.go` with:

```go
func (s *Server) GetFeedItems(w http.ResponseWriter, r *http.Request, params GetFeedItemsParams) {
	limit := listLimit(params.Limit)
	cur, err := decodeCursor(params.Cursor)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}

	var filterFeedID pgtype.UUID
	if params.FeedId != nil {
		filterFeedID = pgtype.UUID{Bytes: *params.FeedId, Valid: true}
	}

	ctx := r.Context()
	rows, err := s.store.Queries.ListFeedItems(ctx, db.ListFeedItemsParams{
		UserID:          userID(ctx),
		FilterFeedID:    filterFeedID,
		CursorCreatedAt: cursorTimestamp(cur),
		CursorID:        cursorUUID(cur),
		LimitCount:      int32(limit + 1),
	})
	if err != nil {
		slog.Error("listing feed items", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list feed items")
		return
	}
	writeJSON(w, http.StatusOK, toItemPage(rows, limit))
}
```

Update its doc comment to say it returns one page and that `nextCursor` is absent on the last page.

- [ ] **Step 6: Update the two non-paginating callers**

`apps/api/internal/api/mcp.go:73` — rename the field and leave the cursor unset (nil = newest first page), keeping MCP behaviour unchanged:

```go
	return b.s.store.Queries.ListItems(ctx, db.ListItemsParams{UserID: uid, LimitCount: int32(limit)})
```

`apps/api/internal/api/feedriver_test.go:236` — same rename:

```go
	afterList, err := s.Queries.ListItems(context.Background(), db.ListItemsParams{UserID: api.DevUserID, LimitCount: 100})
```

- [ ] **Step 7: Update `TestListItems` for the envelope**

In `apps/api/internal/api/server_test.go`, replace the decode block of `TestListItems` (lines 276-283):

```go
	var page struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(page.Items))
	}
	if page.NextCursor != nil {
		t.Errorf("nextCursor = %q with only 2 items and a limit of 50, want absent", *page.NextCursor)
	}
	if page.Items[0]["url"] != "https://example.com/second" {
		t.Errorf("newest-first ordering wrong: got %v first", page.Items[0]["url"])
	}
```

- [ ] **Step 8: Run the whole Go suite**

Run: `cd apps/api && go test -p 1 ./...`

Expected: PASS. Any other test decoding `/items` or `/feed` as a bare array will fail here — fix each by decoding the envelope, the same way as Step 7.

- [ ] **Step 9: Commit**

```bash
git add apps/api/internal/api/
git commit -m "feat(api): /items and /feed return ItemPage with a keyset cursor

Over-fetches by one so nextCursor is emitted only when a further row exists,
and a cursor that fails to decode is a 400 rather than a silent page 1."
```

---

### Task 5: Fix the silent empty-list failure in the extension and dock

Independent of the web/mobile work, and worth landing early: both clients currently turn an unrecognised body into a healthy-looking empty list.

**Files:**
- Modify: `apps/extension/lib/save.ts:102-126` (`recentItems`)
- Modify: `apps/dock/src/lib/api.ts:238-268` (`listItemsFrom`)
- Test: `apps/dock/src/lib/api.test.ts` (add cases; the file already exists)

**Interfaces:**
- Consumes: the `ItemPage` shape from Task 2 (structurally only — neither client imports generated types).
- Produces: no signature changes. `recentItems` and `listItemsFrom` keep returning `{ ok, status, items }`; `ok` becomes `false` for a body whose shape is unrecognised.

- [ ] **Step 1: Write the failing dock tests**

Read `apps/dock/src/lib/api.test.ts` around its existing `/api/items` test (near line 202) and follow its mocking style. Add:

```ts
it("reads items out of the ItemPage envelope", async () => {
  vi.mocked(fetch).mockResolvedValue(
    new Response(JSON.stringify({ items: [{ id: "1", url: "https://a.test" }], nextCursor: "abc" }), {
      status: 200,
      headers: { "content-type": "application/json" },
    }),
  );
  const res = await listRecent(8, settings);
  expect(res.ok).toBe(true);
  expect(res.items).toHaveLength(1);
});

it("still reads a bare array from an instance predating the envelope", async () => {
  vi.mocked(fetch).mockResolvedValue(
    new Response(JSON.stringify([{ id: "1", url: "https://a.test" }]), {
      status: 200,
      headers: { "content-type": "application/json" },
    }),
  );
  const res = await listRecent(8, settings);
  expect(res.ok).toBe(true);
  expect(res.items).toHaveLength(1);
});

it("reports failure rather than an empty list when the body is unrecognised", async () => {
  vi.mocked(fetch).mockResolvedValue(
    new Response(JSON.stringify({ unexpected: true }), {
      status: 200,
      headers: { "content-type": "application/json" },
    }),
  );
  const res = await listRecent(8, settings);
  // An empty library and "the server said something we do not understand"
  // must not look identical to the caller.
  expect(res.ok).toBe(false);
  expect(res.items).toEqual([]);
});
```

Use whatever `settings` fixture the surrounding tests already use.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/dock && pnpm exec vitest run src/lib/api.test.ts`

Expected: FAIL — the envelope case yields `items: []`, and the unrecognised case yields `ok: true`.

- [ ] **Step 3: Add a shared shape reader to the dock**

In `apps/dock/src/lib/api.ts`, above `listItemsFrom`, add:

```ts
/**
 * Read an item list out of either shape: the ItemPage envelope, or the bare
 * array served by an instance predating it. Returns null when the body is
 * neither, so callers can report a failure instead of an empty list — the two
 * used to be indistinguishable.
 */
function readItemList(data: unknown): Item[] | null {
  if (Array.isArray(data)) return data as Item[];
  if (data && typeof data === "object" && Array.isArray((data as { items?: unknown }).items)) {
    return (data as { items: Item[] }).items;
  }
  return null;
}
```

Then replace the parse block inside `listItemsFrom`:

```ts
    try {
      const items = readItemList(await res.json());
      if (items === null) {
        console.error("unrecognised item list body", { path });
        return { ok: false, status: res.status, items: [] };
      }
      return { ok: true, status: res.status, items };
    } catch {
      return { ok: false, status: res.status, items: [] };
    }
```

Note the `catch` also flips from `ok: true` to `ok: false`: a 200 with unparseable JSON is a failure, matching the reasoning already written into `apps/mobile/lib/api.ts:254-259`.

- [ ] **Step 4: Run the dock tests**

Run: `cd apps/dock && pnpm exec vitest run src/lib/api.test.ts`

Expected: PASS, including the pre-existing cases.

- [ ] **Step 5: Apply the same fix to the extension**

In `apps/extension/lib/save.ts`, replace the parse block of `recentItems` (lines 116-123):

```ts
    try {
      const parsed = (await res.json()) as unknown;
      const items = Array.isArray(parsed)
        ? (parsed as Item[])
        : parsed && typeof parsed === "object" && Array.isArray((parsed as { items?: unknown }).items)
          ? ((parsed as { items: Item[] }).items)
          : null;
      if (items === null) {
        console.error("unrecognised item list body");
        return { ok: false, status: res.status, items: [] };
      }
      return { ok: true, status: res.status, items };
    } catch {
      return { ok: false, status: res.status, items: [] };
    }
```

Neither client paginates: the extension shows a handful of recents, the dock shows 8. They only need to stop lying about an empty list.

- [ ] **Step 6: Typecheck both**

Run:
```bash
cd apps/extension && pnpm exec tsc --noEmit
cd ../dock && pnpm exec tsc --noEmit
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add apps/extension/lib/save.ts apps/dock/src/lib/api.ts apps/dock/src/lib/api.test.ts
git commit -m "fix(extension,dock): stop reporting an unreadable item list as empty

Both accepted only a bare array and returned ok:true with items:[] for
anything else, so an unrecognised body was indistinguishable from an empty
library. Both now read the ItemPage envelope, keep the bare-array path for
older instances, and report ok:false for a shape they do not recognise."
```

---

### Task 6: Web — paged-state reducer

Pure logic first, so the component in Task 7 stays thin and the reducer is testable under web's node-only vitest.

**Files:**
- Create: `apps/web/lib/pages.ts`
- Test: `apps/web/lib/pages.test.ts`

**Interfaces:**
- Consumes: `Item`, `ItemPage` from `apps/web/lib/types.ts` (Task 2).
- Produces:
  - `interface PagedState<T> { pages: T[][]; cursor?: string }`
  - `function initialPagedState<T>(items: T[], cursor?: string): PagedState<T>`
  - `function appendPage<T>(state: PagedState<T>, page: { items: T[]; nextCursor?: string }): PagedState<T>`
  - `function mapPagedItems<T>(state: PagedState<T>, fn: (item: T) => T): PagedState<T>`

- [ ] **Step 1: Write the failing test**

Create `apps/web/lib/pages.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { appendPage, initialPagedState, mapPagedItems, type PagedState } from "./pages";

type Row = { id: string; kept?: boolean };

describe("paged state", () => {
  it("starts as a single page carrying the first cursor", () => {
    const s = initialPagedState<Row>([{ id: "a" }, { id: "b" }], "cur1");
    expect(s.pages).toEqual([[{ id: "a" }, { id: "b" }]]);
    expect(s.cursor).toBe("cur1");
  });

  it("keeps each page as its own array so the Mind can render one block per page", () => {
    let s = initialPagedState<Row>([{ id: "a" }], "cur1");
    s = appendPage(s, { items: [{ id: "b" }], nextCursor: "cur2" });
    expect(s.pages).toHaveLength(2);
    expect(s.pages[0]).toEqual([{ id: "a" }]);
    expect(s.pages[1]).toEqual([{ id: "b" }]);
    expect(s.cursor).toBe("cur2");
  });

  it("clears the cursor when a page arrives without one", () => {
    let s = initialPagedState<Row>([{ id: "a" }], "cur1");
    s = appendPage(s, { items: [{ id: "b" }] });
    expect(s.cursor).toBeUndefined();
  });

  it("drops an empty final page rather than rendering an empty block", () => {
    const s = appendPage(initialPagedState<Row>([{ id: "a" }], "cur1"), { items: [] });
    expect(s.pages).toHaveLength(1);
    expect(s.cursor).toBeUndefined();
  });

  it("maps items across every page", () => {
    let s = initialPagedState<Row>([{ id: "a" }], "cur1");
    s = appendPage(s, { items: [{ id: "b" }] });
    const mapped = mapPagedItems(s, (r) => (r.id === "b" ? { ...r, kept: true } : r));
    expect(mapped.pages[1][0].kept).toBe(true);
    expect(mapped.pages[0][0].kept).toBeUndefined();
  });

  it("does not mutate the state it maps", () => {
    const s: PagedState<Row> = initialPagedState<Row>([{ id: "a" }], undefined);
    mapPagedItems(s, (r) => ({ ...r, kept: true }));
    expect(s.pages[0][0].kept).toBeUndefined();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/web && pnpm exec vitest run lib/pages.test.ts`

Expected: FAIL — cannot resolve `./pages`.

- [ ] **Step 3: Write the implementation**

Create `apps/web/lib/pages.ts`:

```ts
/**
 * Cursor-paged list state. Pages are kept separate rather than flattened
 * because the Mind renders one CSS multi-column block per page: appending into
 * a single .mind-col makes the browser rebalance every column, which moves
 * cards the reader has already passed.
 */
export interface PagedState<T> {
  pages: T[][];
  /** Cursor for the next page; undefined once the list is exhausted. */
  cursor?: string;
}

export function initialPagedState<T>(items: T[], cursor?: string): PagedState<T> {
  return { pages: [items], cursor };
}

export function appendPage<T>(
  state: PagedState<T>,
  page: { items: T[]; nextCursor?: string },
): PagedState<T> {
  // An empty page would render as an empty block; the API's lookahead makes
  // this rare, but a concurrent delete can still produce one.
  const pages = page.items.length > 0 ? [...state.pages, page.items] : state.pages;
  return { pages, cursor: page.nextCursor };
}

export function mapPagedItems<T>(state: PagedState<T>, fn: (item: T) => T): PagedState<T> {
  return { ...state, pages: state.pages.map((page) => page.map(fn)) };
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/web && pnpm exec vitest run lib/pages.test.ts`

Expected: PASS (7 tests).

- [ ] **Step 5: Commit**

```bash
git add apps/web/lib/pages.ts apps/web/lib/pages.test.ts
git commit -m "feat(web): paged list state reducer

Pages stay separate rather than flattened so the Mind can render one
.mind-col block per page and never reflow already-rendered cards."
```

---

### Task 7: Web — the Mind river

**Files:**
- Create: `apps/web/components/ItemRiver.tsx`
- Create: `apps/web/components/LoadMore.tsx`
- Modify: `apps/web/app/page.tsx:12-21` (`getRecents`) and `:81-161` (the page body)
- Modify: `apps/web/components/Topbar.tsx:13-14, 57`

**Interfaces:**
- Consumes: `initialPagedState`, `appendPage` (Task 6); `ItemPage` (Task 2); the existing `Grid` component unchanged.
- Produces:
  - `function ItemRiver({ initialItems, initialCursor, colorActive }: { initialItems: Item[]; initialCursor?: string; colorActive?: boolean })`
  - `function LoadMore({ onLoad, loading, error, label }: { onLoad: () => void; loading: boolean; error: boolean; label: string })`
  - `Topbar` gains an optional `hasMore?: boolean` prop.

- [ ] **Step 1: Build the reusable load control**

Create `apps/web/components/LoadMore.tsx`:

```tsx
"use client";

import { tokens } from "@openmind/ui";
import { useEffect, useRef } from "react";

const { color, font } = tokens;

/**
 * The load-more affordance shared by the Mind and the Feed river.
 *
 * The button is the control and is always rendered; the IntersectionObserver
 * merely presses it early. Infinite scroll whose only trigger is a scroll event
 * is unreachable by keyboard and invisible to a screen reader.
 */
export function LoadMore({
  onLoad,
  loading,
  error,
  label,
}: {
  onLoad: () => void;
  loading: boolean;
  error: boolean;
  label: string;
}) {
  const sentinel = useRef<HTMLDivElement | null>(null);
  const onLoadRef = useRef(onLoad);
  onLoadRef.current = onLoad;

  useEffect(() => {
    const node = sentinel.current;
    // Never auto-load into a failure: after an error the reader presses Retry.
    if (!node || loading || error) return;
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) onLoadRef.current();
      },
      { rootMargin: "600px 0px" },
    );
    io.observe(node);
    return () => io.disconnect();
  }, [loading, error]);

  return (
    <div style={{ display: "flex", justifyContent: "center", padding: "28px 0 8px" }}>
      <div ref={sentinel} aria-hidden style={{ position: "absolute", height: 1, width: 1 }} />
      <button
        type="button"
        onClick={onLoad}
        disabled={loading}
        style={{
          font: `500 11px/1 ${font.mono}`,
          letterSpacing: ".04em",
          color: error ? color.danger : color.inkFaint,
          background: "none",
          border: `1px solid ${error ? color.danger : color.hairline}`,
          borderRadius: 20,
          padding: "10px 18px",
          cursor: loading ? "default" : "pointer",
        }}
      >
        {error ? "Couldn't load more — retry" : loading ? "Loading…" : label}
      </button>
    </div>
  );
}
```

- [ ] **Step 2: Build the Mind river**

Create `apps/web/components/ItemRiver.tsx`:

```tsx
"use client";

import { useCallback, useState } from "react";
import { Grid } from "./Grid";
import { LoadMore } from "./LoadMore";
import { appendPage, initialPagedState } from "../lib/pages";
import type { Item, ItemPage } from "../lib/types";

/**
 * The Mind's paged river. Page one arrives from the server render, so first
 * paint is unchanged; later pages are fetched client-side and appended.
 *
 * Each page renders as its own <Grid>, i.e. its own .mind-col block. Appending
 * into one shared block would make the browser rebalance all columns, moving
 * cards the reader has already passed (measured 8 of 12 on a 12-card page).
 */
export function ItemRiver({
  initialItems,
  initialCursor,
  colorActive,
}: {
  initialItems: Item[];
  initialCursor?: string;
  colorActive?: boolean;
}) {
  const [state, setState] = useState(() => initialPagedState(initialItems, initialCursor));
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const [announcement, setAnnouncement] = useState("");

  const loadMore = useCallback(async () => {
    if (loading || !state.cursor) return;
    setLoading(true);
    setFailed(false);
    try {
      const res = await fetch(`/api/items?cursor=${encodeURIComponent(state.cursor)}`);
      if (!res.ok) throw new Error(`failed to load more items: ${res.status}`);
      const page = (await res.json()) as ItemPage;
      setState((prev) => appendPage(prev, page));
      setAnnouncement(`${page.items.length} more saves loaded`);
    } catch (err) {
      console.error("failed to load more items", err);
      setFailed(true);
    } finally {
      setLoading(false);
    }
  }, [loading, state.cursor]);

  return (
    <>
      {state.pages.map((page, i) => (
        // Index keys are safe here: pages are only ever appended, never
        // reordered or spliced.
        <Grid key={i} items={page} colorActive={colorActive} />
      ))}
      <p aria-live="polite" style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)" }}>
        {announcement}
      </p>
      {state.cursor ? (
        <LoadMore onLoad={loadMore} loading={loading} error={failed} label="Load more saves" />
      ) : null}
    </>
  );
}
```

- [ ] **Step 3: Return the cursor from the server fetch**

In `apps/web/app/page.tsx`, replace `getRecents` (lines 12-21):

```tsx
async function getRecents(): Promise<ItemPage> {
  try {
    const res = await apiFetch("/items");
    if (!res.ok) return { items: [] };
    return ((await res.json()) as ItemPage) ?? { items: [] };
  } catch {
    // API/enrichment may be down; render an empty state rather than failing.
    return { items: [] };
  }
}
```

Add `ItemPage` to the `../lib/types` import on line 3.

- [ ] **Step 4: Wire the page body**

In the same file, the page currently does `{ items: await getRecents(), understood: undefined }`. Rework the data section so the unsearched branch keeps its cursor:

```tsx
  const recents = searching ? null : await getRecents();
  const { items, understood } = searching
    ? await getSearch({ q, color, type, domains })
    : { items: recents!.items, understood: undefined };
```

Then replace the render of library items (lines 136-150). When searching, keep today's `<Grid>`; otherwise use the river:

```tsx
          {searching ? (
            libraryItems.length > 0 || feedItems.length === 0 ? (
              <Grid items={libraryItems} colorActive={Boolean(color)} />
            ) : (
              <p
                style={{
                  fontFamily: tokens.font.quote,
                  fontStyle: "italic",
                  fontSize: "1.25rem",
                  color: tokens.color.inkMuted,
                  marginTop: "2rem",
                }}
              >
                Nothing in your Mind matches — these came through your feeds.
              </p>
            )
          ) : (
            <ItemRiver
              initialItems={libraryItems}
              initialCursor={recents?.nextCursor}
              colorActive={Boolean(color)}
            />
          )}
```

Import `ItemRiver` from `../components/ItemRiver`. Leave the `feedItems` divider block below it untouched — it only ever renders while searching.

- [ ] **Step 5: Stop the masthead asserting a total it does not know**

In `apps/web/components/Topbar.tsx`, change the signature (line 13) and the subline (line 57):

```tsx
export function Topbar({ count, q, hasMore }: { count: number; q?: string; hasMore?: boolean }) {
  const noun = count === 1 ? "gathering" : "gatherings";
```

```tsx
        {count.toLocaleString("en-GB")}
        {hasMore ? "+" : ""} {noun} · organised by the machine
```

Then in `page.tsx`, pass it:

```tsx
      <Topbar count={items.length} q={q} hasMore={Boolean(recents?.nextCursor)} />
```

`count` is page one's length, so this reads "50+ gatherings" while more remain. It deliberately does not track client-side appends: a number that ticks upward as you scroll asserts a library size that is not one. A true total needs `total` on `ItemPage`, logged as a follow-up.

- [ ] **Step 6: Typecheck and run the web tests**

Run:
```bash
cd apps/web && pnpm exec tsc --noEmit && pnpm exec vitest run
```

Expected: clean, all tests pass.

- [ ] **Step 7: Verify in the browser**

**Ask the maintainer before starting a dev server — they usually have one running.** With the API and web running, and more than 50 items in the library:

1. Load `/`. The masthead reads `50+ gatherings`.
2. Scroll to the bottom. A second block of cards appends and the cards above it **do not move**.
3. Tab to the "Load more saves" button and press Enter — it works without scrolling.
4. Search for something. No load-more control appears (search is not paginated).

- [ ] **Step 8: Commit**

```bash
git add apps/web/components/ItemRiver.tsx apps/web/components/LoadMore.tsx \
        apps/web/app/page.tsx apps/web/components/Topbar.tsx
git commit -m "feat(web): infinite scroll on the Mind

Page one still server-renders; later pages append as their own .mind-col block
so already-rendered cards never reflow. The button is the real control and the
observer only presses it early. The masthead now says '50+' rather than
asserting a total it cannot know."
```

---

### Task 8: Web — the Feed river

**Files:**
- Modify: `apps/web/components/FeedRiver.tsx:140-338`

**Interfaces:**
- Consumes: `initialPagedState`, `appendPage`, `mapPagedItems` (Task 6); `LoadMore` (Task 7); `ItemPage` (Task 2).
- Produces: no new exports; `FeedRiver`'s props are unchanged.

- [ ] **Step 1: Switch the fetch to paged state**

In `apps/web/components/FeedRiver.tsx`, replace the `items` state and its effect (lines 141-168):

```tsx
  const [state, setState] = useState<PagedState<Item> | null>(null);
  const [activeFeedId, setActiveFeedId] = useState<string | undefined>(undefined);
  const [showAllChips, setShowAllChips] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreFailed, setMoreFailed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoadFailed(false);
    // A filter change is a different list, so page state resets rather than
    // appending this feed's rows underneath another feed's.
    setState(null);
    const params = new URLSearchParams();
    if (activeFeedId) params.set("feedId", activeFeedId);
    const qs = params.toString();
    fetch(`/api/feed${qs ? `?${qs}` : ""}`)
      .then(async (res) => {
        if (!res.ok) throw new Error(`failed to load feed: ${res.status}`);
        return (await res.json()) as ItemPage;
      })
      .then((page) => {
        if (!cancelled) setState(initialPagedState(page.items, page.nextCursor));
      })
      .catch((err) => {
        console.error("failed to load feed river", err);
        if (!cancelled) setLoadFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [activeFeedId, loadAttempt]);
```

Add to the imports:

```tsx
import { appendPage, initialPagedState, mapPagedItems, type PagedState } from "../lib/pages";
import { LoadMore } from "./LoadMore";
import type { Feed, Item, ItemPage } from "../lib/types";
```

- [ ] **Step 2: Add the load-more handler**

Below the effect, add:

```tsx
  const loadMore = useCallback(async () => {
    const cursor = state?.cursor;
    if (loadingMore || !cursor) return;
    setLoadingMore(true);
    setMoreFailed(false);
    try {
      const params = new URLSearchParams({ cursor });
      if (activeFeedId) params.set("feedId", activeFeedId);
      const res = await fetch(`/api/feed?${params.toString()}`);
      if (!res.ok) throw new Error(`failed to load more feed items: ${res.status}`);
      const page = (await res.json()) as ItemPage;
      setState((prev) => (prev ? appendPage(prev, page) : initialPagedState(page.items, page.nextCursor)));
    } catch (err) {
      console.error("failed to load more feed items", err);
      setMoreFailed(true);
    } finally {
      setLoadingMore(false);
    }
  }, [activeFeedId, loadingMore, state?.cursor]);
```

- [ ] **Step 3: Make the keep toggle walk pages**

Replace `setKept` (lines 178-184):

```tsx
  const setKept = useCallback((itemId: string, kept: boolean) => {
    setState((prev) =>
      prev
        ? mapPagedItems(prev, (it) =>
            it.id === itemId ? { ...it, keptAt: kept ? new Date().toISOString() : null } : it,
          )
        : prev,
    );
  }, []);
```

- [ ] **Step 4: Render the pages**

The river is a single `<ul>`, not a masonry, so pages flatten with no layout consequence. Replace the `body` expression's conditions and its list branch (lines 217-271), keeping every existing empty/failure state intact but reading from `state`:

- `items === null` becomes `state === null`
- `items.length === 0 && activeFeedId` becomes `state.pages[0].length === 0 && activeFeedId`
- `items.length === 0` becomes `state.pages[0].length === 0`
- the final list branch becomes:

```tsx
  ) : (
    <>
      <ul className="feed-river" style={{ listStyle: "none", margin: 0, maxWidth: 780 }}>
        {state.pages.flat().map((item) => (
          <Row
            key={item.id}
            item={item}
            feedTitle={titleFor(item.feedId)}
            onKeptChange={(kept) => setKept(item.id, kept)}
          />
        ))}
      </ul>
      {state.cursor ? (
        <LoadMore onLoad={loadMore} loading={loadingMore} error={moreFailed} label="Load more" />
      ) : null}
    </>
  );
```

- [ ] **Step 5: Typecheck and test**

Run: `cd apps/web && pnpm exec tsc --noEmit && pnpm exec vitest run`

Expected: clean, all tests pass.

- [ ] **Step 6: Verify in the browser**

With more than 50 feed items: load `/feed`, scroll to append a second page, then click a feed chip and confirm the river resets to that feed's first page rather than appending to the previous list. Toggle Keep on an item from page 2 and confirm the label changes.

- [ ] **Step 7: Commit**

```bash
git add apps/web/components/FeedRiver.tsx
git commit -m "feat(web): infinite scroll on the Feed river

Pages reset on a feed-filter change, and the keep toggle now maps across
pages rather than a single flat array."
```

---

### Task 9: Mobile — API client returns pages

**Files:**
- Modify: `apps/mobile/lib/api.ts:195-227` (`listItems`), `:229-267` (`listFeedItems`)
- Test: `apps/mobile/lib/__tests__/api-pages.test.ts`

**Interfaces:**
- Consumes: the `ItemPage` shape from Task 2 (structurally; mobile's client is hand-written by design).
- Produces:
  - `type ItemPageResult = { ok: boolean; status: number; items: Item[]; nextCursor?: string }`
  - `function listItems(limit?: number, cursor?: string, override?: Settings): Promise<ItemPageResult>`
  - `function listFeedItems(limit?: number, cursor?: string, override?: Settings): Promise<ItemPageResult>`

- [ ] **Step 1: Write the failing test**

Create `apps/mobile/lib/__tests__/api-pages.test.ts`:

```ts
import { listItems, readItemPage } from "../api";

describe("readItemPage", () => {
  it("reads the ItemPage envelope including the cursor", () => {
    const got = readItemPage({ items: [{ id: "a" }], nextCursor: "cur1" });
    expect(got).toEqual({ items: [{ id: "a" }], nextCursor: "cur1" });
  });

  it("reads a bare array from an instance predating the envelope, with no cursor", () => {
    // Graceful degradation: no cursor means pagination simply stops after
    // page 1 instead of the screen breaking against an older self-host.
    const got = readItemPage([{ id: "a" }]);
    expect(got).toEqual({ items: [{ id: "a" }], nextCursor: undefined });
  });

  it("returns null for an unrecognised body so callers can report a failure", () => {
    expect(readItemPage({ unexpected: true })).toBeNull();
    expect(readItemPage(null)).toBeNull();
    expect(readItemPage("nope")).toBeNull();
  });

  it("treats a missing nextCursor as the end of the list", () => {
    expect(readItemPage({ items: [] })).toEqual({ items: [], nextCursor: undefined });
  });
});

describe("listItems", () => {
  const settings = { instanceUrl: "https://openmind.test", token: "omk_test" } as const;

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("sends the cursor and returns it back out", async () => {
    const fetchMock = jest.spyOn(global, "fetch" as never).mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ items: [{ id: "a" }], nextCursor: "cur2" }),
    } as never);

    const res = await listItems(50, "cur1", settings as never);

    expect(res.ok).toBe(true);
    expect(res.nextCursor).toBe("cur2");
    const url = String((fetchMock.mock.calls[0] as unknown[])[0]);
    expect(url).toContain("limit=50");
    expect(url).toContain("cursor=cur1");
  });

  it("reports failure rather than an empty list when the body is unrecognised", async () => {
    jest.spyOn(global, "fetch" as never).mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ unexpected: true }),
    } as never);

    const res = await listItems(50, undefined, settings as never);
    expect(res.ok).toBe(false);
    expect(res.items).toEqual([]);
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/mobile && pnpm exec jest lib/__tests__/api-pages.test.ts`

Expected: FAIL — `readItemPage` is not exported and `listItems` takes no cursor.

- [ ] **Step 3: Add the page reader and thread the cursor**

In `apps/mobile/lib/api.ts`, add above `listItems`:

```ts
/** One page of items, plus the cursor for the next page (absent at the end). */
export type ItemPageResult = { ok: boolean; status: number; items: Item[]; nextCursor?: string };

/**
 * Read a list body in either shape: the ItemPage envelope, or the bare array
 * served by an instance predating it. An older self-host yields no cursor, so
 * pagination simply stops after page one instead of the screen breaking.
 *
 * Returns null when the body is neither, so callers report a failure rather
 * than an empty library — the two must not look the same.
 */
export function readItemPage(data: unknown): { items: Item[]; nextCursor?: string } | null {
  if (Array.isArray(data)) return { items: data as Item[], nextCursor: undefined };
  if (data && typeof data === "object") {
    const obj = data as { items?: unknown; nextCursor?: unknown };
    if (Array.isArray(obj.items)) {
      return {
        items: obj.items as Item[],
        nextCursor: typeof obj.nextCursor === "string" ? obj.nextCursor : undefined,
      };
    }
  }
  return null;
}
```

Then replace `listItems` and `listFeedItems` with:

```ts
/** List items via GET {instanceUrl}/api/items?limit=&cursor=. */
export async function listItems(
  limit = 50,
  cursor?: string,
  override?: Settings,
): Promise<ItemPageResult> {
  return fetchItemPage("/api/items", { limit, cursor }, override);
}

/** Feed-originated items via GET {instanceUrl}/api/feed?limit=&cursor=, newest first. */
export async function listFeedItems(
  limit = 50,
  cursor?: string,
  override?: Settings,
): Promise<ItemPageResult> {
  return fetchItemPage("/api/feed", { limit, cursor }, override);
}

async function fetchItemPage(
  path: string,
  query: { limit: number; cursor?: string },
  override?: Settings,
): Promise<ItemPageResult> {
  const settings = await resolveSettings(override);
  if (!settings) return { ok: false, status: 0, items: [] };
  const params = new URLSearchParams({ limit: String(query.limit) });
  if (query.cursor) params.set("cursor", query.cursor);
  try {
    const res = await fetch(`${settings.instanceUrl}${path}?${params.toString()}`, {
      method: "GET",
      headers: authHeaders(settings.token),
    });
    if (!res.ok) return { ok: false, status: res.status, items: [] };
    try {
      const page = readItemPage(await res.json());
      if (!page) {
        // A 200 we cannot read is a real failure, not an empty list.
        console.error("unrecognised item list body", { path });
        return { ok: false, status: res.status, items: [] };
      }
      return { ok: true, status: res.status, items: page.items, nextCursor: page.nextCursor };
    } catch (err) {
      console.error(err);
      return { ok: false, status: res.status, items: [] };
    }
  } catch (err) {
    console.error(err);
    return { ok: false, status: 0, items: [] };
  }
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/mobile && pnpm exec jest lib/__tests__/api-pages.test.ts`

Expected: PASS (6 tests).

- [ ] **Step 5: Typecheck**

Run: `cd apps/mobile && pnpm exec tsc --noEmit`

Expected: FAIL at the call sites in `app/(tabs)/index.tsx` and `app/(tabs)/feed.tsx`, which still call `listItems(LIST_LIMIT)` expecting `{ items }`. Those are Tasks 11 and 12; the signature change is source-compatible for `listItems(LIST_LIMIT)` (cursor is optional), so verify the errors are only about the removed `.items`-only assumptions. If `tsc` is clean, better still.

- [ ] **Step 6: Commit**

```bash
git add apps/mobile/lib/api.ts apps/mobile/lib/__tests__/api-pages.test.ts
git commit -m "feat(mobile): item list helpers return a page and a cursor

Keeps the bare-array path so an instance predating the envelope still works,
with no cursor meaning pagination stops after page one. An unreadable 200 is
now a failure rather than an empty list."
```

---

### Task 10: Mobile — paged cache helpers and mutation fan-out

Must land before Tasks 11 and 12. `patchItemInCaches` hard-codes flat cache shapes; once a cache becomes `{ pages, pageParams }` it would throw on every pin, keep, and delete.

**Files:**
- Create: `apps/mobile/lib/paged-cache.ts`
- Test: `apps/mobile/lib/__tests__/paged-cache.test.ts`
- Modify: `apps/mobile/lib/mutations.ts:16-37` (`patchItemInCaches`), `:39-48` (`useInvalidateLists`)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type InfiniteCache<T> = { pages: { items: T[]; nextCursor?: string }[]; pageParams: unknown[] }`
  - `function isInfiniteCache(v: unknown): v is InfiniteCache<unknown>`
  - `function mapCachedItems<T>(cache: unknown, fn: (item: T) => T): unknown` — handles infinite caches, flat arrays, and `{ items }` objects, returning the input untouched for anything else.
  - `function trimToFirstPage(cache: unknown): unknown`

- [ ] **Step 1: Write the failing test**

Create `apps/mobile/lib/__tests__/paged-cache.test.ts`:

```ts
import { isInfiniteCache, mapCachedItems, trimToFirstPage } from "../paged-cache";

type Row = { id: string; pinnedAt?: string | null };

const pin = (r: Row): Row => (r.id === "b" ? { ...r, pinnedAt: "2026-08-04T00:00:00Z" } : r);

describe("isInfiniteCache", () => {
  it("recognises a TanStack infinite cache", () => {
    expect(isInfiniteCache({ pages: [], pageParams: [] })).toBe(true);
  });

  it("rejects the flat shapes", () => {
    expect(isInfiniteCache([{ id: "a" }])).toBe(false);
    expect(isInfiniteCache({ items: [{ id: "a" }] })).toBe(false);
    expect(isInfiniteCache(undefined)).toBe(false);
  });
});

describe("mapCachedItems", () => {
  it("patches an item that lives on a later page", () => {
    const cache = {
      pages: [{ items: [{ id: "a" }] }, { items: [{ id: "z" }, { id: "b" }] }],
      pageParams: [undefined, "cur1"],
    };
    const got = mapCachedItems<Row>(cache, pin) as typeof cache;
    expect(got.pages[1].items[1].pinnedAt).toBe("2026-08-04T00:00:00Z");
    expect(got.pages[0].items[0].pinnedAt).toBeUndefined();
    expect(got.pages[1].nextCursor).toBeUndefined();
    expect(got.pageParams).toEqual([undefined, "cur1"]);
  });

  it("does not mutate the cache it patches", () => {
    const cache = { pages: [{ items: [{ id: "b" }] }], pageParams: [undefined] };
    mapCachedItems<Row>(cache, pin);
    expect(cache.pages[0].items[0].pinnedAt).toBeUndefined();
  });

  it("still patches a flat array cache (desk)", () => {
    const got = mapCachedItems<Row>([{ id: "a" }, { id: "b" }], pin) as Row[];
    expect(got[1].pinnedAt).toBe("2026-08-04T00:00:00Z");
  });

  it("still patches an { items } cache (search)", () => {
    const got = mapCachedItems<Row>({ items: [{ id: "b" }], understood: { text: "x" } }, pin) as {
      items: Row[];
      understood: { text: string };
    };
    expect(got.items[0].pinnedAt).toBe("2026-08-04T00:00:00Z");
    expect(got.understood).toEqual({ text: "x" });
  });

  it("leaves an unset cache alone", () => {
    expect(mapCachedItems<Row>(undefined, pin)).toBeUndefined();
  });
});

describe("trimToFirstPage", () => {
  it("drops every page after the first so one refetch is one request", () => {
    const cache = {
      pages: [{ items: [{ id: "a" }], nextCursor: "cur1" }, { items: [{ id: "b" }] }],
      pageParams: [undefined, "cur1"],
    };
    const got = trimToFirstPage(cache) as typeof cache;
    expect(got.pages).toHaveLength(1);
    expect(got.pageParams).toEqual([undefined]);
  });

  it("leaves non-infinite caches untouched", () => {
    const flat = [{ id: "a" }];
    expect(trimToFirstPage(flat)).toBe(flat);
    expect(trimToFirstPage(undefined)).toBeUndefined();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/mobile && pnpm exec jest lib/__tests__/paged-cache.test.ts`

Expected: FAIL — cannot resolve `../paged-cache`.

- [ ] **Step 3: Write the helpers**

Create `apps/mobile/lib/paged-cache.ts`:

```ts
/**
 * Cache-shape helpers for query caches that may be flat or paged.
 *
 * Library and Feed are infinite queries ({ pages, pageParams }); Desk is a flat
 * array and search is { items, understood }. One mutation touches all four, so
 * the patch helper has to handle every shape rather than assume one.
 */

export type InfiniteCache<T> = {
  pages: { items: T[]; nextCursor?: string }[];
  pageParams: unknown[];
};

export function isInfiniteCache(v: unknown): v is InfiniteCache<unknown> {
  return (
    !!v &&
    typeof v === "object" &&
    Array.isArray((v as { pages?: unknown }).pages) &&
    Array.isArray((v as { pageParams?: unknown }).pageParams)
  );
}

/** Apply fn to every item in a cache, whatever shape it has. Never mutates. */
export function mapCachedItems<T>(cache: unknown, fn: (item: T) => T): unknown {
  if (!cache) return cache;
  if (isInfiniteCache(cache)) {
    const c = cache as InfiniteCache<T>;
    return { ...c, pages: c.pages.map((p) => ({ ...p, items: p.items.map(fn) })) };
  }
  if (Array.isArray(cache)) return (cache as T[]).map(fn);
  const obj = cache as { items?: unknown };
  if (Array.isArray(obj.items)) return { ...obj, items: (obj.items as T[]).map(fn) };
  return cache;
}

/**
 * Drop every page but the first.
 *
 * TanStack Query v5 removed refetchPage, so refetching an infinite query
 * re-requests every loaded page in sequence — ten loaded pages would mean ten
 * requests on one pull-to-refresh. Trimming first makes it one request, and
 * scrolling back down re-pages naturally. Data is never cleared, so there is no
 * spinner flash.
 */
export function trimToFirstPage(cache: unknown): unknown {
  if (!isInfiniteCache(cache)) return cache;
  return { ...cache, pages: cache.pages.slice(0, 1), pageParams: cache.pageParams.slice(0, 1) };
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/mobile && pnpm exec jest lib/__tests__/paged-cache.test.ts`

Expected: PASS (9 tests).

- [ ] **Step 5: Rewrite `patchItemInCaches` to be shape-agnostic**

In `apps/mobile/lib/mutations.ts`, replace lines 16-37:

```ts
  const apply = (it: Item): Item => (it.id === id ? { ...it, ...patch } : it);

  qc.setQueriesData({ queryKey: ["feed"] }, (prev) => mapCachedItems<Item>(prev, apply));
  qc.setQueriesData({ queryKey: ["items"] }, (prev) => mapCachedItems<Item>(prev, apply));
  qc.setQueriesData({ queryKey: ["search"] }, (prev) => mapCachedItems<Item>(prev, apply));
  qc.setQueriesData<Item[]>({ queryKey: queryKeys.desk() }, (prev) => {
    if (!prev) return prev;
    // Unpinning removes from desk immediately.
    if (patch.pinnedAt === null) return prev.filter((it) => it.id !== id);
    // Pinning: if the item isn't on desk yet, leave lists to invalidate
    // (we may not have the full item here). Just patch badge fields if present.
    return prev.map(apply);
  });
  qc.setQueryData(queryKeys.item(id), (prev: Item | undefined) =>
    prev ? { ...prev, ...patch } : prev,
  );
```

Note `apply` changes from taking `Item[] | undefined` to taking a single `Item`, which is what `mapCachedItems` wants. Add the import:

```ts
import { mapCachedItems, trimToFirstPage } from "./paged-cache";
```

- [ ] **Step 6: Stop invalidation fanning out across pages**

Replace `useInvalidateLists` (lines 39-48):

```ts
/** Invalidate list caches after a mutation so Library / Feed / Desk stay in sync. */
export function useInvalidateLists() {
  const qc = useQueryClient();
  return useCallback(() => {
    // Trim before invalidating: v5 refetches every loaded page of an active
    // infinite query, so one pin with ten pages loaded would fire ten requests.
    // The optimistic patch above already keeps the visible list correct.
    qc.setQueriesData({ queryKey: ["items"] }, (prev) => trimToFirstPage(prev));
    qc.setQueriesData({ queryKey: ["feed"] }, (prev) => trimToFirstPage(prev));
    void qc.invalidateQueries({ queryKey: ["items"] });
    void qc.invalidateQueries({ queryKey: ["search"] });
    void qc.invalidateQueries({ queryKey: ["feed"] });
    void qc.invalidateQueries({ queryKey: queryKeys.desk() });
  }, [qc]);
}
```

- [ ] **Step 7: Run the mobile suite and typecheck**

Run: `cd apps/mobile && pnpm exec jest && pnpm exec tsc --noEmit`

Expected: jest passes. `tsc` may still flag the two screens (Tasks 11, 12); confirm no errors originate in `lib/`.

- [ ] **Step 8: Commit**

```bash
git add apps/mobile/lib/paged-cache.ts apps/mobile/lib/__tests__/paged-cache.test.ts \
        apps/mobile/lib/mutations.ts
git commit -m "fix(mobile): make cache patching shape-agnostic before paging lands

patchItemInCaches assumed Item[] for the feed cache and { items } for the
library cache; once either becomes an infinite cache that would throw on every
pin, keep and delete. Invalidation now trims to the first page first, since v5
refetches every loaded page of an active infinite query."
```

---

### Task 11: Mobile — Library infinite scroll

**Files:**
- Modify: `apps/mobile/app/(tabs)/index.tsx:79-110` (the query), `:340-392` (the lists), `:344-347` (grouped headers)
- Modify: `apps/mobile/lib/use-soft-focus-refetch.ts`

**Interfaces:**
- Consumes: `listItems` (Task 9); `trimToFirstPage` (Task 10); `queryKeys.items` unchanged.
- Produces: `useSoftFocusRefetch` gains an optional third argument `onBeforeRefetch?: () => void`, called before the refetch so an infinite query can trim itself first.

- [ ] **Step 1: Let the focus-refetch hook trim first**

In `apps/mobile/lib/use-soft-focus-refetch.ts`, add a third parameter and call it before refetching:

```ts
export function useSoftFocusRefetch(
  query: Pick<UseQueryResult, "isStale" | "isPending" | "refetch" | "dataUpdatedAt">,
  extra?: () => void,
  onBeforeRefetch?: () => void,
): void {
  const extraRef = useRef(extra);
  extraRef.current = extra;
  const beforeRef = useRef(onBeforeRefetch);
  beforeRef.current = onBeforeRefetch;

  useFocusEffect(
    useCallback(() => {
      extraRef.current?.();
      if (query.isStale || query.dataUpdatedAt === 0) {
        // An infinite query refetches every loaded page, so callers trim to the
        // first page here to keep a focus refetch to one request.
        beforeRef.current?.();
        void query.refetch();
      }
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [query.isStale, query.dataUpdatedAt, query.refetch]),
  );
}
```

The `Pick<UseQueryResult, …>` signature already accepts a `UseInfiniteQueryResult` structurally, so no type change is needed there.

- [ ] **Step 2: Switch the Library query to `useInfiniteQuery`**

In `apps/mobile/app/(tabs)/index.tsx`, replace the `listQuery` block (lines 79-110):

```tsx
  const queryClient = useQueryClient();

  const listQuery = useInfiniteQuery({
    queryKey: searching
      ? queryKeys.search(debouncedQuery)
      : filteringByColor
        ? queryKeys.search(`color:${colorFilter}`)
        : queryKeys.items(LIST_LIMIT),
    enabled: !!settings && configured,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }): Promise<LibraryPage> => {
      // Search and the colour filter are not paginated (the API fuses and caps
      // at 50), so they return a single page with no cursor and this hook
      // simply never asks for a second one.
      if (searching) {
        const res = await searchItems({ q: debouncedQuery, parse: true });
        if (!res.ok) throw new ApiError(res.status);
        return { items: res.results.map((r) => r.item), understood: res.understood };
      }
      if (filteringByColor) {
        const res = await searchItems({ color: colorFilter });
        if (!res.ok) throw new ApiError(res.status);
        return { items: res.results.map((r) => r.item) };
      }
      const res = await listItems(LIST_LIMIT, pageParam);
      if (!res.ok) throw new ApiError(res.status);
      return { items: res.items, nextCursor: res.nextCursor };
    },
    getNextPageParam: (last) => last.nextCursor,
    // Keep prior results visible while a new search key loads.
    placeholderData: (prev) => prev,
  });

  const trimToFirst = useCallback(() => {
    queryClient.setQueryData(queryKeys.items(LIST_LIMIT), (prev: unknown) => trimToFirstPage(prev));
  }, [queryClient]);

  useSoftFocusRefetch(
    listQuery,
    () => {
      void flush();
    },
    trimToFirst,
  );
```

Update the type alias near line 56:

```tsx
type LibraryPage = { items: Item[]; understood?: UnderstoodQuery; nextCursor?: string };
```

Imports: add `useInfiniteQuery` and `useQueryClient` from `@tanstack/react-query`, and `trimToFirstPage` from `@/lib/paged-cache`.

- [ ] **Step 3: Derive the flattened list and the understood query**

Wherever the screen currently reads `listQuery.data?.items` and `listQuery.data?.understood`, replace with:

```tsx
  const items = listQuery.data?.pages.flatMap((p) => p.items) ?? [];
  const understood = listQuery.data?.pages[0]?.understood;
```

Read the file around lines 300-340 to find the existing derivations and replace them exactly; keep every downstream use (`groupByKind(items)`, the feed/mind split, empty states) as it is.

- [ ] **Step 4: Drop the count from grouped headers while pages remain**

Replace the grouped-sections block (lines 343-347):

```tsx
  if (grouped) {
    sections = groupByKind(items).map(({ kind, items: sectionItems }) => ({
      // A count over a partial list asserts a library size that is not one, so
      // while more pages remain the header carries the label alone.
      title: listQuery.hasNextPage
        ? typeLabelPlural[kind]
        : `${typeLabelPlural[kind]} · ${sectionItems.length}`,
      data: sectionItems,
    }));
  }
```

- [ ] **Step 5: Wire `onEndReached` and the footer onto both lists**

Add these above the `if (sections)` return:

```tsx
  const onEndReached = useCallback(() => {
    if (listQuery.hasNextPage && !listQuery.isFetchingNextPage) {
      void listQuery.fetchNextPage();
    }
  }, [listQuery]);

  const listFooter = listQuery.isFetchingNextPage ? (
    <View style={styles.footer}>
      <ActivityIndicator color={colors.inkFaint} />
    </View>
  ) : null;
```

Then add to **both** the `<SectionList>` and the `<FlatList>`:

```tsx
        onEndReached={onEndReached}
        onEndReachedThreshold={0.6}
        ListFooterComponent={listFooter}
```

And add the style to the `StyleSheet.create` block:

```tsx
  footer: { paddingVertical: spacing.xl, alignItems: "center" },
```

- [ ] **Step 6: Make pull-to-refresh trim first**

Find the `refreshControl` definition in this file and change its `onRefresh` to trim before refetching:

```tsx
  const refreshControl = (
    <RefreshControl
      refreshing={listQuery.isRefetching && !listQuery.isFetchingNextPage}
      onRefresh={() => {
        trimToFirst();
        void listQuery.refetch();
      }}
      tintColor={colors.inkFaint}
    />
  );
```

Preserve whatever props the existing `RefreshControl` already sets; only `refreshing` and `onRefresh` change.

- [ ] **Step 7: Typecheck and run the suite**

Run: `cd apps/mobile && pnpm exec tsc --noEmit && pnpm exec jest`

Expected: clean; jest passes.

- [ ] **Step 8: Verify on a simulator**

**Ask the maintainer before starting anything — a dev server is usually already running.** With more than 50 items:

1. Library scrolls past 50 and appends, with a spinner in the footer.
2. Pull to refresh: one request in the network log, no full-screen spinner.
3. Turn on grouping while more pages remain: headers show no counts. Scroll to the end and the counts return.
4. Long-press an item on page 2 and pin it: no crash, and the badge updates (this is the regression Task 10 prevents).
5. Type a search: results appear and do not attempt to paginate.

- [ ] **Step 9: Commit**

```bash
git add apps/mobile/app/\(tabs\)/index.tsx apps/mobile/lib/use-soft-focus-refetch.ts
git commit -m "feat(mobile): infinite scroll on the Library tab

Search and the colour filter return a single cursor-less page through the same
hook, so there is no second code path. Pull-to-refresh and focus refetch trim
to the first page so they stay one request. Grouped headers drop their counts
while more pages remain rather than asserting a partial total."
```

---

### Task 12: Mobile — Feed infinite scroll

**Files:**
- Modify: `apps/mobile/app/(tabs)/feed.tsx:54-64` (the query), `:188` (the `FlatList`)

**Interfaces:**
- Consumes: `listFeedItems` (Task 9); `trimToFirstPage` (Task 10).
- Produces: nothing new.

Mobile's Feed has no per-feed filter — that is web-only — so there is no filter-change reset here.

- [ ] **Step 1: Switch to `useInfiniteQuery`**

In `apps/mobile/app/(tabs)/feed.tsx`, replace the `feedQuery` block (lines 54-64):

```tsx
  const queryClient = useQueryClient();

  const feedQuery = useInfiniteQuery({
    queryKey: queryKeys.feed(LIST_LIMIT),
    enabled: !!settings && configured,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const res = await listFeedItems(LIST_LIMIT, pageParam);
      if (!res.ok) throw new ApiError(res.status);
      return { items: res.items, nextCursor: res.nextCursor };
    },
    getNextPageParam: (last) => last.nextCursor,
  });

  const trimToFirst = useCallback(() => {
    queryClient.setQueryData(queryKeys.feed(LIST_LIMIT), (prev: unknown) => trimToFirstPage(prev));
  }, [queryClient]);

  useSoftFocusRefetch(feedQuery, undefined, trimToFirst);

  const items = feedQuery.data?.pages.flatMap((p) => p.items) ?? [];

  const onEndReached = useCallback(() => {
    if (feedQuery.hasNextPage && !feedQuery.isFetchingNextPage) {
      void feedQuery.fetchNextPage();
    }
  }, [feedQuery]);
```

Imports: add `useInfiniteQuery` and `useQueryClient` from `@tanstack/react-query`, `useCallback` from `react` if absent, `ActivityIndicator` from `react-native` if absent, and `trimToFirstPage` from `@/lib/paged-cache`.

- [ ] **Step 2: Repoint the screen's data reads**

The screen previously read `feedQuery.data` as `Item[]`. Read lines 100-200 and replace each `feedQuery.data` use with the `items` derived above. Keep the loading, error, and empty states exactly as they are.

- [ ] **Step 3: Wire the list**

Add to the `<FlatList>` at line 188:

```tsx
        onEndReached={onEndReached}
        onEndReachedThreshold={0.6}
        ListFooterComponent={
          feedQuery.isFetchingNextPage ? (
            <View style={styles.footer}>
              <ActivityIndicator color={colors.inkFaint} />
            </View>
          ) : null
        }
```

Add to the `StyleSheet.create` block:

```tsx
  footer: { paddingVertical: spacing.xl, alignItems: "center" },
```

If the screen has a `RefreshControl`, change its `onRefresh` to call `trimToFirst()` before `feedQuery.refetch()`, as in Task 11 Step 6.

- [ ] **Step 4: Typecheck and run the suite**

Run: `cd apps/mobile && pnpm exec tsc --noEmit && pnpm exec jest`

Expected: clean; jest passes.

- [ ] **Step 5: Verify on a simulator**

With more than 50 feed items: the Feed tab appends past 50 with a footer spinner, pull-to-refresh issues one request, and toggling Keep on an item from page 2 updates without a crash.

- [ ] **Step 6: Commit**

```bash
git add apps/mobile/app/\(tabs\)/feed.tsx
git commit -m "feat(mobile): infinite scroll on the Feed tab"
```

---

### Task 13: Full verification and TODO.md

**Files:**
- Modify: `TODO.md`

**Interfaces:**
- Consumes: everything above.
- Produces: nothing.

- [ ] **Step 1: Run the whole suite**

Run:
```bash
task generate   # must be a no-op: proves no generated file drifted from the contract
task test
task lint
```

Expected: `task generate` reports no changes, `task test` passes (Go with `-p 1`, plus turbo JS), `task lint` clean.

- [ ] **Step 2: Confirm nothing else decodes the old shape**

Run:
```bash
grep -rn "api/items?limit\|/api/feed" apps/ packages/ --include=*.ts --include=*.tsx --include=*.go | grep -v node_modules | grep -v "\.output"
```

Check each hit either reads the envelope or is one of the two deliberately non-paginating clients from Task 5. Fix anything missed.

- [ ] **Step 3: Update TODO.md**

Add under `## Done (recent)`, matching the surrounding entry style (what changed, why, and the caveats):

```markdown
- Infinite scrolling on web and mobile — `/items` and `/feed` now return an
  `ItemPage` envelope (`{items, nextCursor}`) with keyset pagination on
  `(created_at DESC, id DESC)`; migration 0022 adds the matching index and drops
  `items_user_created_idx`, which was a strict prefix of it. Handlers over-fetch
  by one so `nextCursor` is precise and the client never ends on an empty
  request; an undecodable cursor is a 400, never a silent page 1. Web appends one
  `.mind-col` block per page — appending into the shared container rebalanced the
  columns and moved 8 of 12 already-visible cards, measured. The load control is
  a real button with an IntersectionObserver pressing it early, so it stays
  keyboard-reachable. Mobile moved to `useInfiniteQuery`; pull-to-refresh, focus
  refetch, and mutation invalidation all trim to the first page first, because
  TanStack v5 refetches every loaded page. Also fixed a pre-existing silent
  failure: the extension and dock turned an unrecognised list body into
  `ok: true` with an empty list. Search, Lens, Desk and Places deliberately
  unchanged (2026-08-04)
```

Then add under `## Later`:

```markdown
- Pagination follow-ups: search + Lens results are still capped at 50 fused RRF
  matches and would need a cursor over a materialised fused ranking; `/desk` has
  no limit or cursor at all; `ItemPage` carries no `total`, so the Mind's
  masthead says "50+ gatherings" and mobile's grouped headers drop their counts
  while more pages remain — a true total needs a `COUNT(*)` per request or a
  count endpoint. Revisit the Mind's masonry (JS-distributed fixed columns, no
  seam) only if the per-page seam proves annoying against a real 50-card page.
```

- [ ] **Step 4: Commit**

```bash
git add TODO.md
git commit -m "docs: TODO — infinite scroll on web and mobile"
```

---

## Self-Review

**Spec coverage.** Every section of `20260804-infinite-scroll-design.md` maps to a task: contract → 2; keyset/cursor/index → 1, 3, 4; masthead count → 7 Step 5; load control → 7 Steps 1-2; Mind → 6, 7; Feed river → 8; mobile client → 9; mutation caches → 10; refresh fan-out → 10, 11, 12; grouped counts → 11 Step 4; extension/dock → 5; tests → within each task; docs → 13.

**Deliberate deviations from the spec, both noted in-task:**
- The spec sketched a `total`-free masthead treatment; the plan pins it to `hasMore` on `Topbar` rather than tracking client appends, for the reason given in Task 7 Step 5.
- The spec did not name `useSoftFocusRefetch`'s signature change (Task 11 Step 1); it follows from the trim-then-refetch rule.

**Ordering constraint.** Task 10 must precede 11 and 12 — `mapCachedItems` handles both flat and paged shapes, so it is safe to land first while caches are still flat, and it prevents the throw that paging would otherwise introduce.

**Naming consistency, checked across tasks:** `pageCursor`/`encodeCursor`/`decodeCursor`/`errInvalidCursor` (1 → 4), `listLimit`/`cursorTimestamp`/`cursorUUID`/`toItemPage` (4), `LimitCount` after the sqlc rename (3 → 4, including `mcp.go` and `feedriver_test.go`), `PagedState`/`initialPagedState`/`appendPage`/`mapPagedItems`/`countPagedItems` (6 → 7, 8), `readItemPage`/`ItemPageResult` (9 → 11, 12), `isInfiniteCache`/`mapCachedItems`/`trimToFirstPage` (10 → 11, 12), `LoadMore` props (7 → 8).

**Fixed during review:** `countPagedItems` was specified, implemented, and tested but never consumed — Task 7 reads page one's length for the masthead. Removed rather than left as dead code. Task 3's test block also carried a `seedItem` stub with a "delete this" note, which an implementer reading out of order would have added; the tests now inline their seeding.

**Known soft spots**, flagged rather than hidden: Task 3 Step 2 depends on `Store`'s private pool field name, which the implementer must read first; Task 8 Steps 1 and 4, Task 11 Step 3, and Task 12 Step 2 patch by description against line ranges rather than quoting whole files, because those files run long — the implementer must read them before editing.
