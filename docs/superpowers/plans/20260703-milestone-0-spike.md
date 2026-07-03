# Milestone 0 Spike Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Real monorepo skeleton + one-binary pipeline spike (save → River → extract → Gemini summarise/tag → pgvector embed → hybrid search) + evidence-based extraction bake-off.

**Architecture:** Single Go binary (`cmd/openmind serve|work|all|migrate`) with chi HTTP layer generated from `openapi.yaml`, River jobs on Postgres, staged idempotent enrichment through a pluggable `ai.Provider` interface (gemini + noop), hybrid FTS+pgvector search with RRF. Spec: `docs/superpowers/specs/20260703-milestone-0-spike-design.md`.

**Tech Stack:** Go 1.24+, chi, River (riverqueue/river), sqlc + pgx/v5, pgvector-go, oapi-codegen v2, google.golang.org/genai, go-trafilatura, Postgres 17 + pgvector (docker), Taskfile, pnpm + turbo, Next.js.

## Global Constraints

- Capture is sacred: `POST /items` must never call AI or fetch the URL in the request path.
- Every table has `user_id`; every store query is user-scoped (`WHERE user_id = $1 AND ...`).
- All SQL through sqlc in `internal/store`; no inline SQL in handlers/jobs (search SQL lives in sqlc too).
- All AI calls through `ai.Provider`; `AI_PROVIDER=noop` must keep the app fully functional (FTS-only search).
- Never hand-edit generated code (`internal/api/gen.go`, sqlc output, `packages/api-client/src/schema.d.ts`).
- No new required infra beyond Postgres. No flagship AI models — `gemini-flash-lite-latest` and `gemini-embedding-001` only.
- Comments: never use banner/divider-style comment blocks (`// ====== Section ======`). Sparse, constraint-explaining comments only.
- If a library API in this plan doesn't compile, consult context7 for current docs (River: `/websites/riverqueue`, genai: `/googleapis/go-genai`) — do not guess alternates.
- Store tests require the compose Postgres: `docker compose up -d db` first; tests read `TEST_DATABASE_URL` (default `postgres://openmind:openmind@localhost:5433/openmind_test`).
- Commit after every task (steps say when). Repo already has git history — never force-push or amend earlier commits.
- Product vocabulary: Lens/Drift/Desk; card types: article, product, book, recipe, video, tweet, image, note, quote.

---

### Task 1: Repo scaffold & tooling

**Files:**
- Create: `.gitignore`, `go.work`, `docker-compose.yml`, `.env.example`, `Taskfile.yml`, `pnpm-workspace.yaml`, `turbo.json`, `package.json`
- Create: `apps/api/go.mod` (via `go mod init`), `apps/extension/README.md`, `apps/mobile/README.md`, `apps/dock/README.md`

**Interfaces:**
- Produces: compose service `db` (Postgres 17 + pgvector, port **5433**, user/pass/db `openmind`, plus `openmind_test` database); Task targets `dev`, `db`, `generate`, `test`, `lint`, `migrate`; Go module path `github.com/rohithgilla12/openmind/api`.

- [ ] **Step 1: Write root config files**

`.gitignore`:
```gitignore
.DS_Store
node_modules/
.env
.turbo/
.next/
dist/
apps/api/bin/
```

`docker-compose.yml` (pgvector image ships Postgres+extension; init script creates the test DB):
```yaml
services:
  db:
    image: pgvector/pgvector:pg17
    environment:
      POSTGRES_USER: openmind
      POSTGRES_PASSWORD: openmind
      POSTGRES_DB: openmind
    ports:
      - "5433:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./scripts/initdb:/docker-entrypoint-initdb.d
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U openmind"]
      interval: 2s
      timeout: 2s
      retries: 15

  api:
    build: apps/api
    command: ["all"]
    environment:
      DATABASE_URL: postgres://openmind:openmind@db:5432/openmind
      AI_PROVIDER: ${AI_PROVIDER:-noop}
      GEMINI_API_KEY: ${GEMINI_API_KEY:-}
    ports:
      - "8080:8080"
    depends_on:
      db:
        condition: service_healthy

volumes:
  pgdata:
```

`scripts/initdb/01-test-db.sh`:
```bash
#!/bin/bash
set -e
psql -v ON_ERROR_STOP=1 -U openmind -d postgres -c "CREATE DATABASE openmind_test OWNER openmind;"
```

`.env.example`:
```bash
DATABASE_URL=postgres://openmind:openmind@localhost:5433/openmind
TEST_DATABASE_URL=postgres://openmind:openmind@localhost:5433/openmind_test
AI_PROVIDER=noop            # noop | gemini
GEMINI_API_KEY=
PORT=8080
```

`Taskfile.yml`:
```yaml
version: "3"

tasks:
  db:
    cmds: [docker compose up -d db]

  dev:
    deps: [db]
    cmds:
      - task: generate
      - cmd: cd apps/api && go run ./cmd/openmind migrate && go run ./cmd/openmind all

  generate:
    deps: [generate:api, generate:sqlc, generate:ts]

  generate:api:
    dir: apps/api
    sources: ["../../openapi.yaml", "oapi.cfg.yaml"]
    generates: ["internal/api/gen.go"]
    cmds: [go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1 -config oapi.cfg.yaml ../../openapi.yaml]

  generate:sqlc:
    dir: apps/api
    sources: ["sqlc.yaml", "internal/store/queries/*.sql", "internal/store/migrations/*.sql"]
    generates: ["internal/store/db/*.go"]
    cmds: [go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate]

  generate:ts:
    sources: ["openapi.yaml"]
    generates: ["packages/api-client/src/schema.d.ts"]
    cmds: [pnpm --filter @openmind/api-client generate]

  test:
    cmds:
      - cmd: cd apps/api && go test ./...
      - cmd: pnpm turbo run test

  lint:
    cmds:
      - cmd: cd apps/api && go vet ./...
      - cmd: pnpm turbo run lint

  migrate:
    dir: apps/api
    cmds: [go run ./cmd/openmind migrate]
```

`pnpm-workspace.yaml`:
```yaml
packages:
  - "apps/web"
  - "packages/*"
```

`package.json`:
```json
{
  "name": "openmind",
  "private": true,
  "packageManager": "pnpm@10.12.1",
  "devDependencies": { "turbo": "^2.5.0" }
}
```

`turbo.json`:
```json
{
  "$schema": "https://turbo.build/schema.json",
  "tasks": {
    "build": { "dependsOn": ["^build"], "outputs": [".next/**", "dist/**"] },
    "lint": {},
    "test": {},
    "dev": { "cache": false, "persistent": true }
  }
}
```

- [ ] **Step 2: Init Go workspace and placeholder apps**

```bash
mkdir -p apps/api apps/extension apps/mobile apps/dock packages scripts/initdb
cd apps/api && go mod init github.com/rohithgilla12/openmind/api && cd ../..
printf 'package api\n' > apps/api/doc.go
go work init ./apps/api
for a in extension mobile dock; do echo "# openmind $a — placeholder (Milestone 1+)" > apps/$a/README.md; done
chmod +x scripts/initdb/01-test-db.sh
```

- [ ] **Step 3: Verify**

Run: `docker compose up -d db && docker compose ps` → db healthy.
Run: `psql postgres://openmind:openmind@localhost:5433/openmind_test -c 'select 1'` → returns 1.
Run: `cd apps/api && go build ./... ` → OK. `task --list` → shows tasks.

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "chore: monorepo scaffold (taskfile, compose, go workspace, pnpm/turbo)"
```

---

### Task 2: openapi.yaml v0 + Go codegen wiring

**Files:**
- Create: `openapi.yaml`, `apps/api/oapi.cfg.yaml`
- Generated: `apps/api/internal/api/gen.go`

**Interfaces:**
- Produces: Go types `api.Item`, `api.CreateItemRequest`, `api.SearchResult`; server interface `api.ServerInterface` with `CreateItem`, `ListItems`, `SearchItems`. Task 8 implements it; Task 10 generates the TS client from the same spec.

- [ ] **Step 1: Write `openapi.yaml`**

```yaml
openapi: 3.0.3
info: { title: Openmind API, version: 0.0.1 }
paths:
  /items:
    post:
      operationId: createItem
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/CreateItemRequest" }
      responses:
        "201":
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Item" }
    get:
      operationId: listItems
      parameters:
        - { name: limit, in: query, schema: { type: integer, default: 50 } }
      responses:
        "200":
          content:
            application/json:
              schema:
                type: array
                items: { $ref: "#/components/schemas/Item" }
  /search:
    get:
      operationId: searchItems
      parameters:
        - { name: q, in: query, required: true, schema: { type: string } }
      responses:
        "200":
          content:
            application/json:
              schema:
                type: array
                items: { $ref: "#/components/schemas/SearchResult" }
components:
  schemas:
    CreateItemRequest:
      type: object
      required: [url]
      properties:
        url: { type: string, format: uri }
    Item:
      type: object
      required: [id, url, status, createdAt]
      properties:
        id: { type: string, format: uuid }
        url: { type: string }
        title: { type: string }
        summary: { type: string }
        tags: { type: array, items: { type: string } }
        cardType: { type: string, enum: [article, product, book, recipe, video, tweet, image, note, quote] }
        status: { type: string, enum: [pending, enriched, failed] }
        createdAt: { type: string, format: date-time }
    SearchResult:
      type: object
      required: [item, score]
      properties:
        item: { $ref: "#/components/schemas/Item" }
        score: { type: number }
```

`apps/api/oapi.cfg.yaml`:
```yaml
package: api
output: internal/api/gen.go
generate:
  chi-server: true
  models: true
```

- [ ] **Step 2: Generate and verify**

Run: `mkdir -p apps/api/internal/api && task generate:api`
Then: `cd apps/api && go get github.com/go-chi/chi/v5 github.com/oapi-codegen/runtime && go build ./...` → OK. Confirm `internal/api/gen.go` contains `type ServerInterface interface` with the three operations.

- [ ] **Step 3: Commit**

```bash
git add openapi.yaml apps/api/oapi.cfg.yaml apps/api/internal/api/gen.go apps/api/go.mod apps/api/go.sum
git commit -m "feat(api): openapi v0 contract + oapi-codegen wiring"
```

---

### Task 3: Store layer — migrations, sqlc, migrate command

**Files:**
- Create: `apps/api/internal/store/migrations/0001_init.sql`, `apps/api/internal/store/migrate.go`, `apps/api/sqlc.yaml`, `apps/api/internal/store/queries/items.sql`, `apps/api/internal/store/store.go`, `apps/api/cmd/openmind/main.go`
- Test: `apps/api/internal/store/store_test.go`
- Generated: `apps/api/internal/store/db/*.go`

**Interfaces:**
- Produces: `store.New(pool *pgxpool.Pool) *Store`; `store.Migrate(ctx, pool)` (app schema + River schema); `Store.Queries` (*db.Queries) with `CreateItem(ctx, db.CreateItemParams) (db.Item, error)`, `GetItem(ctx, db.GetItemParams)`, `ListItems`, `UpdateItemExtraction`, `UpdateItemEnrichment`, `SetItemStatus`, `UpsertEmbedding`, `EnsureUser(ctx, uuid) `; `cmd/openmind` binary with `migrate` subcommand (other subcommands added in Tasks 7–8).
- Consumes: compose db from Task 1.

- [ ] **Step 1: Write migration `0001_init.sql`**

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    url text NOT NULL,
    title text NOT NULL DEFAULT '',
    body text NOT NULL DEFAULT '',
    lead_image_url text NOT NULL DEFAULT '',
    summary text NOT NULL DEFAULT '',
    tags text[] NOT NULL DEFAULT '{}',
    card_type text NOT NULL DEFAULT 'article',
    status text NOT NULL DEFAULT 'pending',
    search_tsv tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', title), 'A') ||
        setweight(to_tsvector('english', summary), 'B') ||
        setweight(to_tsvector('english', array_to_string(tags, ' ')), 'B') ||
        setweight(to_tsvector('english', left(body, 100000)), 'C')
    ) STORED,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX items_user_created_idx ON items (user_id, created_at DESC);
CREATE INDEX items_search_tsv_idx ON items USING gin (search_tsv);

CREATE TABLE item_embeddings (
    item_id uuid PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id),
    embedding vector(768) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX item_embeddings_user_idx ON item_embeddings (user_id);
```

- [ ] **Step 2: Write the migrator `internal/store/migrate.go`**

Tiny sequential migrator (stdlib-first, no new dep) + River schema via rivermigrate:

```go
package store

import (
	"context"
	"embed"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		var done bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, name).Scan(&done); err != nil {
			return fmt.Errorf("checking migration %s: %w", name, err)
		}
		if done {
			continue
		}
		sql, _ := migrationsFS.ReadFile("migrations/" + name)
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("beginning tx for %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("applying %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("recording %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("committing %s: %w", name, err)
		}
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("creating river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("migrating river schema: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: sqlc config + queries**

`apps/api/sqlc.yaml`:
```yaml
version: "2"
sql:
  - engine: postgresql
    schema: internal/store/migrations
    queries: internal/store/queries
    gen:
      go:
        package: db
        out: internal/store/db
        sql_package: pgx/v5
        overrides:
          - db_type: vector
            go_type: { import: "github.com/pgvector/pgvector-go", type: "Vector" }
```

`internal/store/queries/items.sql`:
```sql
-- name: EnsureUser :exec
INSERT INTO users (id) VALUES ($1) ON CONFLICT DO NOTHING;

-- name: CreateItem :one
INSERT INTO items (user_id, url) VALUES ($1, $2) RETURNING *;

-- name: GetItem :one
SELECT * FROM items WHERE user_id = $1 AND id = $2;

-- name: ListItems :many
SELECT * FROM items WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2;

-- name: UpdateItemExtraction :exec
UPDATE items SET title = $3, body = $4, lead_image_url = $5, card_type = $6, updated_at = now()
WHERE user_id = $1 AND id = $2;

-- name: UpdateItemEnrichment :exec
UPDATE items SET summary = $3, tags = $4, updated_at = now()
WHERE user_id = $1 AND id = $2;

-- name: SetItemStatus :exec
UPDATE items SET status = $3, updated_at = now() WHERE user_id = $1 AND id = $2;

-- name: UpsertEmbedding :exec
INSERT INTO item_embeddings (item_id, user_id, embedding) VALUES ($1, $2, $3)
ON CONFLICT (item_id) DO UPDATE SET embedding = EXCLUDED.embedding;
```

`internal/store/store.go`:
```go
package store

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

type Store struct {
	Pool    *pgxpool.Pool
	Queries *db.Queries
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool, Queries: db.New(pool)}
}
```

Run: `task generate:sqlc` then `cd apps/api && go get github.com/pgvector/pgvector-go github.com/jackc/pgx/v5 github.com/riverqueue/river github.com/riverqueue/river/riverdriver/riverpgxv5 && go build ./...` → OK.

- [ ] **Step 4: Write failing store test `internal/store/store_test.go`**

```go
package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://openmind:openmind@localhost:5433/openmind_test"
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	pool.Exec(ctx, `TRUNCATE items, item_embeddings CASCADE`)
	return store.New(pool)
}

func TestItemLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: "https://example.com"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.Status != "pending" {
		t.Errorf("status = %q, want pending", item.Status)
	}
	vec := make([]float32, 768)
	vec[0] = 1
	if err := s.Queries.UpsertEmbedding(ctx, db.UpsertEmbeddingParams{ItemID: item.ID, UserID: userID, Embedding: pgvector.NewVector(vec)}); err != nil {
		t.Fatalf("upsert embedding: %v", err)
	}
	otherUser := uuid.New()
	s.Queries.EnsureUser(ctx, otherUser)
	if _, err := s.Queries.GetItem(ctx, db.GetItemParams{UserID: otherUser, ID: item.ID}); err == nil {
		t.Error("cross-tenant read succeeded; want no rows")
	}
}
```

Note: sqlc may generate `UserID`/`ID` types as `pgtype.UUID` depending on config; if so, add to `sqlc.yaml` overrides: `- db_type: uuid` → `go_type: { import: "github.com/google/uuid", type: "UUID" }`, regenerate.

- [ ] **Step 5: Run test to verify it fails, then wire `cmd/openmind migrate`**

Run: `cd apps/api && go test ./internal/store/` → fails (compile or migrate errors) → fix until pass.

`cmd/openmind/main.go`:
```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rohithgilla12/openmind/api/internal/store"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		slog.Error("openmind failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: openmind <serve|work|all|migrate>")
	}
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	switch args[0] {
	case "migrate":
		return store.Migrate(ctx, pool)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
```

Run: `go test ./... && DATABASE_URL=postgres://openmind:openmind@localhost:5433/openmind go run ./cmd/openmind migrate` → PASS / exits 0.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat(store): schema, migrator, sqlc queries, migrate command"
```

---

### Task 4: AI adapter interface + noop provider

**Files:**
- Create: `apps/api/internal/ai/ai.go`, `apps/api/internal/ai/noop.go`
- Test: `apps/api/internal/ai/noop_test.go`

**Interfaces:**
- Produces:
```go
package ai

type Enrichment struct {
	Summary string
	Tags    []string
}

type Provider interface {
	Name() string
	Summarise(ctx context.Context, title, body string) (string, error)
	Tag(ctx context.Context, title, body string) ([]string, error)
	Embed(ctx context.Context, text string) ([]float32, error) // nil, ErrNotSupported from noop
	ParseQuery(ctx context.Context, q string) (string, error)  // passthrough for now
}

var ErrNotSupported = errors.New("ai: operation not supported by provider")

func FromEnv() Provider // reads AI_PROVIDER: "gemini" → NewGemini(os.Getenv("GEMINI_API_KEY")), default → NewNoop()
```

- [ ] **Step 1: Write failing test `noop_test.go`**

```go
package ai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/ai"
)

func TestNoopProvider(t *testing.T) {
	p := ai.NewNoop()
	ctx := context.Background()
	if s, err := p.Summarise(ctx, "T", "B"); err != nil || s != "" {
		t.Errorf("Summarise = (%q, %v), want empty, nil", s, err)
	}
	if tags, err := p.Tag(ctx, "T", "B"); err != nil || len(tags) != 0 {
		t.Errorf("Tag = (%v, %v), want empty, nil", tags, err)
	}
	if _, err := p.Embed(ctx, "x"); !errors.Is(err, ai.ErrNotSupported) {
		t.Errorf("Embed err = %v, want ErrNotSupported", err)
	}
	if q, err := p.ParseQuery(ctx, "red poster"); err != nil || q != "red poster" {
		t.Errorf("ParseQuery = (%q, %v), want passthrough", q, err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd apps/api && go test ./internal/ai/` → FAIL (package missing).

- [ ] **Step 3: Implement `ai.go` (interface, ErrNotSupported, FromEnv) and `noop.go`**

`noop.go`:
```go
package ai

import "context"

type Noop struct{}

func NewNoop() *Noop { return &Noop{} }

func (*Noop) Name() string                                             { return "noop" }
func (*Noop) Summarise(context.Context, string, string) (string, error) { return "", nil }
func (*Noop) Tag(context.Context, string, string) ([]string, error)     { return nil, nil }
func (*Noop) Embed(context.Context, string) ([]float32, error)          { return nil, ErrNotSupported }
func (*Noop) ParseQuery(_ context.Context, q string) (string, error)    { return q, nil }
```

`FromEnv` in `ai.go` (gemini branch added in Task 6; until then return noop with a `slog.Warn` for unknown values).

- [ ] **Step 4: Run test → PASS. Step 5: Commit**

```bash
git add apps/api/internal/ai && git commit -m "feat(ai): provider interface + noop provider"
```

---

### Task 5: Extractor interface + trafilatura implementation

**Files:**
- Create: `apps/api/internal/enrich/extract.go`, `apps/api/internal/enrich/trafilatura.go`
- Test: `apps/api/internal/enrich/trafilatura_test.go`, fixture `apps/api/internal/enrich/testdata/article.html`

**Interfaces:**
- Produces:
```go
package enrich

type Extraction struct {
	Title        string
	Body         string
	LeadImageURL string
}

type Extractor interface {
	Name() string
	Extract(ctx context.Context, url string) (Extraction, error)
}

func NewTrafilatura(client *http.Client) *Trafilatura
```

- [ ] **Step 1: Write fixture + failing test**

`testdata/article.html`: a real-ish article page (title in `<title>` and `<h1>`, `og:image` meta, several `<p>` paragraphs of body text, plus nav/footer boilerplate to prove noise removal). Write ~40 lines of HTML.

`trafilatura_test.go`:
```go
package enrich_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/enrich"
)

func TestTrafilaturaExtract(t *testing.T) {
	html, err := os.ReadFile("testdata/article.html")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(html)
	}))
	defer srv.Close()

	ex := enrich.NewTrafilatura(srv.Client())
	got, err := ex.Extract(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !strings.Contains(got.Title, "Commonplace Books") {
		t.Errorf("title = %q", got.Title)
	}
	if !strings.Contains(got.Body, "marginalia") {
		t.Errorf("body missing article text")
	}
	if strings.Contains(got.Body, "Subscribe to our newsletter") {
		t.Errorf("body contains boilerplate")
	}
}
```

(Fixture must contain "Commonplace Books" in title, "marginalia" in a body paragraph, "Subscribe to our newsletter" in footer.)

- [ ] **Step 2: Run → FAIL. Step 3: Implement**

`trafilatura.go`:
```go
package enrich

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/markusmobius/go-trafilatura"
)

type Trafilatura struct{ client *http.Client }

func NewTrafilatura(client *http.Client) *Trafilatura {
	if client == nil {
		client = http.DefaultClient
	}
	return &Trafilatura{client: client}
}

func (*Trafilatura) Name() string { return "trafilatura" }

func (t *Trafilatura) Extract(ctx context.Context, rawURL string) (Extraction, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Extraction{}, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", "openmind/0.1 (+https://github.com/rohithgilla12/open-mind)")
	resp, err := t.client.Do(req)
	if err != nil {
		return Extraction{}, fmt.Errorf("fetching %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Extraction{}, fmt.Errorf("fetching %s: status %d", rawURL, resp.StatusCode)
	}
	parsed, _ := url.Parse(rawURL)
	result, err := trafilatura.Extract(resp.Body, trafilatura.Options{OriginalURL: parsed, IncludeImages: true})
	if err != nil {
		return Extraction{}, fmt.Errorf("extracting %s: %w", rawURL, err)
	}
	return Extraction{
		Title:        result.Metadata.Title,
		Body:         result.ContentText,
		LeadImageURL: result.Metadata.Image,
	}, nil
}
```

Run: `go get github.com/markusmobius/go-trafilatura && go test ./internal/enrich/` → PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/api/internal/enrich && git commit -m "feat(enrich): extractor interface + trafilatura implementation"
```

---

### Task 6: Gemini provider

**Files:**
- Create: `apps/api/internal/ai/gemini.go`
- Modify: `apps/api/internal/ai/ai.go` (FromEnv gemini branch)
- Test: `apps/api/internal/ai/gemini_test.go` (skips without `GEMINI_API_KEY`)

**Interfaces:**
- Consumes: `ai.Provider` from Task 4.
- Produces: `ai.NewGemini(ctx, apiKey) (*Gemini, error)`; models `gemini-flash-lite-latest` (generate) and `gemini-embedding-001` at 768 dims (must match `vector(768)`).

- [ ] **Step 1: Write test (integration, skippable)**

```go
package ai_test

import (
	"context"
	"os"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/ai"
)

func TestGeminiProvider(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY not set")
	}
	ctx := context.Background()
	p, err := ai.NewGemini(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := p.Summarise(ctx, "Go", "Go is a statically typed compiled language designed at Google.")
	if err != nil || sum == "" {
		t.Errorf("Summarise = (%q, %v)", sum, err)
	}
	tags, err := p.Tag(ctx, "Go", "Go is a statically typed compiled language designed at Google.")
	if err != nil || len(tags) == 0 {
		t.Errorf("Tag = (%v, %v)", tags, err)
	}
	vec, err := p.Embed(ctx, "hello world")
	if err != nil || len(vec) != 768 {
		t.Errorf("Embed len = %d, err = %v; want 768", len(vec), err)
	}
}
```

- [ ] **Step 2: Implement `gemini.go`**

```go
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"
)

const (
	geminiGenModel   = "gemini-flash-lite-latest"
	geminiEmbedModel = "gemini-embedding-001"
	embedDims        = int32(768)
)

type Gemini struct{ client *genai.Client }

func NewGemini(ctx context.Context, apiKey string) (*Gemini, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, fmt.Errorf("creating genai client: %w", err)
	}
	return &Gemini{client: client}, nil
}

func (*Gemini) Name() string { return "gemini" }

func (g *Gemini) Summarise(ctx context.Context, title, body string) (string, error) {
	prompt := fmt.Sprintf("Summarise this saved web page in 2-3 sentences for a personal knowledge library. Title: %s\n\n%s", title, truncate(body, 12000))
	resp, err := g.client.Models.GenerateContent(ctx, geminiGenModel, genai.Text(prompt), nil)
	if err != nil {
		return "", fmt.Errorf("gemini summarise: %w", err)
	}
	return resp.Text(), nil
}

func (g *Gemini) Tag(ctx context.Context, title, body string) ([]string, error) {
	prompt := fmt.Sprintf("Generate 3-6 short lowercase topic tags for this saved page. Title: %s\n\n%s", title, truncate(body, 12000))
	cfg := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   &genai.Schema{Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
	}
	resp, err := g.client.Models.GenerateContent(ctx, geminiGenModel, genai.Text(prompt), cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini tag: %w", err)
	}
	var tags []string
	if err := json.Unmarshal([]byte(resp.Text()), &tags); err != nil {
		return nil, fmt.Errorf("parsing gemini tags: %w", err)
	}
	return tags, nil
}

func (g *Gemini) Embed(ctx context.Context, text string) ([]float32, error) {
	dims := embedDims
	resp, err := g.client.Models.EmbedContent(ctx, geminiEmbedModel, genai.Text(truncate(text, 8000)), &genai.EmbedContentConfig{OutputDimensionality: &dims})
	if err != nil {
		return nil, fmt.Errorf("gemini embed: %w", err)
	}
	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("gemini embed: empty response")
	}
	return resp.Embeddings[0].Values, nil
}

func (g *Gemini) ParseQuery(_ context.Context, q string) (string, error) { return q, nil }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
```

Update `FromEnv` in `ai.go` to return gemini when `AI_PROVIDER=gemini` (error if `GEMINI_API_KEY` empty).

- [ ] **Step 3: Verify**

Run: `go get google.golang.org/genai && go test ./internal/ai/` → PASS (gemini test skips locally; run once with a real key: `GEMINI_API_KEY=... go test ./internal/ai/ -run TestGemini -v` → PASS).

- [ ] **Step 4: Commit**

```bash
git add apps/api/internal/ai apps/api/go.mod apps/api/go.sum
git commit -m "feat(ai): gemini flash-lite + embedding provider"
```

---

### Task 7: Enrichment pipeline + River worker (idempotency test)

**Files:**
- Create: `apps/api/internal/enrich/classify.go`, `apps/api/internal/enrich/pipeline.go`, `apps/api/internal/jobs/enrich.go`, `apps/api/internal/ai/fake.go` (test helper provider)
- Test: `apps/api/internal/enrich/classify_test.go`, `apps/api/internal/enrich/pipeline_test.go`

**Interfaces:**
- Consumes: `store.Store` (Task 3), `ai.Provider` (Task 4), `enrich.Extractor` (Task 5).
- Produces:
```go
// internal/enrich
func Classify(url string, ex Extraction) string // returns a card type string
type Pipeline struct { Store *store.Store; AI ai.Provider; Extractor Extractor }
func (p *Pipeline) Run(ctx context.Context, userID, itemID uuid.UUID) error

// internal/jobs
type EnrichArgs struct { UserID uuid.UUID `json:"user_id"`; ItemID uuid.UUID `json:"item_id"` }
func (EnrichArgs) Kind() string { return "enrich_item" }
type EnrichWorker struct { river.WorkerDefaults[EnrichArgs]; Pipeline *enrich.Pipeline }
func NewRiverClient(pool *pgxpool.Pool, p *enrich.Pipeline, workersOn bool) (*river.Client[pgx.Tx], error)

// internal/ai — fake for tests
func NewFake() *Fake // Summarise → "summary of <title>", Tag → ["fake","tags"], Embed → deterministic 768-vector derived from text hash
```

- [ ] **Step 1: Write failing classify test** (table-driven)

```go
package enrich

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name, url string
		want      string
	}{
		{"youtube", "https://www.youtube.com/watch?v=abc", "video"},
		{"tweet", "https://x.com/user/status/123", "tweet"},
		{"twitter", "https://twitter.com/user/status/123", "tweet"},
		{"image ext", "https://cdn.site.com/photo.jpg", "image"},
		{"default article", "https://blog.example.com/post", "article"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.url, Extraction{}); got != tt.want {
				t.Errorf("Classify(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Implement `Classify`** (host/path/extension heuristics; default "article"). Run → PASS.

- [ ] **Step 3: Write failing pipeline test (includes idempotency)**

```go
package enrich_test

// uses testStore-style setup (copy the helper from store_test or export a
// storetest helper), an httptest server serving testdata/article.html,
// ai.NewFake(), and enrich.Pipeline.

func TestPipelineRunIsIdempotent(t *testing.T) {
	s := newTestStore(t) // same pattern as internal/store test helper
	ctx := context.Background()
	userID := uuid.New()
	s.Queries.EnsureUser(ctx, userID)
	srv := serveFixture(t, "testdata/article.html")
	item, _ := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: srv.URL})

	p := &enrich.Pipeline{Store: s, AI: ai.NewFake(), Extractor: enrich.NewTrafilatura(srv.Client())}
	if err := p.Run(ctx, userID, item.ID); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first, _ := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
	if first.Status != "enriched" || first.Summary == "" || len(first.Tags) == 0 {
		t.Fatalf("not enriched: %+v", first)
	}
	if err := p.Run(ctx, userID, item.ID); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second, _ := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
	if second.Summary != first.Summary || second.Title != first.Title || second.Status != first.Status {
		t.Errorf("second run changed state:\nfirst  %+v\nsecond %+v", first, second)
	}
}

func TestPipelineNoopProviderStillCompletes(t *testing.T) {
	// same setup with ai.NewNoop(): Run must return nil error,
	// item status "enriched", summary empty, no embedding row.
}
```

- [ ] **Step 4: Implement `ai.Fake`, `Pipeline.Run`, run tests → PASS**

`pipeline.go` core:
```go
func (p *Pipeline) Run(ctx context.Context, userID, itemID uuid.UUID) error {
	q := p.Store.Queries
	item, err := q.GetItem(ctx, db.GetItemParams{UserID: userID, ID: itemID})
	if err != nil {
		return fmt.Errorf("loading item %s: %w", itemID, err)
	}

	ex, err := p.Extractor.Extract(ctx, item.Url)
	if err != nil {
		if serr := q.SetItemStatus(ctx, db.SetItemStatusParams{UserID: userID, ID: itemID, Status: "failed"}); serr != nil {
			return fmt.Errorf("marking failed after extract error %v: %w", err, serr)
		}
		return fmt.Errorf("extracting %s: %w", item.Url, err)
	}
	cardType := Classify(item.Url, ex)
	if err := q.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: itemID,
		Title: ex.Title, Body: ex.Body, LeadImageUrl: ex.LeadImageURL, CardType: cardType,
	}); err != nil {
		return fmt.Errorf("saving extraction: %w", err)
	}

	summary, err := p.AI.Summarise(ctx, ex.Title, ex.Body)
	if err != nil {
		return fmt.Errorf("summarising: %w", err) // River retries; save stays intact
	}
	tags, err := p.AI.Tag(ctx, ex.Title, ex.Body)
	if err != nil {
		return fmt.Errorf("tagging: %w", err)
	}
	if err := q.UpdateItemEnrichment(ctx, db.UpdateItemEnrichmentParams{UserID: userID, ID: itemID, Summary: summary, Tags: tags}); err != nil {
		return fmt.Errorf("saving enrichment: %w", err)
	}

	embedInput := ex.Title + "\n" + summary + "\n" + ex.Body
	vec, err := p.AI.Embed(ctx, embedInput)
	switch {
	case errors.Is(err, ai.ErrNotSupported):
		// noop provider: FTS-only mode, no embedding row
	case err != nil:
		return fmt.Errorf("embedding: %w", err)
	default:
		if err := q.UpsertEmbedding(ctx, db.UpsertEmbeddingParams{ItemID: itemID, UserID: userID, Embedding: pgvector.NewVector(vec)}); err != nil {
			return fmt.Errorf("saving embedding: %w", err)
		}
	}
	return q.SetItemStatus(ctx, db.SetItemStatusParams{UserID: userID, ID: itemID, Status: "enriched"})
}
```

- [ ] **Step 5: River wiring `internal/jobs/enrich.go`**

```go
package jobs

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
)

type EnrichArgs struct {
	UserID uuid.UUID `json:"user_id"`
	ItemID uuid.UUID `json:"item_id"`
}

func (EnrichArgs) Kind() string { return "enrich_item" }

type EnrichWorker struct {
	river.WorkerDefaults[EnrichArgs]
	Pipeline *enrich.Pipeline
}

func (w *EnrichWorker) Work(ctx context.Context, job *river.Job[EnrichArgs]) error {
	return w.Pipeline.Run(ctx, job.Args.UserID, job.Args.ItemID)
}

func NewRiverClient(pool *pgxpool.Pool, p *enrich.Pipeline, workersOn bool) (*river.Client[pgx.Tx], error) {
	cfg := &river.Config{}
	if workersOn {
		workers := river.NewWorkers()
		river.AddWorker(workers, &EnrichWorker{Pipeline: p})
		cfg.Workers = workers
		cfg.Queues = map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 5}}
	}
	client, err := river.NewClient(riverpgxv5.New(pool), cfg)
	if err != nil {
		return nil, fmt.Errorf("creating river client: %w", err)
	}
	return client, nil
}
```

Run: `go build ./... && go test ./...` → PASS.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat(enrich): staged idempotent pipeline + river worker"
```

---

### Task 8: HTTP API + serve/work/all commands

**Files:**
- Create: `apps/api/internal/api/server.go`, `apps/api/internal/api/middleware.go`
- Modify: `apps/api/cmd/openmind/main.go` (add serve|work|all)
- Create: `apps/api/Dockerfile`
- Test: `apps/api/internal/api/server_test.go`

**Interfaces:**
- Consumes: `api.ServerInterface` (Task 2), `store.Store`, `jobs.NewRiverClient`, `ai.FromEnv`, `enrich` (Tasks 3–7).
- Produces: `api.NewServer(s *store.Store, riverClient *river.Client[pgx.Tx], provider ai.Provider) http.Handler`; dev-user middleware injecting fixed UUID `00000000-0000-0000-0000-000000000001` into context (`EnsureUser`ed at startup); search handler delegated to Task 9 (returns `[]` until then).

- [ ] **Step 1: Write failing handler test**

Test with real store + river client (insert-only), httptest server:
```go
func TestCreateItemIsInstantAndPending(t *testing.T) {
	// setup store + river insert-only client against test db
	srv := httptest.NewServer(api.NewServer(s, riverClient, ai.NewNoop()))
	start := time.Now()
	resp := postJSON(t, srv.URL+"/items", `{"url":"https://example.com/a"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("save took %v; capture must be instant", elapsed)
	}
	var item map[string]any
	json.NewDecoder(resp.Body).Decode(&item)
	if item["status"] != "pending" {
		t.Errorf("status = %v, want pending", item["status"])
	}
	// and a river job row exists for kind enrich_item
}
```
Plus `TestListItems` returning the created item, newest first.

- [ ] **Step 2: Implement `server.go`**

`NewServer` builds chi router, wraps generated `HandlerFromMux`, implements `ServerInterface`:
- `CreateItem`: decode body → validate URL (`url.ParseRequestURI`, http/https only → else 400) → `EnsureUser`-backed dev user from context → `Queries.CreateItem` → `riverClient.Insert(ctx, jobs.EnrichArgs{UserID, ItemID}, nil)` → 201 with mapped `api.Item`. Job insert failure: log + still 201 (save is sacred; enrichment can be re-queued).
- `ListItems`: `Queries.ListItems` (default limit 50, cap 200) → JSON array.
- `SearchItems`: placeholder returning `[]` (Task 9 replaces).
- One `toAPIItem(db.Item) api.Item` mapper.

- [ ] **Step 3: Wire cmd subcommands**

In `run()`: build pool → `store.New` → `ai.FromEnv()` → pipeline with `enrich.NewTrafilatura(nil)` →
- `serve`: river client `workersOn=false` (insert-only, no Start), `http.ListenAndServe(":"+port, api.NewServer(...))`
- `work`: river client `workersOn=true`, `client.Start(ctx)`, block on signal
- `all`: both (start river, then serve). Graceful shutdown on SIGINT: `client.Stop(ctx)`, `server.Shutdown(ctx)`.
Startup: `Queries.EnsureUser(ctx, devUserID)`.

`Dockerfile` (multi-stage: `golang:1.24-alpine` build → `gcr.io/distroless/static`, `ENTRYPOINT ["/openmind"]`).

- [ ] **Step 4: Run tests → PASS; manual smoke**

```bash
task migrate
DATABASE_URL=postgres://openmind:openmind@localhost:5433/openmind AI_PROVIDER=noop go run ./cmd/openmind all &
curl -s -XPOST localhost:8080/items -d '{"url":"https://go.dev/blog/error-handling-and-go"}' -H 'content-type: application/json'
sleep 5 && curl -s localhost:8080/items | head -c 600   # status should be "enriched"
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(api): http handlers, dev-user middleware, serve/work/all commands, dockerfile"
```

---

### Task 9: Hybrid search (FTS + pgvector, RRF)

**Files:**
- Create: `apps/api/internal/store/queries/search.sql`, `apps/api/internal/search/search.go`
- Modify: `apps/api/internal/api/server.go` (real `SearchItems`)
- Test: `apps/api/internal/search/search_test.go`

**Interfaces:**
- Consumes: store, `ai.Provider.Embed`.
- Produces: `search.Hybrid(ctx, s *store.Store, p ai.Provider, userID uuid.UUID, q string, limit int) ([]search.Result, error)` where `Result{Item db.Item; Score float64}`.

- [ ] **Step 1: Add sqlc queries `search.sql`**

```sql
-- name: SearchFTS :many
SELECT *, ts_rank(search_tsv, websearch_to_tsquery('english', $2))::float8 AS rank
FROM items
WHERE user_id = $1 AND search_tsv @@ websearch_to_tsquery('english', $2)
ORDER BY rank DESC LIMIT $3;

-- name: SearchVector :many
SELECT i.*, (1 - (e.embedding <=> $2))::float8 AS similarity
FROM item_embeddings e JOIN items i ON i.id = e.item_id
WHERE e.user_id = $1
ORDER BY e.embedding <=> $2 LIMIT $3;
```

Run `task generate:sqlc` → new methods on `db.Queries`.

- [ ] **Step 2: Write failing test**

Real store; insert two items for one user (via CreateItem + UpdateItemExtraction/Enrichment) — one about "sourdough fermentation", one about "rust borrow checker"; embed with `ai.NewFake()` (deterministic vectors) via `UpsertEmbedding`. Assert:
- `Hybrid(..., "sourdough", ...)` returns the bread item first.
- With `ai.NewNoop()` (Embed unsupported), FTS-only still returns it — no error.
- Another user's search returns nothing (tenant scoping).

- [ ] **Step 3: Implement `search.go` (RRF in Go — simplest correct fusion)**

```go
func Hybrid(ctx context.Context, s *store.Store, p ai.Provider, userID uuid.UUID, q string, limit int) ([]Result, error) {
	const k = 60
	scores := map[uuid.UUID]float64{}
	items := map[uuid.UUID]db.Item{}

	fts, err := s.Queries.SearchFTS(ctx, db.SearchFTSParams{UserID: userID, WebsearchToTsquery: q, Limit: int32(limit * 2)})
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	for rank, row := range fts {
		scores[row.ID] += 1.0 / float64(k+rank+1)
		items[row.ID] = rowToItem(row)
	}

	if vec, err := p.Embed(ctx, q); err == nil {
		vres, err := s.Queries.SearchVector(ctx, db.SearchVectorParams{UserID: userID, Embedding: pgvector.NewVector(vec), Limit: int32(limit * 2)})
		if err != nil {
			return nil, fmt.Errorf("vector search: %w", err)
		}
		for rank, row := range vres {
			scores[row.ID] += 1.0 / float64(k+rank+1)
			items[row.ID] = vrowToItem(row)
		}
	} else if !errors.Is(err, ai.ErrNotSupported) {
		slog.Warn("query embedding failed; falling back to FTS only", "err", err)
	}
	ids := make([]uuid.UUID, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return scores[ids[i]] > scores[ids[j]] })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	results := make([]Result, 0, len(ids))
	for _, id := range ids {
		results = append(results, Result{Item: items[id], Score: scores[id]})
	}
	return results, nil
}
```
(sqlc names params after expressions; check generated `db` code for actual param struct field names and use those.)

- [ ] **Step 4: Wire `SearchItems` handler to `search.Hybrid`; run all tests → PASS. Step 5: Commit**

```bash
git add -A && git commit -m "feat(search): hybrid fts + pgvector with rrf fusion"
```

---

### Task 10: Web stub + packages (ui tokens, api-client)

**Files:**
- Create: `packages/api-client/package.json`, `packages/api-client/src/index.ts`, generated `packages/api-client/src/schema.d.ts`
- Create: `packages/ui/package.json`, `packages/ui/src/tokens.ts`
- Create: `apps/web/` (Next.js app: `package.json`, `next.config.ts`, `tsconfig.json`, `app/layout.tsx`, `app/page.tsx`)

**Interfaces:**
- Consumes: `openapi.yaml`.
- Produces: `@openmind/api-client` exporting `createClient(baseUrl)` (openapi-fetch typed against schema); `@openmind/ui` exporting `tokens` (paper `#F7F6F3`, ink `#191918`, cobalt `#2438FF`, line `#E7E5DF`, fonts per docs/design).

- [ ] **Step 1: api-client**

`packages/api-client/package.json`:
```json
{
  "name": "@openmind/api-client",
  "version": "0.0.1",
  "type": "module",
  "main": "src/index.ts",
  "scripts": { "generate": "openapi-typescript ../../openapi.yaml -o src/schema.d.ts" },
  "dependencies": { "openapi-fetch": "^0.13.0" },
  "devDependencies": { "openapi-typescript": "^7.6.0", "typescript": "^5.8.0" }
}
```

`src/index.ts`:
```ts
import createFetchClient from "openapi-fetch";
import type { paths } from "./schema";

export function createClient(baseUrl: string) {
  return createFetchClient<paths>({ baseUrl });
}
export type { paths };
```

Run: `pnpm install && pnpm --filter @openmind/api-client generate` → `schema.d.ts` created; commit it (generated but checked in, per contract workflow).

- [ ] **Step 2: ui tokens**

`packages/ui/src/tokens.ts`:
```ts
export const tokens = {
  color: {
    paper: "#F7F6F3",
    ink: "#191918",
    cobalt: "#2438FF",
    line: "#E7E5DF",
  },
  font: {
    sans: "'Instrument Sans', 'IBM Plex Sans', sans-serif",
    mono: "'JetBrains Mono', 'IBM Plex Mono', monospace",
    quote: "'Newsreader', serif",
  },
} as const;
```
(`package.json` name `@openmind/ui`, main `src/tokens.ts`.)

- [ ] **Step 3: Next.js stub**

Minimal by hand (no create-next-app churn): `app/page.tsx` fetches `GET /items` via `createClient(process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080")` in a server component and renders a plain `<ul>` of titles/urls on `tokens.color.paper`. That's the whole UI this milestone.

- [ ] **Step 4: Verify + commit**

Run: `pnpm install && pnpm turbo run build --filter=web` → builds. (Don't leave a dev server running.)
```bash
git add -A && git commit -m "feat(web): next.js stub + generated ts client + ui tokens"
```

---

### Task 11: Extraction bake-off

**Files:**
- Create: `apps/api/internal/enrich/readability.go`, `apps/api/internal/enrich/jina.go`, `apps/api/cmd/bakeoff/main.go`, `apps/api/testdata/bakeoff.json`
- Create: `docs/bakeoff-results.md` (generated scorecard skeleton, then reviewed)

**Interfaces:**
- Consumes: `enrich.Extractor` (Task 5).
- Produces: `enrich.NewReadability(client) *Readability`, `enrich.NewJina(client, apiKey) *Jina` (both implement `Extractor`); `bakeoff` CLI: reads corpus JSON `[{"url": "...", "type": "article"}]`, runs all three extractors per URL, writes a markdown table (title, body length, first 200 body chars, image?, latency, error) to stdout.

- [ ] **Step 1: Implement Readability** (`github.com/go-shiori/go-readability`: fetch like trafilatura, `readability.FromReader(resp.Body, parsedURL)` → `article.Title`, `article.TextContent`, `article.Image`). Reuse the existing fixture test pattern with a second test asserting it implements `Extractor`.

- [ ] **Step 2: Implement Jina** (`GET https://r.jina.ai/<url>`, header `Accept: application/json` and `Authorization: Bearer <key>` when key set; parse `{"data":{"title":..., "content":..., "images":...}}`; without key still works rate-limited). Unit test with `httptest` faking the Jina response shape (point base URL at test server via unexported field or option).

- [ ] **Step 3: Corpus `testdata/bakeoff.json`** — 18 real URLs, mix per spec: 5 long-form articles (e.g. paulgraham.com essay, NYT-adjacent blog, Substack post), 3 recipes, 2 product pages, 2 tweets/X, 2 YouTube, 2 image-heavy posts, 1 paywalled (medium.com member post), 1 JS-heavy SPA. Pick live URLs at implementation time; record chosen URLs in the JSON with their expected card type.

- [ ] **Step 4: CLI `cmd/bakeoff/main.go`** — flags `-corpus` (default `testdata/bakeoff.json`) and `-extractor` (default all); loops URL × extractor with 20s timeout each, prints one markdown table section per URL. No DB required.

- [ ] **Step 5: Run + score**

```bash
cd apps/api && go run ./cmd/bakeoff > ../../docs/bakeoff-results.md
```
Then manually score each cell 0–2 on title/body/noise/image in `docs/bakeoff-results.md`, add a totals table and a **Decision** section naming the default extractor and confirming Jina as config-gated fallback. If the winner isn't trafilatura, swap the default in `cmd/openmind` wiring (one line) and note it.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat(bakeoff): readability + jina extractors, corpus, scorecard"
```

---

### Task 12: End-to-end verification + wrap-up

**Files:**
- Modify: `TODO.md`
- Modify (if needed): `docs/self-hosting.md` (create: env vars table from `.env.example`)

**Interfaces:** none — verification and bookkeeping.

- [ ] **Step 1: Definition-of-done run (real Gemini)**

```bash
docker compose build && AI_PROVIDER=gemini GEMINI_API_KEY=<key> docker compose up -d
curl -s -XPOST localhost:8080/items -d '{"url":"<a real article url>"}' -H 'content-type: application/json'
# wait ~15s, then:
curl -s localhost:8080/items | python3 -m json.tool          # status enriched, summary + tags populated
curl -s 'localhost:8080/search?q=<word appearing ONLY in the AI summary>' | python3 -m json.tool  # returns the item
```
Record the actual URL + summary-only search term used in the task notes/commit message.

- [ ] **Step 2: Noop parity run**

```bash
AI_PROVIDER=noop docker compose up -d --force-recreate api
# save another URL; confirm enriched (no summary), FTS finds it by a title word
```

- [ ] **Step 3: Full test + lint sweep**

Run: `task test && task lint` → all green.

- [ ] **Step 4: Update TODO.md**

Move Milestone 0 items to Done (with one-line outcomes, e.g. bake-off winner, Next.js decision). Promote Milestone 1 "Later" items to Now/Next. Keep masonry perf spike + Karakeep deep-dive + naming decision as their own Next items.

- [ ] **Step 5: Write `docs/self-hosting.md`** — quickstart (`docker compose up`, env var table from `.env.example`). Brief; expanded in Milestone 1.

- [ ] **Step 6: Final commit**

```bash
git add -A && git commit -m "chore: milestone 0 complete — e2e verified, todo + self-hosting docs updated"
```
