# Reel places Phase 4 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two rungs to reel place-extraction — an opportunistic inline-JSON location tag (pure Go) and an optional `yt-dlp`+`ffmpeg` deep-media frame rung behind `REEL_MEDIA=video` — so reels whose caption/thumbnail name no place can still surface one.

**Architecture:** A new isolated `internal/reelmedia` package owns all binary shell-out (`URL → []frame bytes`); a new `Provider.ExtractPlacesVisionFrames` method reads all frames in one batched vision call; the location tag is parsed during the pipeline's existing page fetch and persisted on a new `items.tagged_location` column; the `extract_places` worker orchestrates the ladder, escalating to the video rung only when the cheaper rungs found nothing.

**Tech Stack:** Go 1.x, River jobs, sqlc + pgx, Postgres, `google.golang.org/genai` (Gemini), `os/exec` (new), `yt-dlp` + `ffmpeg` (optional, user-installed).

## Global Constraints

- **Capture is sacred; enrichment is best-effort.** Every new rung must be non-fatal — a failure is logged and skipped, never fails or retries the job.
- **No new required infrastructure.** `yt-dlp`/`ffmpeg` are optional; absence must degrade cleanly. Never a `docker compose` requirement.
- **Cheap models only** (principle 6): one batched vision call, ≤8 frames, downscaled.
- **Every store method is `user_id`-scoped.** New rows/reads stay scoped.
- **No hand-edited generated code** (`internal/store/db/*`, `internal/api/gen.go`). Regenerate via `task generate:sqlc`.
- **No decorative banner comments** (`// ====`).
- **`REEL_MEDIA` = `off | thumbnail | video`, default `thumbnail`.** `off` = caption + location only; `thumbnail` = + thumbnail vision (today's behaviour); `video` = + frame escalation.
- **Merge default confidences:** location `0.98`, caption `0.85`, video `0.75`, thumbnail(vision) `0.70`. `source` labels: `"location"`, `"caption"`, `"video"`, `"vision"`.
- **Escalation trigger:** video rung runs only when `len(merged) == 0` after the cheaper rungs.
- All Go commands run from `apps/api`. Build check: `go build ./...`. Full tests: `go test ./...`. Package tests: `go test ./internal/<pkg>/ -run <Name> -v`.

## File Structure

- Create `internal/store/migrations/0019_item_tagged_location.sql` — adds `items.tagged_location`.
- Modify `internal/store/queries/items.sql` — `UpdateItemExtraction` gains `tagged_location`.
- Regenerate `internal/store/db/*.go` (sqlc).
- Modify `internal/enrich/socialvideo.go` (+ the `Extraction` struct) — parse + persist the location tag.
- Modify `internal/ai/ai.go` — `PlaceGroup` + `MergePlaces`; add `ExtractPlacesVisionFrames` to `Provider`; frames prompt const.
- Modify `internal/ai/{noop,fake,gemini,openai,chain}.go` — implement `ExtractPlacesVisionFrames`.
- Create `internal/reelmedia/reelmedia.go` (+ `reelmedia_test.go`) — mode, binary detection, frame extraction.
- Modify `internal/jobs/places.go` — the ladder + new worker fields.
- Modify `internal/jobs/enrich.go` — `NewRiverClient` threads `(mode, extractor)`.
- Modify `cmd/openmind/main.go` — resolve mode, detect binaries, downgrade, thread.
- Modify `docs/self-hosting.md` — document `REEL_MEDIA` + optional binaries.

No `openapi.yaml` change: `Place.source` is already a free string, so new source values need no schema change.

---

### Task 1: Add `items.tagged_location` column + query

**Files:**
- Create: `internal/store/migrations/0019_item_tagged_location.sql`
- Modify: `internal/store/queries/items.sql` (UpdateItemExtraction)
- Regenerate: `internal/store/db/*.go`

**Interfaces:**
- Produces: `db.Item.TaggedLocation string`; `db.UpdateItemExtractionParams.TaggedLocation string`.

- [ ] **Step 1: Write the migration**

Create `internal/store/migrations/0019_item_tagged_location.sql`:

```sql
-- Tagged location parsed from a social-video page's inline JSON (Phase 4).
-- Empty string = no tag (matches the item_places hint/address convention).
ALTER TABLE items ADD COLUMN tagged_location text NOT NULL DEFAULT '';
```

- [ ] **Step 2: Extend the UpdateItemExtraction query**

In `internal/store/queries/items.sql`, replace the `UpdateItemExtraction` body with:

```sql
-- name: UpdateItemExtraction :exec
UPDATE items SET title = $3, body = $4, lead_image_url = $5, card_type = $6, tagged_location = $7, updated_at = now()
WHERE user_id = $1 AND id = $2;
```

(`GetItem` is `SELECT *`, so it picks up the new column automatically — no change.)

- [ ] **Step 3: Regenerate sqlc**

Run (from repo root): `task generate:sqlc`
Expected: `internal/store/db/items.sql.go` regenerates; `git diff` shows `TaggedLocation string` added to both the `Item` struct and `UpdateItemExtractionParams`.

- [ ] **Step 4: Verify it compiles**

Note: `UpdateItemExtraction` now needs a `TaggedLocation` param at every call site. The only caller (`runSocialVideo`) is updated in Task 2, so a build here will fail on that call site.
Run: `cd apps/api && go build ./internal/store/...`
Expected: `internal/store/...` builds (the store package itself compiles). A full `go build ./...` will fail until Task 2 — that is expected.

- [ ] **Step 5: Commit**

```bash
git add apps/api/internal/store/migrations/0019_item_tagged_location.sql apps/api/internal/store/queries/items.sql apps/api/internal/store/db/
git commit -m "feat(places): add items.tagged_location column"
```

---

### Task 2: Parse + persist the inline-JSON location tag

**Files:**
- Modify: `internal/enrich/socialvideo.go` (+ the `Extraction` struct — search `type Extraction struct`; it already has `Title`, `Body`, `LeadImageURL`)
- Test: `internal/enrich/socialvideo_test.go`

**Interfaces:**
- Consumes: `db.UpdateItemExtractionParams.TaggedLocation` (Task 1).
- Produces: `Extraction.TaggedLocation string`; `parseTaggedLocation(raw []byte) string`.

- [ ] **Step 1: Write the failing test**

Add to `internal/enrich/socialvideo_test.go`:

```go
func TestParseTaggedLocation(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{"present", `<script>{"location":{"id":"123","name":"Blue Bottle Coffee"}}</script>`, "Blue Bottle Coffee"},
		{"unicode escapes", `{"location":{"name":"Café Lisboa"}}`, "Café Lisboa"},
		{"absent", `<html><head><meta property="og:title" content="x"></head></html>`, ""},
		{"malformed no name", `{"location":{"id":"9"}}`, ""},
		{"empty", ``, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseTaggedLocation([]byte(tt.html)); got != tt.want {
				t.Fatalf("parseTaggedLocation() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/api && go test ./internal/enrich/ -run TestParseTaggedLocation -v`
Expected: FAIL — `undefined: parseTaggedLocation`.

- [ ] **Step 3: Add the field + parser**

In `internal/enrich/socialvideo.go`: add `"bytes"`, `"regexp"`, and `"strconv"` to imports. Add the field `TaggedLocation string` to the `Extraction` struct. Add:

```go
// locationNameRe matches an inline-JSON tagged location, e.g. Instagram's
// GraphQL blob `"location":{"id":"…","name":"Blue Bottle Coffee"}`. Best-effort
// and opportunistic: most login-walled pages omit it, which is fine.
var locationNameRe = regexp.MustCompile(`"location":\{[^{}]*?"name":"([^"]{1,200})"`)

// parseTaggedLocation returns the first inline-JSON tagged-location name in the
// page, JSON-unescaped, or "" when none is present.
func parseTaggedLocation(raw []byte) string {
	m := locationNameRe.FindSubmatch(raw)
	if m == nil {
		return ""
	}
	name := string(m[1])
	// The capture is a raw JSON string body; unescape \uXXXX and friends.
	if unq, err := strconv.Unquote(`"` + name + `"`); err == nil {
		name = unq
	}
	return strings.TrimSpace(name)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/api && go test ./internal/enrich/ -run TestParseTaggedLocation -v`
Expected: PASS.

- [ ] **Step 5: Read the raw body once and populate the field**

In `extractOpenGraph`, replace the streaming parse with a read-once so both the OG tokeniser and the location regex see the same bytes:

```go
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return Extraction{}, fmt.Errorf("reading %s: %w", rawURL, err)
	}
	og, err := parseOpenGraph(bytes.NewReader(raw))
	if err != nil {
		return Extraction{}, fmt.Errorf("parsing %s: %w", rawURL, err)
	}
	og.TaggedLocation = parseTaggedLocation(raw)
	return og, nil
```

- [ ] **Step 6: Persist it in runSocialVideo**

In `runSocialVideo`, add `TaggedLocation: ex.TaggedLocation` to the `db.UpdateItemExtractionParams` literal (alongside `CardType: "video"`). This also fixes the Task 1 build break.

- [ ] **Step 7: Build + full enrich tests**

Run: `cd apps/api && go build ./... && go test ./internal/enrich/ -v`
Expected: build passes; all enrich tests PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/api/internal/enrich/socialvideo.go apps/api/internal/enrich/socialvideo_test.go
git commit -m "feat(places): parse + persist inline-JSON tagged location"
```

---

### Task 3: Generalise the place merge

**Files:**
- Modify: `internal/ai/ai.go` (replace `MergePlacesWithSource`)
- Modify: `internal/jobs/places.go:111` (call site)
- Test: `internal/ai/ai_test.go` (replace the `MergePlacesWithSource` test)

**Interfaces:**
- Produces: `ai.PlaceGroup{Places []Place; Source string; DefaultConf float64}`; `ai.MergePlaces(groups ...PlaceGroup) []Placed`.

- [ ] **Step 1: Write the failing test**

In `internal/ai/ai_test.go`, remove the existing `MergePlacesWithSource` test and add:

```go
func TestMergePlaces(t *testing.T) {
	loc := []Place{{Name: "Fabrica"}}                       // default 0.98
	caption := []Place{{Name: "Fabrica"}, {Name: "Copenhagen Coffee Lab"}} // default 0.85
	vision := []Place{{Name: "Fabrica", Confidence: 0.9}}   // explicit 0.9

	got := MergePlaces(
		PlaceGroup{Places: loc, Source: "location", DefaultConf: 0.98},
		PlaceGroup{Places: caption, Source: "caption", DefaultConf: 0.85},
		PlaceGroup{Places: vision, Source: "vision", DefaultConf: 0.70},
	)

	if len(got) != 2 {
		t.Fatalf("want 2 merged places, got %d: %+v", len(got), got)
	}
	// Fabrica: location 0.98 beats caption 0.85 and vision 0.9 → source "location".
	if got[0].Name != "Fabrica" || got[0].Source != "location" {
		t.Errorf("Fabrica winner = %+v, want location", got[0])
	}
	// First-seen order preserved (location added Fabrica first).
	if got[1].Name != "Copenhagen Coffee Lab" || got[1].Source != "caption" {
		t.Errorf("second = %+v, want Copenhagen Coffee Lab/caption", got[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/api && go test ./internal/ai/ -run TestMergePlaces -v`
Expected: FAIL — `undefined: PlaceGroup` / `MergePlaces`.

- [ ] **Step 3: Replace MergePlacesWithSource with MergePlaces**

In `internal/ai/ai.go`, delete `MergePlacesWithSource` and add:

```go
// PlaceGroup is one source's candidates plus the source label and the
// confidence to assume when a candidate carries none (0).
type PlaceGroup struct {
	Places      []Place
	Source      string
	DefaultConf float64
}

// MergePlaces combines candidate groups by normalised name, keeping the
// highest-confidence candidate and its source. Ties are broken by group order
// (earlier group wins), so callers pass groups in precedence order. First-seen
// name order is preserved in the result.
func MergePlaces(groups ...PlaceGroup) []Placed {
	byName := make(map[string]Placed)
	order := make([]string, 0)
	for _, g := range groups {
		for _, p := range g.Places {
			if p.Confidence == 0 {
				p.Confidence = g.DefaultConf
			}
			key := strings.ToLower(strings.TrimSpace(p.Name))
			if key == "" {
				continue
			}
			cur, ok := byName[key]
			if !ok {
				byName[key] = Placed{Place: p, Source: g.Source}
				order = append(order, key)
				continue
			}
			if p.Confidence > cur.Confidence {
				byName[key] = Placed{Place: p, Source: g.Source}
			}
		}
	}
	out := make([]Placed, 0, len(order))
	for _, key := range order {
		out = append(out, byName[key])
	}
	return out
}
```

Update the `Placed.Source` doc comment to read `// "location", "caption", "video", or "vision"`.

- [ ] **Step 4: Update the job call site (no behaviour change yet)**

In `internal/jobs/places.go`, replace line ~111:

```go
	merged := ai.MergePlaces(
		ai.PlaceGroup{Places: captionPlaces, Source: "caption", DefaultConf: 0.85},
		ai.PlaceGroup{Places: visionPlaces, Source: "vision", DefaultConf: 0.70},
	)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd apps/api && go build ./... && go test ./internal/ai/ ./internal/jobs/ -v`
Expected: PASS (job behaviour unchanged; merge tests green).

- [ ] **Step 6: Commit**

```bash
git add apps/api/internal/ai/ai.go apps/api/internal/ai/ai_test.go apps/api/internal/jobs/places.go
git commit -m "refactor(places): generalise place merge to variadic groups"
```

---

### Task 4: Add `ExtractPlacesVisionFrames` provider method

**Files:**
- Modify: `internal/ai/ai.go` (interface + prompt const), `internal/ai/chain.go`, `internal/ai/noop.go`, `internal/ai/openai.go`, `internal/ai/fake.go`, `internal/ai/gemini.go`
- Test: `internal/ai/fake_test.go` (or the existing provider test file) + `internal/ai/chain_test.go`

**Interfaces:**
- Produces: `Provider.ExtractPlacesVisionFrames(ctx context.Context, title, caption string, frames [][]byte) ([]Place, error)` on every provider and `*Chain`.

- [ ] **Step 1: Write the failing test (fake provider)**

Add to the fake provider test file:

```go
func TestFakeExtractPlacesVisionFrames(t *testing.T) {
	f := NewFake() // use the existing fake constructor if named differently
	if got, err := f.ExtractPlacesVisionFrames(context.Background(), "t", "c", nil); err != nil || got != nil {
		t.Fatalf("empty frames: got (%v, %v), want (nil, nil)", got, err)
	}
	got, err := f.ExtractPlacesVisionFrames(context.Background(), "t", "c", [][]byte{{0x1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("want fixture places for non-empty frames, got none")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd apps/api && go test ./internal/ai/ -run TestFakeExtractPlacesVisionFrames -v`
Expected: FAIL — method not on the interface / fake.

- [ ] **Step 3: Add the interface method + prompt const**

In `internal/ai/ai.go`, add to the `Provider` interface after `ExtractPlacesVision`:

```go
	// ExtractPlacesVisionFrames returns places grounded in on-screen text
	// across several sampled video frames, read in one batched call. Text-only
	// providers return ErrNotSupported; empty frames yield an empty list.
	ExtractPlacesVisionFrames(ctx context.Context, title, caption string, frames [][]byte) ([]Place, error)
```

Add a prompt const near the other place prompts:

```go
const extractPlacesFramesInstruction = `You extract real-world, visitable places from several frames sampled from one social-media video. ` +
	`Read on-screen text overlays and any place names visible across the frames; use the optional title/caption only to disambiguate a name you can already see. ` +
	`Return only specific named places a person could visit (cafes, restaurants, bars, hotels, shops, landmarks, parks, museums). ` +
	`Never invent places from vibes, cuisine cues, or scenery alone; if no place name is readable, return an empty list. ` +
	`Deduplicate places that appear in multiple frames. ` +
	`For each place set "hint" to any city/area/country visible (or "" if none), and "confidence" to a number from 0 to 1. ` +
	`Respond with only a JSON object of the form {"places": [{"name": string, "hint": string, "confidence": number}]}.`
```

- [ ] **Step 4: Implement on noop and openai (unsupported)**

In `internal/ai/noop.go`:

```go
func (Noop) ExtractPlacesVisionFrames(ctx context.Context, title, caption string, frames [][]byte) ([]Place, error) {
	return nil, ErrNotSupported
}
```

In `internal/ai/openai.go` (text-only, mirrors its `ExtractPlacesVision`):

```go
func (p *OpenAI) ExtractPlacesVisionFrames(ctx context.Context, title, caption string, frames [][]byte) ([]Place, error) {
	return nil, ErrNotSupported
}
```

(Match the exact receiver names/types used by each file's existing `ExtractPlacesVision`.)

- [ ] **Step 5: Implement on fake**

In `internal/ai/fake.go`, mirror `ExtractPlacesVision`:

```go
func (f *Fake) ExtractPlacesVisionFrames(ctx context.Context, title, caption string, frames [][]byte) ([]Place, error) {
	if len(frames) == 0 {
		return nil, nil
	}
	return []Place{
		{Name: "Frame Diner", Hint: "Faketown", Confidence: 0.8},
		{Name: "Vision Landmark", Hint: "Faketown", Confidence: 0.7},
	}, nil
}
```

(Use the exact receiver type of the other `Fake` methods.)

- [ ] **Step 6: Implement on gemini (batched multi-part)**

In `internal/ai/gemini.go`, mirror `ExtractPlacesVision` (at ~:167) but append one image part per frame. Use the existing config/schema helper that `ExtractPlaces`/`ExtractPlacesVision` use:

```go
func (g *Gemini) ExtractPlacesVisionFrames(ctx context.Context, title, caption string, frames [][]byte) ([]Place, error) {
	parts := []*genai.Part{genai.NewPartFromText(placesUserPrompt(extractPlacesFramesInstruction, title, caption))}
	for _, f := range frames {
		if len(f) == 0 {
			continue
		}
		parts = append(parts, genai.NewPartFromBytes(f, httpDetectImageMIME(f)))
	}
	if len(parts) == 1 { // no usable frames
		return nil, nil
	}
	contents := []*genai.Content{genai.NewContentFromParts(parts, genai.RoleUser)}
	cfg := &genai.GenerateContentConfig{ResponseMIMEType: "application/json", ResponseSchema: placesResponseSchema()}
	resp, err := g.client.Models.GenerateContent(ctx, geminiGenModel, contents, cfg)
	if err != nil {
		return nil, classifyGeminiErr(err)
	}
	return parsePlacesResponse(resp) // reuse the same response-parsing helper as ExtractPlacesVision
}
```

Match the exact prompt-building and response-parsing helpers this file already uses for `ExtractPlacesVision` (names like `placesUserPrompt`/`parsePlacesResponse` are illustrative — use whatever `ExtractPlacesVision` calls; read it first and mirror it exactly, only changing the instruction const and adding the per-frame parts loop).

- [ ] **Step 7: Implement on the chain**

In `internal/ai/chain.go`, mirror `ExtractPlacesVision`:

```go
func (c *Chain) ExtractPlacesVisionFrames(ctx context.Context, title, caption string, frames [][]byte) ([]Place, error) {
	return runChain(ctx, c, "ExtractPlacesVisionFrames", func(p Provider) ([]Place, error) {
		return p.ExtractPlacesVisionFrames(ctx, title, caption, frames)
	})
}
```

- [ ] **Step 8: Add a chain test**

Add to `internal/ai/chain_test.go` a case asserting the chain falls over from a noop (`ErrNotSupported`) to the fake and returns the fake's fixtures for non-empty frames. Follow the existing `ExtractPlacesVision` chain test shape.

- [ ] **Step 9: Run tests + build**

Run: `cd apps/api && go build ./... && go test ./internal/ai/ -v`
Expected: build passes (interface satisfied by all providers); tests PASS.

- [ ] **Step 10: Commit**

```bash
git add apps/api/internal/ai/
git commit -m "feat(places): add batched ExtractPlacesVisionFrames provider method"
```

---

### Task 5: `internal/reelmedia` package

**Files:**
- Create: `internal/reelmedia/reelmedia.go`
- Test: `internal/reelmedia/reelmedia_test.go`

**Interfaces:**
- Produces: `reelmedia.Mode` (`ModeOff`, `ModeThumbnail`, `ModeVideo`); `ModeFromEnv() Mode`; `Detect() (*Extractor, bool)`; `(*Extractor).Frames(ctx, url string) ([][]byte, error)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/reelmedia/reelmedia_test.go`:

```go
package reelmedia

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModeFromEnv(t *testing.T) {
	cases := map[string]Mode{"": ModeThumbnail, "thumbnail": ModeThumbnail, "off": ModeOff, "video": ModeVideo, "bogus": ModeThumbnail}
	for in, want := range cases {
		t.Setenv("REEL_MEDIA", in)
		if got := ModeFromEnv(); got != want {
			t.Errorf("ModeFromEnv(%q) = %v, want %v", in, got, want)
		}
	}
}

// fakeRunner records args and writes the output files each stage expects.
type fakeRunner struct{ calls [][]string }

func (r *fakeRunner) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	switch {
	case strings.Contains(name, "yt-dlp"):
		// last arg is the URL; the -o value is the output path.
		out := argValue(args, "-o")
		_ = os.WriteFile(out, []byte("fakevideo"), 0o600)
	case strings.Contains(name, "ffprobe"):
		return []byte("12.5\n"), nil
	case strings.Contains(name, "ffmpeg"):
		// output pattern is the last arg, e.g. /dir/frame_%03d.jpg
		pattern := args[len(args)-1]
		for i := 1; i <= 3; i++ {
			_ = os.WriteFile(strings.Replace(pattern, "%03d", pad3(i), 1), []byte{0xFF, 0xD8, byte(i)}, 0o600)
		}
	}
	return nil, nil
}

func TestFramesHappyPath(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRunner{}
	e := &Extractor{ytDLP: "yt-dlp", ffmpeg: "ffmpeg", ffprobe: "ffprobe", maxFrames: 8, run: r.run, tempBase: dir}
	frames, err := e.Frames(context.Background(), "https://instagram.com/reel/abc")
	if err != nil {
		t.Fatalf("Frames: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("want 3 frames, got %d", len(frames))
	}
	// yt-dlp got the URL and a size cap; ffmpeg got an fps filter.
	joined := ""
	for _, c := range r.calls {
		joined += strings.Join(c, " ") + "\n"
	}
	if !strings.Contains(joined, "--max-filesize") || !strings.Contains(joined, "instagram.com/reel/abc") {
		t.Errorf("yt-dlp args missing cap/url:\n%s", joined)
	}
	if !strings.Contains(joined, "fps=") {
		t.Errorf("ffmpeg args missing fps filter:\n%s", joined)
	}
	// Temp working dir is cleaned up.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("temp dir not cleaned: %v", entries)
	}
	_ = filepath.Join // keep import if unused after edits
}
```

Add tiny helpers `argValue` and `pad3` to the test file (or inline them):

```go
func argValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
func pad3(i int) string { return string(rune('0'+i/100%10)) + string(rune('0'+i/10%10)) + string(rune('0'+i%10)) }
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd apps/api && go test ./internal/reelmedia/ -v`
Expected: FAIL — package/types not defined.

- [ ] **Step 3: Implement the package**

Create `internal/reelmedia/reelmedia.go`:

```go
// Package reelmedia is the optional deep-media rung for reel place extraction:
// it downloads a social video with yt-dlp and samples frames with ffmpeg,
// returning JPEG bytes. It knows nothing about AI or places. All binaries are
// user-installed and optional; absence is handled by the caller (Detect).
package reelmedia

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Mode is the resolved REEL_MEDIA ladder ceiling.
type Mode int

const (
	ModeOff Mode = iota
	ModeThumbnail
	ModeVideo
)

// ModeFromEnv reads REEL_MEDIA (off|thumbnail|video), defaulting to thumbnail
// (preserving Phase 2 behaviour). Unknown values fall back to thumbnail.
func ModeFromEnv() Mode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("REEL_MEDIA"))) {
	case "off":
		return ModeOff
	case "video":
		return ModeVideo
	default:
		return ModeThumbnail
	}
}

const (
	maxFramesDefault = 8
	maxFileSize      = "50M"
	frameLongEdge    = 768
	extractTimeout   = 60 * time.Second
)

type runFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRun(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Extractor holds resolved binary paths and limits. Construct via Detect.
type Extractor struct {
	ytDLP, ffmpeg, ffprobe string
	maxFrames              int
	run                    runFunc
	tempBase               string // "" = os.TempDir(); overridden in tests
}

// Detect resolves yt-dlp, ffmpeg, and ffprobe on PATH. ok=false (nil Extractor)
// means deep media is unavailable and the caller should downgrade to thumbnail.
func Detect() (*Extractor, bool) {
	yt, err1 := exec.LookPath("yt-dlp")
	ff, err2 := exec.LookPath("ffmpeg")
	fp, err3 := exec.LookPath("ffprobe")
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, false
	}
	return &Extractor{ytDLP: yt, ffmpeg: ff, ffprobe: fp, maxFrames: maxFramesDefault, run: execRun}, true
}

// Frames downloads the video at url and returns up to maxFrames JPEG frames
// sampled evenly across it. Best-effort: any failure returns an error the
// caller logs and ignores. The working dir is always removed.
func (e *Extractor) Frames(ctx context.Context, url string) (frames [][]byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, extractTimeout)
	defer cancel()

	dir, err := os.MkdirTemp(e.tempBase, "reelmedia-*")
	if err != nil {
		return nil, fmt.Errorf("temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	video := filepath.Join(dir, "v.mp4")
	if _, err := e.run(ctx, e.ytDLP,
		"--no-playlist", "--no-warnings", "--max-filesize", maxFileSize,
		"--socket-timeout", "20", "-f", "mp4/best[ext=mp4]/best",
		"-o", video, url,
	); err != nil {
		return nil, fmt.Errorf("yt-dlp: %w", err)
	}
	if _, statErr := os.Stat(video); statErr != nil {
		return nil, fmt.Errorf("yt-dlp produced no file: %w", statErr)
	}

	dur := e.probeDuration(ctx, video) // seconds; 0 on failure
	fps := "1"
	if dur > 0 {
		fps = strconv.FormatFloat(float64(e.maxFrames)/dur, 'f', 4, 64)
	}
	pattern := filepath.Join(dir, "frame_%03d.jpg")
	if _, err := e.run(ctx, e.ffmpeg,
		"-hide_banner", "-loglevel", "error", "-i", video,
		"-vf", fmt.Sprintf("fps=%s,scale=%d:-1:force_original_aspect_ratio=decrease", fps, frameLongEdge),
		"-frames:v", strconv.Itoa(e.maxFrames), "-q:v", "4", pattern,
	); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "frame_*.jpg"))
	sort.Strings(matches)
	for _, m := range matches {
		b, rerr := os.ReadFile(m)
		if rerr == nil && len(b) > 0 {
			frames = append(frames, b)
		}
	}
	return frames, nil
}

// probeDuration returns the clip length in seconds, or 0 if ffprobe fails.
func (e *Extractor) probeDuration(ctx context.Context, video string) float64 {
	out, err := e.run(ctx, e.ffprobe,
		"-v", "error", "-show_entries", "format=duration",
		"-of", "default=nw=1:nk=1", video,
	)
	if err != nil {
		return 0
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/api && go test ./internal/reelmedia/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/api/internal/reelmedia/
git commit -m "feat(places): reelmedia package (yt-dlp + ffmpeg frame sampling)"
```

---

### Task 6: Wire the ladder into the extract_places worker

**Files:**
- Modify: `internal/jobs/places.go`
- Test: `internal/jobs/places_test.go` (extend existing DB-backed tests)

**Interfaces:**
- Consumes: `reelmedia.Mode`, `(*reelmedia.Extractor).Frames`, `ai.MergePlaces`, `ai.Provider.ExtractPlacesVisionFrames`, `db.Item.TaggedLocation`.
- Produces: `ExtractPlacesWorker.Mode reelmedia.Mode`, `ExtractPlacesWorker.Extractor *reelmedia.Extractor`.

- [ ] **Step 1: Write failing tests**

Extend `internal/jobs/places_test.go` (mirror the existing DB-backed test setup — fake provider + real Postgres). Add cases:

```go
// A tagged location is stored even when the provider is noop.
func TestExtractPlaces_LocationTagUnderNoop(t *testing.T) { /* set item.tagged_location via UpdateItemExtraction; Provider = noop; expect one item_places row Name=<tag> Source="location" */ }

// video mode escalates to frames only when cheaper rungs are empty.
func TestExtractPlaces_VideoEscalatesWhenEmpty(t *testing.T) { /* Provider = fake returning no caption/vision places but frame places; Extractor = stub returning 1 frame; Mode = ModeVideo; expect Source="video" rows */ }

// video mode does NOT escalate when caption already found a place.
func TestExtractPlaces_NoEscalationWhenCaptionHit(t *testing.T) { /* fake returns a caption place; stub Extractor records zero Frames calls */ }

// thumbnail mode never calls Frames.
func TestExtractPlaces_ThumbnailModeSkipsVideo(t *testing.T) { /* Mode = ModeThumbnail; stub Extractor.Frames must not be called */ }
```

Use a tiny stub extractor via an interface (see Step 3). Assert idempotency by running `Work` twice and comparing `ListItemPlaces`.

- [ ] **Step 2: Run to verify they fail**

Run: `cd apps/api && go test ./internal/jobs/ -run TestExtractPlaces -v`
Expected: FAIL — new fields/interface not present.

- [ ] **Step 3: Add a framer seam + worker fields**

In `internal/jobs/places.go`, add imports for `reelmedia`. Define a narrow interface so tests can stub the extractor:

```go
// framer samples video frames for the deep-media rung; *reelmedia.Extractor
// satisfies it. nil means the rung is unavailable.
type framer interface {
	Frames(ctx context.Context, url string) ([][]byte, error)
}
```

Extend the worker:

```go
	// Mode is the resolved REEL_MEDIA ceiling. Thumbnail (default) runs caption
	// + thumbnail vision; Off is caption + location only; Video adds frames.
	Mode reelmedia.Mode
	// Extractor runs the deep-media rung. Nil (or Mode != Video) disables it.
	Extractor framer
```

- [ ] **Step 4: Implement the ladder**

Rework `Work` after the item load. Replace the caption/vision/merge block with:

```go
	loc := strings.TrimSpace(item.TaggedLocation)
	var locationPlaces, captionPlaces, visionPlaces, videoPlaces []ai.Place
	if loc != "" {
		locationPlaces = []ai.Place{{Name: loc, Confidence: 0.98}}
	}
	ran := false

	if hasText {
		places, err := w.Provider.ExtractPlaces(ctx, item.Title, item.Body)
		switch {
		case errors.Is(err, ai.ErrNotSupported):
		case err != nil:
			return fmt.Errorf("extracting places: %w", err)
		default:
			captionPlaces, ran = places, true
		}
	}

	if hasImage && w.Mode >= reelmedia.ModeThumbnail {
		if data, _ := fetchLeadImage(ctx, w.HTTPClient, item.LeadImageUrl); len(data) > 0 {
			places, err := w.Provider.ExtractPlacesVision(ctx, item.Title, item.Body, data)
			switch {
			case errors.Is(err, ai.ErrNotSupported):
			case err != nil:
				slog.Warn("vision place extraction failed, keeping caption results", "item_id", item.ID, "err", err)
			default:
				visionPlaces, ran = places, true
			}
		}
	}

	merged := ai.MergePlaces(
		ai.PlaceGroup{Places: locationPlaces, Source: "location", DefaultConf: 0.98},
		ai.PlaceGroup{Places: captionPlaces, Source: "caption", DefaultConf: 0.85},
		ai.PlaceGroup{Places: visionPlaces, Source: "vision", DefaultConf: 0.70},
	)

	// Deep-media rung: escalate only when the cheaper rungs found nothing.
	if w.Mode == reelmedia.ModeVideo && w.Extractor != nil && len(merged) == 0 {
		frames, ferr := w.Extractor.Frames(ctx, item.Url)
		switch {
		case ferr != nil:
			slog.Warn("reel frame extraction failed", "item_id", item.ID, "err", ferr)
		case len(frames) > 0:
			places, verr := w.Provider.ExtractPlacesVisionFrames(ctx, item.Title, item.Body, frames)
			switch {
			case errors.Is(verr, ai.ErrNotSupported):
			case verr != nil:
				slog.Warn("frame place extraction failed", "item_id", item.ID, "err", verr)
			default:
				videoPlaces, ran = places, true
				merged = ai.MergePlaces(
					ai.PlaceGroup{Places: locationPlaces, Source: "location", DefaultConf: 0.98},
					ai.PlaceGroup{Places: captionPlaces, Source: "caption", DefaultConf: 0.85},
					ai.PlaceGroup{Places: videoPlaces, Source: "video", DefaultConf: 0.75},
					ai.PlaceGroup{Places: visionPlaces, Source: "vision", DefaultConf: 0.70},
				)
			}
		}
	}

	// Write when any provider rung ran, or when a location tag exists (so a noop
	// instance still records the tagged place). Nothing at all → leave rows.
	if !ran && len(locationPlaces) == 0 {
		return nil
	}
```

The subsequent `rows := ...` build + geocode + atomic delete/insert stays unchanged (it already iterates `merged`). Update the `hasText`/`hasImage` early-return guard at the top to also stay if only a location tag exists — but note the location tag is only set when the item had text/image extraction earlier, so the existing `if !hasText && !hasImage { return nil }` is safe to keep (a social-video item always has at least a degraded title). Leave it as-is.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd apps/api && go build ./... && go test ./internal/jobs/ -run TestExtractPlaces -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/api/internal/jobs/places.go apps/api/internal/jobs/places_test.go
git commit -m "feat(places): ladder with location tag + video-frame escalation"
```

---

### Task 7: Config wiring + docs

**Files:**
- Modify: `internal/jobs/enrich.go` (`NewRiverClient` signature + worker registration)
- Modify: `cmd/openmind/main.go` (resolve mode, detect, downgrade, thread; update all `NewRiverClient` call sites)
- Modify: `docs/self-hosting.md`

**Interfaces:**
- Consumes: `reelmedia.ModeFromEnv`, `reelmedia.Detect`, `ExtractPlacesWorker.{Mode,Extractor}`.

- [ ] **Step 1: Thread mode + extractor through NewRiverClient**

In `internal/jobs/enrich.go`, add `reelmedia` import. Change the signature to accept the mode and extractor after `geocoder`:

```go
func NewRiverClient(pool *pgxpool.Pool, p *enrich.Pipeline, feedService FeedRefresher, kindleDeps KindleDeps, geocoder geo.Geocoder, reelMode reelmedia.Mode, reelExtractor *reelmedia.Extractor, workersOn bool) (*river.Client[pgx.Tx], error) {
```

Update the worker registration (line ~97):

```go
		river.AddWorker(workers, &ExtractPlacesWorker{Store: p.Store, Provider: p.AI, Geocoder: geocoder, Mode: reelMode, Extractor: reelExtractor})
```

Note: `Extractor` is the `framer` interface; passing a nil `*reelmedia.Extractor` must present as a nil interface. Guard at the call site (Step 2) by passing a literal `nil` for the field when detection failed — assign `var ext *reelmedia.Extractor` and only set it when detected, then pass `ext`; in the worker, compare `w.Extractor != nil`. To avoid a typed-nil-in-interface pitfall, in `NewRiverClient` set the field conditionally:

```go
		epw := &ExtractPlacesWorker{Store: p.Store, Provider: p.AI, Geocoder: geocoder, Mode: reelMode}
		if reelExtractor != nil {
			epw.Extractor = reelExtractor
		}
		river.AddWorker(workers, epw)
```

- [ ] **Step 2: Resolve + detect + downgrade in main.go**

In `cmd/openmind/main.go`, near `geocoder, err := geo.FromEnv()`, add:

```go
	reelMode := reelmedia.ModeFromEnv()
	var reelExtractor *reelmedia.Extractor
	if reelMode == reelmedia.ModeVideo {
		ext, ok := reelmedia.Detect()
		if !ok {
			slog.Warn("REEL_MEDIA=video but yt-dlp/ffmpeg not found on PATH; downgrading to thumbnail")
			reelMode = reelmedia.ModeThumbnail
		} else {
			reelExtractor = ext
		}
	}
	slog.Info("reel media", "mode", reelMode, "deep_media", reelExtractor != nil)
```

Add the `reelmedia` import. Update **every** `jobs.NewRiverClient(...)` call site (serve/work/all/mcp — search `NewRiverClient(`) to pass `reelMode, reelExtractor` after the `geocoder` argument.

- [ ] **Step 3: Build + full test suite**

Run: `cd apps/api && go build ./... && go test ./...`
Expected: build passes; all tests PASS (store/job tests need Postgres — run via `task test` or the compose DB per repo convention).

- [ ] **Step 4: Document REEL_MEDIA**

In `docs/self-hosting.md`, in the place-extraction / geocoding area, add a short subsection:

```markdown
### Reel deep media (optional)

Place extraction reads a reel's caption, an embedded tagged location, and (by
default) its thumbnail. Set `REEL_MEDIA` to control the vision ladder:

| `REEL_MEDIA` | Behaviour |
|---|---|
| `off` | Caption + tagged location only — no vision model calls. |
| `thumbnail` (default) | Adds thumbnail vision (on-screen text). |
| `video` | Adds a deep-media rung: when caption/thumbnail name no place, download the clip and read sampled frames. |

`video` requires user-installed **`yt-dlp`** and **`ffmpeg`** on `PATH`; if either
is missing the server logs a warning and behaves as `thumbnail`. These binaries
are never bundled and never a `docker compose` requirement.
```

- [ ] **Step 5: Commit**

```bash
git add apps/api/internal/jobs/enrich.go apps/api/cmd/openmind/main.go docs/self-hosting.md
git commit -m "feat(places): REEL_MEDIA config wiring + self-hosting docs"
```

---

## Self-Review

**Spec coverage:**
- Deep media (yt-dlp+ffmpeg, ≤8 frames, batched call) → Tasks 4, 5, 6. ✓
- Location tag (parse + persist, outranks caption) → Tasks 1, 2, 6 (0.98 default). ✓
- `REEL_MEDIA` off/thumbnail/video, default thumbnail, startup detect + downgrade → Tasks 5, 7. ✓
- Escalate only when cheaper rungs empty → Task 6 (`len(merged) == 0`). ✓
- Generalised merge + precedence/confidence table → Task 3 + Task 6 group order. ✓
- Best-effort/never-block + temp cleanup + bounds + SSRF boundary → Task 5 (timeout, max-filesize, cleanup) + Task 6 (all rungs logged-and-skipped). ✓
- No openapi change (source is free string) → noted; no task needed. ✓
- Testing matrix (reelmedia fake runner, provider frames, MergePlaces, job DB tests, location parse, config) → Tasks 2–7. ✓

**Placeholder scan:** Gemini Step 6 names helpers illustratively (`placesUserPrompt`/`parsePlacesResponse`) with an explicit instruction to mirror the file's existing `ExtractPlacesVision` — acceptable because the exact helpers exist in that file and the implementer reads it first. Job Step 4 test bodies are described (not full code) but each states exact setup + assertion; expand inline when implementing. No TBD/TODO.

**Type consistency:** `PlaceGroup{Places,Source,DefaultConf}` and `MergePlaces` names match across Tasks 3, 6. `reelmedia.Mode`/`ModeThumbnail`/`ModeVideo` consistent across 5, 6, 7. `ExtractPlacesVisionFrames` signature identical across Tasks 4, 6. `framer` interface (Task 6) satisfied by `*reelmedia.Extractor` (Task 5 `Frames` signature matches). `items.tagged_location` → `db.Item.TaggedLocation` used in Tasks 2, 6. ✓
