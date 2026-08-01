# `repo` card type — design

**Date:** 2026-08-01
**Status:** approved, not yet implemented

## Problem

Every code-host save lands as an `article`. A saved repository is not an article: it has no
readable body worth summarising, and it is the single most common kind of link in this library's
actual use. Grouping repos with blog posts makes both harder to find, and makes a
`types: ["article"]` Lens noisy.

## Decision

Add a tenth card type, `repo`, assigned by URL heuristics at enrichment time — the same mechanism
that already produces `video` and `tweet`. No AI is involved in classification
(`enrich/pipeline.go:115` calls `enrich.Classify` directly).

### Scope

`repo` covers **any code host**, not just github.com — mirroring how `video` already spans YouTube
plus a `socialVideoHosts` set. Initial hosts:

```
github.com  gitlab.com  codeberg.org  bitbucket.org
```

`sr.ht` is deliberately excluded: its `~user/repo` path shape needs a different rule, and it can be
added later without redesign. `gist.github.com` is a different host and stays an `article`.

### The path rule

A URL on a code host is a `repo` when its path has **at least two non-empty segments** and its
**first segment is not reserved**.

Sub-pages of a repository stay `repo` — an issue, a pull request, a release, or a blob is still that
project. Splitting them across two card types would feel arbitrary in the grid.

```
github.com/sqlc-dev/sqlc                  → repo
github.com/sqlc-dev/sqlc/pull/42          → repo
github.com/sqlc-dev/sqlc/blob/main/a.png  → repo      (not image — see ordering)
github.com/torvalds                       → article   (profile, 1 segment)
github.com/pricing                        → article   (1 segment)
github.com/features/copilot               → article   (reserved first segment)
github.com                                → article
gitlab.com/group/sub/project              → repo
gitlab.com/explore                        → article
gist.github.com/user/abc123               → article   (host not in set)
```

**Ordering matters.** The code-host branch runs *before* the image-extension branch in `Classify`.
A `.png` under `/blob/` is an HTML page on github.com, not an image; classifying it as `image`
would produce a card whose lead image is a web page. `raw.githubusercontent.com` is a separate host
and is unaffected, so genuine raw-image saves still classify as `image`.

### Reserved first segments

The `≥2 segments` test alone is not sufficient: `github.com/features/copilot` and
`github.com/topics/go` both pass it. The denylist is the set the hosts themselves reserve against
usernames:

```
about       apps         collections  contact    enterprise  explore
features    join         login        marketplace  notifications
orgs        pricing      pulls        readme     security    settings
sponsors    topics       trending     -
```

`-` covers GitLab's `/-/` infix routes. The list is not exhaustive by construction — see
*Known debt*.

## Contract changes

`openapi.yaml` is edited first, then `task generate` regenerates the Go server types and the TS
client. The enum appears **twice**:

- `Item.cardType` (line 689)
- `LensRule.types` (line 809)

Three Go sites hand-mirror the enum and must be updated in the same change:

- `internal/ai/ai.go:73` — the `parseQueryInstruction` prompt lists the types verbatim, so
  "show me repos" parses into a type filter
- `internal/ai/ai.go:211` — `cardTypes`, which sanitises model-proposed filters
- `internal/api/lenses.go:41` — lens rule validation

## Backfill

`internal/store/migrations/0021_repo_card_type.sql` updates existing rows in place:

```sql
UPDATE items SET card_type = 'repo'
WHERE card_type = 'article'
  AND url ~* '^https?://(www\.)?(github\.com|gitlab\.com|codeberg\.org|bitbucket\.org)/[^/?#]+/[^/?#]+'
  AND url !~* '^https?://(www\.)?(github\.com|gitlab\.com|codeberg\.org|bitbucket\.org)/(about|apps|collections|contact|enterprise|explore|features|join|login|marketplace|notifications|orgs|pricing|pulls|readme|security|settings|sponsors|topics|trending|-)(/|$)';
```

Two predicates rather than one regex with a negative lookahead: Postgres ARE supports lookahead,
but the split form is legible in a code review and each half maps to one half of the Go rule.

Scoped to `card_type = 'article'` so it cannot overwrite an `image` or PDF classification. It
touches one column: no refetch, no AI spend, no risk to a hand-corrected title or summary.
Re-queuing enrichment was rejected for exactly that reason — it would refetch every page and re-run
summarise/tag/embed to change one column.

**The rule now exists twice** (Go and SQL regex). This is the migration's main risk, so it ships
with a parity test: one shared fixture table of URLs, asserted to classify identically through
`enrich.Classify` and through the migration's regex. Without it the two drift the first time
someone adds a host.

## Presentation

| File | Change |
|------|--------|
| `apps/web/lib/cards.ts` | `CardKind`, `KNOWN_KINDS`, `typeGradient`, `typeLabel: "Repo"` |
| `apps/mobile/lib/cards.ts` | the same, plus `typeLabelPlural: "Repos"` |
| `apps/mobile/lib/theme.ts` | `CardKind` union + gradient entry |
| `apps/web/components/LensForm.tsx` | `ALL_TYPES` |

No new `ItemCard` renderer. `repo` takes the existing article-shaped branch — hero, serif title,
summary — which is right for a repo with an OG image and a description.

Gradient: `gold → green`, unclaimed by the other nine types and reading as an old terminal.

### Rollout safety

`cardKind()` normalises unknown types to `article` in both clients. An un-updated client — an old
mobile build, the dock — renders `repo` items as articles rather than breaking. **API and clients
can therefore ship in any order**, and self-hosters running a stale web build see no errors.

## Testing

- `classify_test.go` table rows: repo root, repo sub-page, blob-with-image-extension, profile,
  reserved first segment, bare host, nested GitLab group, non-code host, gist.
- Migration parity test over the shared fixture list (above).
- Lens rule validation accepts `repo`; `sanitiseTypes` keeps it and still drops unknowns.
- Pipeline idempotency is already covered by the existing enrichment tests — re-running must
  produce `repo` twice.

## Known debt

The reserved-segment denylist is maintenance. When a host adds a new reserved path, we misclassify
it as a repo until someone notices. The alternative — matching repo roots only — was rejected
because it splits one project's saves across two card types. The denylist is the better trade, but
it is real ongoing debt and should be noted in `TODO.md` under Later.
