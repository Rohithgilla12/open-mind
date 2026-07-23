# Reel places Phase 4 — deep media + location tag (design)

Date: 2026-07-23. Detailed implementation design for Phase 4 of the reel
place-extraction feature. Builds on the overview + Phases 1–3 in
`20260716-reel-places-design.md`; that document stays the roadmap of record,
this one is the buildable spec for Phase 4.

## Goal

Extend place extraction beyond the caption + thumbnail rungs with two additions,
both in one implementation cycle:

1. **Deep media** — behind `REEL_MEDIA=video`, download the reel's mp4 with a
   user-installed `yt-dlp`, sample ≤8 frames with `ffmpeg`, and read them in one
   batched vision call. Only self-hosters who install the binaries pay for this;
   it is never a compose requirement.
2. **Location tag** — reels sometimes embed a tagged location in the page's
   inline JSON. Parse it opportunistically (pure Go, no binaries) during the
   existing capture-time fetch; it is a near-certain candidate that outranks
   caption text.

## Decisions (locked in brainstorming)

- **Both sub-features ship together** in this spec.
- **`REEL_MEDIA` = `off | thumbnail | video`, default `thumbnail`.** Default
  `thumbnail` preserves today's behaviour exactly (Phase 2 thumbnail vision runs
  unconditionally now, so a literal default `off` would be a silent regression).
  `off` is an explicit opt-out (caption + location only, no vision model calls);
  `video` adds the frame-escalation rung.
- **Video escalates only when needed** — the frame rung runs only if the cheaper
  rungs (caption + location + thumbnail) produced **zero** places. This honours
  the "cheapest first, stop when confident" ladder and avoids downloading every
  reel. No confidence threshold knob for now (YAGNI).
- **Location tag: capture-time parse + persist (approach Y).** Parsed in the
  pipeline's existing page fetch and persisted on the item, so the job needs no
  second Instagram fetch (the spec's load-bearing risk is anonymous-fetch
  blocking; minimising fetches matters).

## Ladder flow — `ExtractPlacesWorker.Work`

The worker keeps its shape (refetch item → rungs → merge → geocode → atomic
delete+insert). `mode` is the effective `REEL_MEDIA` mode resolved at startup
(see Config); `extractor` is a `*reelmedia.Extractor` or nil.

```
1. caption  = Provider.ExtractPlaces(title, body)              // text rung, always
2. location = candidate from item.TaggedLocation, if set       // location rung, always
3. thumb    = Provider.ExtractPlacesVision(leadImage)           // if mode >= thumbnail
                                                               //   and lead image present
   merged   = MergePlaces(location, caption, thumb)
4. if mode == video && extractor != nil && len(merged) == 0 {  // escalate only when empty
       frames, err = extractor.Frames(ctx, item.Url)           // best-effort (see Errors)
       if err == nil && len(frames) > 0 {
           video  = Provider.ExtractPlacesVisionFrames(title, body, frames)
           merged = MergePlaces(location, caption, thumb, video)
       }
   }
5. geocode each candidate, atomic delete+insert                // unchanged
```

- Rung ordering by `REEL_MEDIA`: `off` skips rungs 3 and 4; `thumbnail` skips 4;
  `video` may run 4.
- **Write guard.** Today the worker only persists when a provider rung ran
  successfully (the `ran` flag). Extend it so the delete+insert also runs when a
  location candidate exists even if every provider returned `ErrNotSupported`
  (noop instance with a tagged reel still gets its location place). Concretely:
  proceed to write when `ran || location != nil`. An empty result set with
  `ran==true` still clears stale rows (idempotent re-run behaviour preserved).

## `internal/reelmedia` package

New package owning all shell-out. It knows nothing about AI or places — a pure
"URL → frame bytes" unit, so it can be understood and tested in isolation.

```go
package reelmedia

// Mode is the resolved REEL_MEDIA ladder rung.
type Mode int
const ( ModeOff Mode = iota; ModeThumbnail; ModeVideo )

func ModeFromEnv() Mode            // parse REEL_MEDIA, default ModeThumbnail

// Extractor holds resolved binary paths + limits. nil means "no deep media".
type Extractor struct { /* ytDLP, ffmpeg paths; maxFrames; maxBytes; timeout */ }

func Detect() (*Extractor, bool)   // exec.LookPath("yt-dlp") && exec.LookPath("ffmpeg")

func (e *Extractor) Frames(ctx context.Context, url string) ([][]byte, error)
```

`Frames`:
1. `os.MkdirTemp` for a per-call working dir; `defer os.RemoveAll`.
2. `yt-dlp` via `exec.CommandContext`: `--no-playlist`, `--max-filesize 50M`,
   an mp4 format selector, a socket timeout, output to the temp dir, the URL.
3. `ffmpeg` samples up to `maxFrames` (8) frames evenly across the clip,
   downscaled to a long edge of ~768px (caps vision tokens), written as JPEGs.
4. Read the JPEG bytes in order and return them.

**Testability seam.** Command execution goes through a tiny `runner` interface
(`run(ctx, name string, args ...string) ([]byte, error)` plus temp-dir hooks) so
tests inject a fake runner and assert the constructed arguments + the
frame-decode path. CI has neither binary, so a real-binary integration test
`t.Skip`s when `yt-dlp`/`ffmpeg` are absent.

## AI provider — batched frames method

`Provider` gains:

```go
ExtractPlacesVisionFrames(ctx context.Context, title, caption string, frames [][]byte) ([]Place, error)
```

- **Gemini** — one `GenerateContent` call: a text prompt part followed by one
  image part per frame (`genai.NewPartFromBytes`), reusing `placesResponseSchema`
  and JSON mode. This is the single batched vision call.
- **openai / noop** — `ErrNotSupported`.
- **fake** — deterministic fixtures (empty frames → `nil,nil`; else a fixed set,
  including one name overlapping caption at higher confidence to exercise merge).
- **chain** — wrapped by `runChain` like the other ops.
- Reuses the existing `extractPlacesVisionInstruction` prompt and
  `sanitisePlaces`.

## Location tag (approach Y)

**Parse (capture time).** Extend the OG extractor in
`internal/enrich/socialvideo.go` so the same fetch that reads `og:*` also scans
the page for a tagged-location string in the inline JSON (opportunistic — absent
on most login-walled pages, which is fine). Add `TaggedLocation string` to the
`Extraction` struct.

**Persist.** Migration `0018_item_tagged_location.sql` adds a nullable
`items.tagged_location text` column. `runSocialVideo` writes it through
`UpdateItemExtraction` (the query + `UpdateItemExtractionParams` gain the field).
Nullable and populated only for social-video items; every other card type leaves
it NULL.

**Consume.** The job reads `item.TaggedLocation`; if non-empty it becomes a
`Place{Name: <tag>, Confidence: 0.98}` in the `location` group. `hint` is left
empty (the tag is already a resolved place name); geocoding treats it like any
other candidate.

## Merge + precedence

Replace `MergePlacesWithSource(caption, vision []Place) []Placed` with a variadic
form (only the job calls it, so it is a clean swap):

```go
type placeGroup struct { places []Place; source string; defaultConf float64 }
func MergePlaces(groups ...placeGroup) []Placed
```

Semantics unchanged from today except for the extra groups: key by
`strings.ToLower(strings.TrimSpace(Name))`, keep the highest-confidence entry,
ties broken by group order (earliest group wins). Apply `defaultConf` when a
candidate's `Confidence == 0`.

Groups are passed in precedence order with default confidences:

| Group     | `source`     | default confidence |
|-----------|--------------|--------------------|
| location  | `"location"` | 0.98               |
| caption   | `"caption"`  | 0.85               |
| video     | `"video"`    | 0.75               |
| thumbnail | `"vision"`   | 0.70               |

Location's high default (and first-group tie-break) makes it outrank caption as
required. Real model-supplied confidences still win when present.

## Config wiring

- `reelmedia.ModeFromEnv()` reads `REEL_MEDIA` (`off|thumbnail|video`, default
  `thumbnail`; unknown value → default + a logged warning).
- In `cmd/openmind/main.go` `run`, next to `geo.FromEnv()`: resolve the mode; if
  it is `ModeVideo`, call `reelmedia.Detect()`. If detection fails (missing
  binary), **log a warning and downgrade the effective mode to `thumbnail`** so
  the worker never attempts a shell-out it cannot complete. Log the resolved
  effective mode + binary presence at startup (mirrors the geocoder log line).
- Thread `(mode, *Extractor)` into `jobs.NewRiverClient` and onto
  `ExtractPlacesWorker` (new fields), exactly as `geocoder` is threaded today.
- Document `REEL_MEDIA` and the `yt-dlp`/`ffmpeg` optional dependency in
  `docs/self-hosting.md` (optional integration, off the compose path).

## Error handling & safety

- **Capture stays sacred; enrichment stays best-effort.** Every new rung is
  non-fatal: `Frames` errors (blocked download, ffmpeg failure, context timeout,
  empty output) are logged and skipped, never failing or retrying the job —
  identical to today's thumbnail-vision handling. The temp dir is always removed
  via `defer`.
- **Bounds.** `--max-filesize` (disk), an `exec.CommandContext` deadline ~60s
  (time), ≤8 frames and ~768px downscale (vision tokens).
- **SSRF boundary.** `yt-dlp` fetches the URL directly rather than through
  `SafeHTTPClient`. The `extract_places` job is only enqueued for
  `IsSocialVideoURL` hosts (Instagram/TikTok), so the input is already
  constrained; the URL is never attacker-chosen beyond that allowlist. Stated
  explicitly so the boundary is a deliberate, reviewed decision.
- **Idempotency.** Re-running the job reproduces the same rows: the mode gating,
  the empty-set escalation trigger, and the merge are all deterministic given
  the same stored item + fixtures; the final delete+insert is atomic.

## Testing

- **`reelmedia`**: fake `runner` → assert `yt-dlp` + `ffmpeg` argument
  construction and the frame-decode path; mode parsing (incl. unknown →
  default); real-binary integration test `t.Skip`s when binaries are absent.
- **Provider** `ExtractPlacesVisionFrames`: fake fixtures; Gemini multi-part
  request shape where feasible.
- **`MergePlaces`**: table tests — location outranks caption, tie-break by group
  order, case-insensitive dedupe, default-confidence application.
- **Job** (DB-backed, fake provider + fake extractor): escalation fires **only**
  when cheaper rungs are empty; mode gating (`off` → no vision/video,
  `thumbnail` → no video, `video` → escalates on empty); location tag persisted
  → candidate written even under the noop provider; idempotency (run twice, same
  rows); geocoder-off → NULL coords; cross-tenant scoping.
- **Location-tag parse**: `httptest` fixtures — tag present, absent, malformed
  JSON, non-HTML.
- **Config**: `REEL_MEDIA` parse + the binary-absent downgrade to `thumbnail`.

## Out of scope

Everything in the overview spec's out-of-scope list still holds. Additionally:
low-confidence (rather than empty-set) escalation triggers; scene-detection frame
sampling (even spacing only); direct short-video upload to Gemini (frames only);
caching downloaded media; YouTube.

## Risks

- **Instagram/TikTok blocking `yt-dlp` downloads** — the same anonymous-access
  risk as the OG fetch, one rung up. Mitigated by: default `thumbnail` (video is
  strictly opt-in), best-effort degrade (a failed download just leaves the
  cheaper rungs' result), and escalation only when cheaper rungs found nothing
  (so blocked downloads cost nothing when the caption already worked).
- **Binary/version drift** (`yt-dlp` breaks often against IG/TikTok changes) —
  self-hoster's responsibility; absence or failure degrades cleanly, and it is
  never on the default path.
- **Vision cost from frames** — capped by ≤8 frames + downscale + one batched
  call + empty-set-only escalation, keeping it on the cheap tier per principle 6.
