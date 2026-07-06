# RSS/Atom Feed Subscriptions — Design

Date: 2026-07-06 · Status: Designed autonomously (user: "continue, you pick"; auth deferred) · Advances M2 imports / "capture from anywhere"

## Goal

Subscribe to RSS/Atom feeds: add a feed URL → its current entries are backfilled as items immediately, and a periodic poll saves new entries as they publish. No new infra (River periodic jobs run in the existing worker binary), no new dependency (stdlib `encoding/xml`), SSRF-safe fetches, idempotent (never double-saves an entry).

## Scope decisions (locked)

- **Subscriptions, not one-shot** — feeds persist and are re-polled.
- **RSS 2.0 + Atom via stdlib `encoding/xml`** (covers ~95% of feeds); RSS 1.0/RDF and podcast-specific tags out of scope.
- Entries become normal **article items** (url = entry link) going through the standard enrichment pipeline (cheap models, async). No feed-specific card type.
- Deferred: Omnivore-JSON import, PDF capture (separate backlog items).

## Data model

Migration `0005_feeds.sql`:
```
feeds (
  id uuid pk, user_id uuid not null,
  url text not null,              -- the feed URL (unique per user)
  title text not null default '', -- feed <title>
  site_url text not null default '',
  last_polled_at timestamptz,     -- null until first poll
  last_status text not null default '', -- 'ok' | 'error: ...' for UI
  created_at timestamptz not null default now()
)
unique (user_id, url); index (user_id).
```
No per-entry table — dedup is by item URL (an entry already saved as an item is skipped), reusing the importer's `ListItemURLs` approach, user-scoped.

## Feed parser (`internal/feeds/parse.go`, no deps)

`Parse(data []byte) (Feed, error)` where `Feed{Title, SiteURL string; Entries []Entry}`, `Entry{URL, Title string}`. Detect RSS 2.0 (`<rss><channel><item>`, entry link = `<link>`) vs Atom (`<feed><entry>`, link = `<link href rel="alternate">` or first link; title from `<title>`). Resolve relative entry links against the feed/site base. Skip entries with no usable http(s) link. Malformed XML → error. TDD with small RSS + Atom fixtures.

## Feed service (`internal/feeds/service.go`)

Consumes the store, `enrich.SafeHTTPClient` (SSRF-safe, redirect-capped, size-limited), the parser, and the River client (to enqueue enrichment).
- `Add(ctx, userID, feedURL) (Feed, addedCount, error)` — validate http(s); fetch+parse; insert the feed row (title/site from parse); backfill: for each entry URL not already an item for this user (and not dupe within the feed), create a pending article item + enqueue `EnrichArgs`; set `last_polled_at`/`last_status`. Dupe feed URL → 409-worthy sentinel.
- `Refresh(ctx, feed) (newCount, error)` — same fetch/parse/dedup/create/enqueue for one feed; updates `last_polled_at`/`last_status` (stores `error: …` on failure but never returns fatal — a bad feed must not break the poll loop).
- `RefreshDue(ctx, olderThan)` — list feeds whose `last_polled_at` is null or older than the interval and `Refresh` each; used by the periodic job.
- Cap entries processed per feed per poll (e.g. 100) and log if truncated.

## Periodic poll job (`internal/jobs`)

- `PollFeedsArgs{}` Kind `poll_feeds`; `PollFeedsWorker{Service *feeds.Service}` → `service.RefreshDue(ctx, 30*time.Minute)`.
- Wire in `NewRiverClient(workersOn=true)`: register the worker + add `cfg.PeriodicJobs` with `river.NewPeriodicJob(river.PeriodicInterval(30*time.Minute), func()(river.JobArgs,*river.InsertOpts){return PollFeedsArgs{},nil}, &river.PeriodicJobOpts{RunOnStart:true})`. The API (insert-only) process registers neither. `NewRiverClient` needs the feed service (or enough to build it) when workersOn — thread it through cmd wiring like the pipeline.

## API (`internal/api/feeds.go`, openapi → regen)

- `POST /feeds {url}` → 201 `Feed` (+ maybe `addedCount`); 409 if already subscribed; 400 bad url; 502 if the feed can't be fetched/parsed (don't persist a broken feed on add).
- `GET /feeds` → `Feed[]` (user-scoped, newest first).
- `DELETE /feeds/{id}` → 204 / 404 (does NOT delete already-imported items — just stops polling).
- Bearer + rate limit as all routes. Schema `Feed{id,url,title,siteUrl,lastPolledAt?,lastStatus,createdAt}`.

## Web (`apps/web`)

- `/feeds` page: add-feed form (URL input → POST via `/api/feeds` proxy, inline result), list of feeds (title, host, last-polled relative time, status pill, delete). Empty state teaches what feeds do.
- Topbar/sidebar: a "Feeds" link near Import/Export. Proxies `/api/feeds` (GET/POST) + `/api/feeds/[id]` (DELETE), cookie→bearer via `apiFetch`. Tokens-only styling.

## Testing

- Go: parser unit tests (RSS 2.0 + Atom fixtures, relative-link resolution, malformed → error, no-link entries skipped). Service/DB tests: Add backfills + dedups + is idempotent on re-add-attempt (409); Refresh saves only new entries + is idempotent (re-run same feed → 0 new); a fetch error sets `last_status` and doesn't throw. Handler tests (201/409/400, list, delete 204/404, cross-tenant 404). All user-scoped.
- Web: build + lint; e2e on the box (add a real feed → items appear; list shows it; delete).

## Out of scope

Per-entry read/unread state, feed folders/OPML import, podcast enclosures, feed favicons, conditional GET (etag/last-modified — nice-to-have follow-up), configurable poll interval.

## Execution

Subagent-driven; reuse `internal/importer` (dedup + item-create pattern) and `internal/jobs` conventions. Deploy api+web after whole-branch review; the periodic job starts running on the box automatically (it runs `all`).
