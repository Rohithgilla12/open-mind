# Card Detail + Export + Delete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Type-aware item detail page (reader view), item deletion, JSON export.

**Architecture:** Contract-first: `GET/DELETE /items/{id}` (ItemDetail = Item + body) and `GET /export` in openapi.yaml → regenerated Go server + TS types → Go handlers over new sqlc queries → web detail page + proxy routes. Spec: `docs/superpowers/specs/20260703-card-detail-export-design.md`.

**Tech Stack:** Existing stack only; no new deps.

## Global Constraints

- Every query user-scoped; cross-tenant access must 404 (not 403 — no existence oracle).
- ItemDetail = all Item fields + required `body`. Export = JSON array of ItemDetail ordered created_at ASC. Web export route sets `Content-Disposition: attachment; filename="openmind-export-<YYYYMMDD>.json"`.
- Rate limiter guard extends to `GET /export`.
- Web: tokens-only styling; server-side data via apiFetch (pass `req` in proxy routes so Bearer works); reader column ~65ch; delete uses danger token + native confirm.
- Generated code never hand-edited; errors `%w`; no banner comments; suite `go test -p 1 ./...` + web build/lint green; commit per task.

---

### Task 1: Contract + Go — get/delete/export

**Files:**
- Modify: `openapi.yaml`, `apps/api/internal/store/queries/items.sql`, `apps/api/internal/api/server.go`, `apps/api/internal/api/ratelimit.go`
- Generated: `apps/api/internal/api/gen.go`, `apps/api/internal/store/db/*`, `packages/api-client/src/schema.d.ts`
- Test: extend `apps/api/internal/api/server_test.go`

**Interfaces:**
- Produces (openapi): `ItemDetail` schema (allOf Item + required body); `getItem` (GET /items/{id}, 200 ItemDetail | 404), `deleteItem` (DELETE /items/{id}, 204 | 404), `exportItems` (GET /export, 200 array of ItemDetail). Path param `id: uuid`.
- Produces (sqlc): `DeleteItem(ctx, DeleteItemParams{UserID, ID}) (int64, error)` via `:execrows`; `ListItemsForExport(ctx, userID) ([]Item, error)` ordered created_at ASC.
- Produces (Go): `toAPIItemDetail(db.Item) ItemDetail` (Item mapping + Body).

- [ ] **Step 1: openapi.yaml** — add:

```yaml
  /items/{id}:
    get:
      operationId: getItem
      parameters:
        - { name: id, in: path, required: true, schema: { type: string, format: uuid } }
      responses:
        "200":
          description: item detail
          content:
            application/json:
              schema: { $ref: "#/components/schemas/ItemDetail" }
        "404":
          description: not found
    delete:
      operationId: deleteItem
      parameters:
        - { name: id, in: path, required: true, schema: { type: string, format: uuid } }
      responses:
        "204": { description: deleted }
        "404": { description: not found }
  /export:
    get:
      operationId: exportItems
      responses:
        "200":
          description: full library export
          content:
            application/json:
              schema:
                type: array
                items: { $ref: "#/components/schemas/ItemDetail" }
```
and schema:
```yaml
    ItemDetail:
      allOf:
        - $ref: "#/components/schemas/Item"
        - type: object
          required: [body]
          properties:
            body: { type: string }
```

- [ ] **Step 2: `task generate`**; sqlc queries:

```sql
-- name: DeleteItem :execrows
DELETE FROM items WHERE user_id = $1 AND id = $2;

-- name: ListItemsForExport :many
SELECT * FROM items WHERE user_id = $1 ORDER BY created_at ASC;
```

- [ ] **Step 3: Failing handler tests** — `TestGetItemDetail` (create+fetch → 200 with body field; other-user's id → 404; random uuid → 404), `TestDeleteItem` (204 → subsequent GET 404; other-user delete → 404 and row survives), `TestExportItems` (2 items → array of 2, bodies present, ASC order).

- [ ] **Step 4: Implement handlers** — `GetItem`: `Queries.GetItem`, `pgx.ErrNoRows` → 404 JSON; `DeleteItem`: execrows 0 → 404 else 204 no body; `ExportItems`: `ListItemsForExport` → map `toAPIItemDetail` → JSON array (plain `writeJSON`). Extend `guarded` in ratelimit.go with `GET /export`. Run suite → PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(api): item detail, delete, and export endpoints"
```

---

### Task 2: Web — detail page, delete, export

**Files:**
- Create: `apps/web/app/item/[id]/page.tsx`, `apps/web/app/item/[id]/DeleteButton.tsx`, `apps/web/app/api/items/[id]/route.ts` (DELETE proxy), `apps/web/app/api/export/route.ts`
- Modify: `apps/web/components/ItemCard.tsx` (wrap in Link), `apps/web/app/page.tsx` (export link), `apps/web/lib/types.ts` (ItemDetail type from generated paths)

**Interfaces:**
- Consumes: Task 1 endpoints via `apiFetch`; `tokens`; generated `paths` types.
- Produces: `/item/[id]` (server component; `params: Promise<{id: string}>` — await it); DELETE proxy `/api/items/[id]`; `/api/export` with `Content-Disposition: attachment; filename="openmind-export-<YYYYMMDD>.json"` (date from `new Date()` formatted YYYYMMDD).

- [ ] **Step 1: proxy routes**

`app/api/items/[id]/route.ts`:
```ts
import { NextResponse } from "next/server";
import { apiFetch } from "../../../../lib/api";

export async function DELETE(req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const res = await apiFetch(`/items/${id}`, { method: "DELETE" }, req);
  return new NextResponse(null, { status: res.status });
}
```

`app/api/export/route.ts`:
```ts
import { apiFetch } from "../../../lib/api";

export async function GET(req: Request) {
  const res = await apiFetch("/export", undefined, req);
  const stamp = new Date().toISOString().slice(0, 10).replaceAll("-", "");
  return new Response(res.body, {
    status: res.status,
    headers: {
      "content-type": "application/json",
      "content-disposition": `attachment; filename="openmind-export-${stamp}.json"`,
    },
  });
}
```

- [ ] **Step 2: detail page** — `app/item/[id]/page.tsx` server component: `apiFetch(\`/items/${id}\`)`; non-ok → `notFound()`. Render per spec: header row (back link "← library", DeleteButton), type-aware body:
  - note: body text, `tokens.font.quote`, italic, pre-wrap.
  - image: `<img>` max-width 100%, title below, source link.
  - tweet/video: summary/body as styled quote + source link (video shows leadImageUrl thumb first).
  - default: leadImageUrl (when present), `<h1>` title, domain + "Open original ↗" link, summary in a bordered callout, tag chips (line border, mono font, lowercase), body split on `\n\n` into `<p>`s inside a `maxWidth: "65ch"` column.
  - status pending → banner "Still enriching…" (cobalt); failed → muted note + original link.

- [ ] **Step 3: DeleteButton.tsx** (client): danger-token text button; `confirm("Delete this item?")` → `fetch(\`/api/items/${id}\`, {method: "DELETE"})` → 204 → `router.push("/")` + `router.refresh()`; error → inline message.

- [ ] **Step 4: card link + export link** — wrap ItemCard content in `<Link href={\`/item/${item.id}\`}>` (style: no underline, inherit colour); add to page.tsx a right-aligned "Export JSON" link (`<a href="/api/export">`, mono font, small).

- [ ] **Step 5: Verify + commit**

Run: `pnpm turbo run build --filter=web && pnpm turbo run lint --filter=web` → PASS.
```bash
git add apps/web packages && git commit -m "feat(web): item detail reader view, delete, json export"
```

---

### Task 3: E2E + docs + wrap-up

**Files:**
- Modify: `TODO.md`, `docs/self-hosting.md` (export mention)

- [ ] **Step 1: Local compose e2e** — fresh build (`OPENMIND_TOKEN=devtoken docker compose up -d --build api web`, db already up): save URL, wait for enrich; `GET /api/export` with Bearer → array with body; open `/item/<id>` with cookie (login via curl) → 200 HTML containing the title; `DELETE /api/items/<id>` with Bearer → 204; export now one fewer. Record outputs. Stop web+api after.
- [ ] **Step 2: Docs** — self-hosting.md: one paragraph on export (where the link lives, what's in the file). TODO.md: "Card detail + reader view" + "JSON export" → Done (dated, evidence).
- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "feat: card detail e2e evidence, docs, todo"
```

- [ ] **Step 4 (controller):** merge, push, rsync + rebuild api/web on the deploy box, re-run the e2e trio there.
