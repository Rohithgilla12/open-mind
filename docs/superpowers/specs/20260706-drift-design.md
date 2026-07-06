# Drift — Resurfacing — Design

Date: 2026-07-06 · Status: Designed autonomously (user: "do next milestones") · Builds the designed-but-unbuilt **Drift** screen (docs/design/README.md §5); pairs with Desk (keep → pin)

## Goal

A calm, finite daily resurfacing of forgotten saves: Openmind shows you a small handful of older, never-revisited items one at a time; for each you **keep** it (pins to your Desk) or **let it go**. Signature "kept by a machine" feature, fully designed, self-contained, no new dependency.

## Resurfacing algorithm (locked, v1)

Track `items.last_drifted_at timestamptz` (null = never surfaced in Drift). A Drift **candidate** is an item that is:
- `status = 'enriched'` (skip pending/failed — nothing to resurface yet),
- NOT pinned (`pinned_at IS NULL` — already on the desk, not "forgotten"),
- not recently drifted (`last_drifted_at IS NULL OR last_drifted_at < now() - interval '30 days'`).

Ordering biases toward **older + never-revisited**: `ORDER BY last_drifted_at NULLS FIRST, created_at ASC` (never-drifted first, then oldest saved). Batch size **5** per day ("your drift for today"). This is deliberately simple; a fuller spaced-repetition weighting can come later.

## API

- `GET /drift` → `{ items: Item[], total: int }` — **read-only** (no mutation): up to 5 candidates in the ordering above, plus `total` = count of all current candidates (for the "n of total" mono line). User-scoped.
- `POST /drift/{id}` body `{ keep: bool }` — the action: always sets `last_drifted_at = now()` (so it won't resurface for 30 days); if `keep` also sets `pinned_at = now()` (→ appears on Desk). 200 (empty/ok) | 404 (cross-tenant/missing). One statement: `UPDATE items SET last_drifted_at = now(), pinned_at = CASE WHEN $3 THEN now() ELSE pinned_at END, updated_at = now() WHERE user_id = $1 AND id = $2`. Bearer + rate-limited (guarded).
- sqlc: `ListDriftCandidates(user_id, limit) :many`, `CountDriftCandidates(user_id) :one`, `DriftAction(user_id, id, keep bool) :execrows`.
- No `Item` schema change needed (last_drifted_at is internal; not exposed).

## Web

- **`/drift` route** (full-screen experience per design §5, adapted to our stack): radial dark background (`#242019→#161410`), centered column. Top: mono "Drift · {index+1} of {total} today" + progress dots (gold = seen/current, faint = todo). Center: a single **floating card** (`floaty` keyframe — the mockup's `translateY(-8px)` 5.5s ease-in-out) rendering the current item (lead image/gradient, Newsreader title, summary, palette dots, mono meta with the saved-age line "Saved N months ago · never revisited"). Buttons: **Let go** (outline, light) and **Keep on my desk** (gold fill). ✕ / Esc closes back to The Mind. On action → POST /drift/{id}{keep} → advance to the next card (client holds the batch fetched once). **Completion state**: gold ❍, "That's your drift for today.", tally "kept X · let Y go", "Back to The Mind". Empty (no candidates) state: "Nothing to drift — your mind is all caught up."
- Data: the `/drift` page server-fetches the batch (`GET /drift`) once; a client component drives the one-at-a-time flow + POSTs actions via a `/api/drift/[id]` proxy. Keep is a real pin (shows on Desk immediately).
- **Sidebar**: the "Drift" nav item (currently muted "soon") becomes a live link to `/drift`. (After this, all three primary nav items — Desk / The Mind / Drift — are live.)
- Tokens-only except the intentional dark Drift canvas (dark is part of the Drift design, unlike the rest of the warm app) — use the design's exact dark values via tokens where they exist, literal dark gradient where they don't (documented; Drift is the one deliberately-dark screen).

## Testing

- Go: migration additive (last_drifted_at column); `ListDriftCandidates` excludes pinned/pending/recently-drifted, orders never-drifted-then-oldest, user-scoped, respects limit; `CountDriftCandidates` matches; `DriftAction` sets last_drifted_at (+ pins when keep), user-scoped (cross-tenant → 0 rows → 404); a drifted item drops out of the next candidate list (won't immediately re-surface). GET /drift + POST /drift/{id} handlers (batch+total, keep pins, letgo doesn't, 404).
- Web: build + lint; e2e on box (GET /drift returns candidates; POST keep pins → item appears on /desk; POST letgo marks it; drifted items excluded from a subsequent GET /drift).

## Out of scope

True spaced-repetition scheduling, per-day hard quota enforcement server-side (the 5-batch + 30-day cooldown is the finiteness), Drift notifications/reminders, a revisit counter, animations beyond the floaty card + fade transitions.

## Execution

Subagent-driven. Reuse Desk's pin plumbing (keep = pin), Grid/ItemCard styling primitives for the card, and the feeds/lens/desk page + proxy patterns. Deploy api+web after whole-branch review; restart cloudflared.
