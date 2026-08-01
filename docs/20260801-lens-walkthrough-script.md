# Lens walkthrough — video script & storyboard

Target length **~100 seconds**. Worked example: a Lens that collects every x.com/Twitter save.
Voice: calm, first-person-plural, no hype. UK English on screen.

## Before you record

- Seed a library with visible variety — `tools/screenshots/seed.sql` is the existing seed. You need
  at least 6–8 items of card type `tweet` plus a mix of articles/images so the type filter visibly
  *narrows* something.
- Have at least one other Lens already in the sidebar, so "New lens" reads as an addition, not the
  first-run empty state.
- Window at a fixed size, browser chrome hidden, cursor visible. Record at 2× for crisp downscale.
- Have one x.com URL ready in the clipboard for the payoff beat (Scene 6).

## Beats

| # | Time | On screen | Narration (VO) |
|---|------|-----------|----------------|
| 1 | 0:00–0:08 | The Mind (`/`), full grid, slow scroll of two rows. Hold. | "This is everything you've saved. No folders. Nothing filed." |
| 2 | 0:08–0:18 | Cursor to the sidebar; the existing Lens dots catch the eye. Click **+ New lens**. | "A Lens is a saved rule. Not a folder you drag things into — a question the library keeps answering." |
| 3 | 0:18–0:32 | `/lens/new` form. Type the name: **Posts from X**. Pause on the empty Query field, the eight colour swatches, and the greyed **Save lens** button. | "Give it a name. Then tell it what to look for: some text, a colour, or a kind of card." |
| 4 | 0:32–0:44 | Skip Query and Colour — deliberately. Click the **Post** chip in Card types. It fills; the helper line resolves and **Save lens** enables. | "We want one kind of card. Every x.com link you save is already recognised as a post, the moment it's enriched. So the rule is just: posts." |
| 5 | 0:44–0:56 | Click **Save lens** → lands on `/lens/[id]`. Header: terracotta rule, dot, **Posts from X**, and the meta line — `N gatherings · Post · live view`. Grid populates below. | "And there they are. Everything already saved, gathered — with no filing, and no work done up front." |
| 6 | 0:56–1:16 | **The payoff.** Split or cut: save a fresh x.com link (extension popup or share sheet), cut back to the Lens tab, refresh. Count ticks up; the new card is in the grid. Hold on the new card. | "Here's the part that matters. Save something new… and it appears here. You never told it where to go. The Lens is a live view, not a bucket — it's still asking the question." |
| 7 | 1:16–1:30 | Hover the header actions. Set the digest control to **Weekly**, pick a weekday. Then hover **Send digest to Kindle**. | "If you want it to come to you: a weekly digest, or the whole Lens sent to your Kindle." |
| 8 | 1:30–1:40 | Back to `/` and the search bar. Run any search, then click the **◫ Save as lens** chip — it seeds the form with the query, colour, and type chips already filled. Freeze. | "Any search you like can become a Lens. If a question is worth asking twice, save it." |

## On-screen text (lower thirds — one per scene, max)

- Scene 2 — `A Lens is a saved rule`
- Scene 4 — `x.com → recognised as a Post on save`
- Scene 6 — `Live view. Nothing filed.`
- Scene 7 — `Digest · Kindle`
- Scene 8 — `Any search → a Lens`

## The one honest caveat

Scene 6 is the whole video. If the "save something new, watch it appear" beat isn't clean and
obviously unedited, the piece doesn't land — everything else is a filter UI people have seen before.
Do it in one take with a real save if you can.

## Do not show

- **Don't say "tweet" in the narration.** The chip and the lens meta line both read **Post**
  (`apps/web/lib/cards.ts:86` maps card type `tweet` → label `Post`). Only the internal type is
  `tweet`; nothing user-facing says it, and neither should the VO.
- **Use Domains for x.com, not Query.** Put `x.com, twitter.com` in the Domains field (hosts only —
  subdomains match). Optionally add the **Post** type chip. Don't type `x.com` into Query — the
  full-text index covers title, summary, tags, and body, not the URL.
- **Don't claim "every tweet ever".** A domains/types-only rule has no ranking signal, so it reads
  your most recent items (200 scanned) and returns up to 50. Say "everything you've saved" over a
  modest library and it's true; don't put a number on screen.
- Don't linger on unkept feed items unless you're ready to explain the feed river — Lens views
  default to library scope (kept Mind only).

## Reference — the rule being built

```json
POST /lenses
{ "name": "Posts from X", "rule": { "domains": ["x.com", "twitter.com"], "types": ["tweet"] }, "digestSchedule": "0 8 * * 1" }
```

Backing code, if you need to check a claim before recording:

- `apps/api/internal/enrich/classify.go:32` — x.com / twitter.com / mobile.twitter.com → `tweet`
- `apps/api/internal/search/search.go:176` — `RunLensRule`, incl. the types-only fallback
- `apps/api/internal/store/migrations/0006_user_tags.sql:8` — what the FTS vector actually indexes
- `apps/web/components/LensForm.tsx` — the form in Scenes 3–4
- `apps/web/app/lens/[id]/page.tsx` — the header and actions in Scenes 5 and 7
