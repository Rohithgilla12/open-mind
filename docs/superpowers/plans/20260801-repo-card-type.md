# `repo` Card Type Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Classify saved code-host URLs (github.com, gitlab.com, codeberg.org, bitbucket.org) as a new `repo` card type instead of `article`, including a backfill of items already saved.

**Architecture:** `repo` is the tenth card type. Classification is a pure URL heuristic in `enrich.Classify`, the same mechanism that already produces `video` and `tweet` — no AI involved. The card-type enum lives in `openapi.yaml` and is mirrored by hand in three Go sites and two TS client maps; all of them must move together. Existing rows are fixed by a one-column SQL migration whose predicate is exposed as an `IMMUTABLE` SQL function so a test can assert it agrees with the Go rule.

**Tech Stack:** Go 1.x (chi, sqlc, pgx), Postgres, oapi-codegen, Next.js + vitest (web), Expo (mobile), Taskfile.

**Design spec:** `docs/superpowers/specs/20260801-repo-card-type-design.md`

## Global Constraints

- **Contract first.** `openapi.yaml` is edited before any Go handler or TS consumer; `task generate` regenerates `apps/api/internal/api/gen.go` and `packages/api-client`. Never hand-edit generated files.
- **Card type string is `repo`** (singular, lowercase) everywhere in code and data. The user-facing label is `Repo`, plural `Repos`.
- **Hosts, exactly:** `github.com`, `gitlab.com`, `codeberg.org`, `bitbucket.org`. Not `sr.ht`, not `gist.github.com`.
- **Reserved first path segments, exactly:** `about apps collections contact enterprise explore features join login marketplace notifications orgs pricing pulls readme security settings sponsors topics trending -`
- **Go tests are table-driven.** Run with `cd apps/api && go test -p 1 ./...` (`-p 1` is required: store/enrich/search tests share one Postgres).
- **Postgres must be up** for Task 3's test: `task db` (compose db on port 5433, `TEST_DATABASE_URL` defaults to `postgres://openmind:openmind@localhost:5433/openmind_test`).
- Do **not** `git add -A` in this repo — unrelated WIP from parallel sessions is usually present. Stage the exact paths listed in each commit step.

---

### Task 1: Contract + Go enum mirrors

Adds `repo` to the schema and to the three Go sites that hand-copy the enum. Nothing classifies as `repo` yet — this task only makes the type *legal*, so a Lens rule can request it and the query parser can emit it.

**Files:**
- Modify: `openapi.yaml:689` (`Item.cardType`), `openapi.yaml:809` (`LensRule.types`)
- Modify: `apps/api/internal/ai/ai.go:73` (`parseQueryInstruction`), `apps/api/internal/ai/ai.go:210-213` (`cardTypes`)
- Modify: `apps/api/internal/api/lenses.go:41` (`validCardType`)
- Test: `apps/api/internal/ai/ai_test.go` (package `ai`), `apps/api/internal/api/lenses_internal_test.go` (package `api`)
- Regenerated (do not hand-edit): `apps/api/internal/api/gen.go`, `packages/api-client/src/schema.d.ts`

**Interfaces:**
- Consumes: nothing.
- Produces: the string literal `"repo"` is accepted by `validCardType(t string) bool` and retained by `sanitiseTypes(in []string) []string`.

- [ ] **Step 1: Write the failing tests**

Append to `apps/api/internal/ai/ai_test.go`:

```go
func TestSanitiseTypesKeepsRepo(t *testing.T) {
	got := sanitiseTypes([]string{"repo", "REPO", "gizmo", "article"})
	want := []string{"repo", "article"}
	if len(got) != len(want) {
		t.Fatalf("sanitiseTypes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sanitiseTypes[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
```

Append to `apps/api/internal/api/lenses_internal_test.go`:

```go
func TestValidCardTypeRepo(t *testing.T) {
	if !validCardType("repo") {
		t.Error("validCardType(\"repo\") = false, want true")
	}
	if validCardType("gizmo") {
		t.Error("validCardType(\"gizmo\") = true, want false")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/api && go test ./internal/ai/ -run TestSanitiseTypesKeepsRepo -v && go test ./internal/api/ -run TestValidCardTypeRepo -v`

Expected: both FAIL — `sanitiseTypes` drops `"repo"` (length mismatch), `validCardType("repo")` returns false.

- [ ] **Step 3: Add `repo` to the OpenAPI enum in both places**

`openapi.yaml` line 689:

```yaml
        cardType: { type: string, enum: [article, product, book, recipe, video, tweet, image, note, quote, repo] }
```

`openapi.yaml` line 809 (inside `LensRule.types`):

```yaml
          items: { type: string, enum: [article, product, book, recipe, video, tweet, image, note, quote, repo] }
```

- [ ] **Step 4: Regenerate the server types and TS client**

Run: `task generate`

Expected: `apps/api/internal/api/gen.go` and `packages/api-client/src/schema.d.ts` both gain `repo` in the card-type unions. If the task no-ops, the checksum cache is stale — run `task generate --force`.

- [ ] **Step 5: Update the three Go mirrors**

`apps/api/internal/ai/ai.go:73` — the type list inside `parseQueryInstruction`:

```go
	`"types" (a subset of [article, product, book, recipe, video, tweet, image, note, quote, repo] the user is asking for, otherwise []). ` +
```

`apps/api/internal/ai/ai.go:210-213`:

```go
var cardTypes = map[string]bool{
	"article": true, "product": true, "book": true, "recipe": true,
	"video": true, "tweet": true, "image": true, "note": true, "quote": true,
	"repo": true,
}
```

`apps/api/internal/api/lenses.go:41`:

```go
	case "article", "product", "book", "recipe", "video", "tweet", "image", "note", "quote", "repo":
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd apps/api && go test -p 1 ./internal/ai/ ./internal/api/`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add openapi.yaml apps/api/internal/ai/ai.go apps/api/internal/ai/ai_test.go \
        apps/api/internal/api/lenses.go apps/api/internal/api/lenses_internal_test.go \
        apps/api/internal/api/gen.go packages/api-client/src/schema.d.ts
git commit -m "feat(api): add repo to the card type enum"
```

---

### Task 2: Classify code-host URLs as `repo`

**Files:**
- Modify: `apps/api/internal/enrich/classify.go`
- Test: `apps/api/internal/enrich/classify_test.go`

**Interfaces:**
- Consumes: `"repo"` being a legal card type (Task 1).
- Produces: `enrich.Classify(rawURL string, _ Extraction) string` returns `"repo"` for repository-shaped code-host URLs. Task 3's parity test calls this.

- [ ] **Step 1: Write the failing test rows**

Add these rows to the existing `tests` table in `apps/api/internal/enrich/classify_test.go`, after the `{"default article", ...}` row:

```go
		{"repo root", "https://github.com/sqlc-dev/sqlc", "repo"},
		{"repo sub-page", "https://github.com/sqlc-dev/sqlc/pull/42", "repo"},
		{"repo blob with image extension", "https://github.com/o/r/blob/main/logo.png", "repo"},
		{"repo trailing slash", "https://github.com/o/r/", "repo"},
		{"repo with query", "https://github.com/o/r?tab=readme-ov-file", "repo"},
		{"uppercase host and owner", "https://GitHub.com/O/R", "repo"},
		{"github profile", "https://github.com/torvalds", "article"},
		{"github reserved segment", "https://github.com/features/copilot", "article"},
		{"github reserved segment cased", "https://github.com/Topics/go", "article"},
		{"github bare host", "https://github.com", "article"},
		{"gitlab nested group", "https://gitlab.com/group/sub/project", "repo"},
		{"gitlab dash route", "https://gitlab.com/-/profile", "article"},
		{"codeberg repo", "https://codeberg.org/forgejo/forgejo", "repo"},
		{"bitbucket repo", "https://bitbucket.org/workspace/project", "repo"},
		{"gist is not a repo", "https://gist.github.com/user/abc123", "article"},
		{"raw image host unaffected", "https://raw.githubusercontent.com/o/r/main/a.png", "image"},
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/api && go test ./internal/enrich/ -run TestClassify -v`

Expected: FAIL — every `repo` row reports `Classify(...) = "article", want "repo"`, and `repo blob with image extension` reports `"image"`.

- [ ] **Step 3: Implement the rule**

In `apps/api/internal/enrich/classify.go`, add below the `imageExts` var:

```go
// codeHosts are the code forges whose repository URLs classify as repo cards.
// A host alone is not enough: the same hosts serve profiles and marketing
// pages, so the path must also be repository-shaped (see isRepoPath).
var codeHosts = map[string]bool{
	"github.com":    true,
	"gitlab.com":    true,
	"codeberg.org":  true,
	"bitbucket.org": true,
}

// reservedOwners are first path segments the code hosts reserve for their own
// routes, so none can ever be a repository owner. Without this, two-segment
// product pages like github.com/features/copilot would read as repositories.
// "-" covers GitLab's /-/ infix routes.
var reservedOwners = map[string]bool{
	"about": true, "apps": true, "collections": true, "contact": true,
	"enterprise": true, "explore": true, "features": true, "join": true,
	"login": true, "marketplace": true, "notifications": true, "orgs": true,
	"pricing": true, "pulls": true, "readme": true, "security": true,
	"settings": true, "sponsors": true, "topics": true, "trending": true,
	"-": true,
}

// isRepoPath reports whether p is repository-shaped: at least two non-empty
// segments whose first is not a reserved host route. Sub-pages keep the repo
// type — an issue, a pull request, or a blob is still that project.
func isRepoPath(p string) bool {
	segs := make([]string, 0, 4)
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	if len(segs) < 2 {
		return false
	}
	return !reservedOwners[strings.ToLower(segs[0])]
}
```

Then in `Classify`, insert the check **after** the `socialVideoHosts` block and **before** the `imageExts` check:

```go
	if _, ok := socialVideoHosts[host]; ok {
		return "video"
	}

	// Before the extension check on purpose: a .png under /blob/ is an HTML
	// page on the forge, not an image file.
	if codeHosts[host] && isRepoPath(parsed.Path) {
		return "repo"
	}

	if imageExts[strings.ToLower(path.Ext(parsed.Path))] {
		return "image"
	}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/api && go test ./internal/enrich/ -run TestClassify -v`

Expected: PASS, all rows.

- [ ] **Step 5: Run the full enrich package to check nothing regressed**

Run: `cd apps/api && go test -p 1 ./internal/enrich/`

Expected: PASS. The pipeline idempotency tests must still pass — `Classify` is pure, so a second run yields `repo` again.

- [ ] **Step 6: Commit**

```bash
git add apps/api/internal/enrich/classify.go apps/api/internal/enrich/classify_test.go
git commit -m "feat(enrich): classify code-host repository URLs as repo cards"
```

---

### Task 3: Backfill migration + SQL/Go parity test

Existing code-host saves are already `article`. This migration fixes them in place — one column, no refetch, no AI spend.

**Files:**
- Create: `apps/api/internal/store/migrations/0021_repo_card_type.sql`
- Create: `apps/api/internal/store/repo_url_test.go` (package `store_test`)

**Interfaces:**
- Consumes: `enrich.Classify` returning `"repo"` (Task 2); `testStore(t *testing.T) *store.Store` from `apps/api/internal/store/store_test.go`; `store.Store.Pool` (`*pgxpool.Pool`).
- Produces: SQL function `item_url_is_repo(url text) RETURNS boolean`, kept after the backfill so the parity test has something stable to call.

- [ ] **Step 1: Write the failing parity test**

Create `apps/api/internal/store/repo_url_test.go`:

```go
package store_test

import (
	"context"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/enrich"
)

// repoURLFixtures pin the two implementations of one rule together: the Go
// classifier that types new saves, and the SQL twin that backfilled the old
// ones. If they ever disagree, a library contains two definitions of "repo".
var repoURLFixtures = []struct {
	url    string
	isRepo bool
}{
	{"https://github.com/sqlc-dev/sqlc", true},
	{"https://github.com/sqlc-dev/sqlc/pull/42", true},
	{"https://github.com/o/r/blob/main/logo.png", true},
	{"https://github.com/o/r/", true},
	{"https://github.com/o/r?tab=readme-ov-file", true},
	{"https://www.github.com/o/r", true},
	{"https://GitHub.com/O/R", true},
	{"https://github.com/torvalds", false},
	{"https://github.com/features/copilot", false},
	{"https://github.com/Topics/go", false},
	{"https://github.com", false},
	{"https://gitlab.com/group/sub/project", true},
	{"https://gitlab.com/-/profile", false},
	{"https://codeberg.org/forgejo/forgejo", true},
	{"https://bitbucket.org/workspace/project", true},
	{"https://gist.github.com/user/abc123", false},
	{"https://blog.example.com/post", false},
}

func TestRepoURLSQLMatchesGoClassify(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, f := range repoURLFixtures {
		t.Run(f.url, func(t *testing.T) {
			var sqlSaysRepo bool
			if err := s.Pool.QueryRow(ctx, `SELECT item_url_is_repo($1)`, f.url).Scan(&sqlSaysRepo); err != nil {
				t.Fatalf("item_url_is_repo(%q): %v", f.url, err)
			}
			goSaysRepo := enrich.Classify(f.url, enrich.Extraction{}) == "repo"
			if sqlSaysRepo != goSaysRepo {
				t.Errorf("%s: SQL says repo=%v, Go says repo=%v", f.url, sqlSaysRepo, goSaysRepo)
			}
			if goSaysRepo != f.isRepo {
				t.Errorf("%s: classified repo=%v, want %v", f.url, goSaysRepo, f.isRepo)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `task db && cd apps/api && go test -p 1 ./internal/store/ -run TestRepoURLSQLMatchesGoClassify -v`

Expected: FAIL with `function item_url_is_repo(unknown) does not exist`.

- [ ] **Step 3: Write the migration**

Create `apps/api/internal/store/migrations/0021_repo_card_type.sql`:

```sql
-- Code-forge repository URLs get their own card type instead of being filed as
-- articles. The rule for new saves lives in Go (enrich.Classify); this function
-- is its SQL twin, used for the one-off backfill below and kept afterwards so
-- internal/store/repo_url_test.go can assert the two still agree.
--
-- Two predicates rather than one regex with a negative lookahead: each half
-- maps to one half of the Go rule (>=2 path segments; first segment not a
-- reserved host route), which keeps them reviewable side by side.
CREATE OR REPLACE FUNCTION item_url_is_repo(url text) RETURNS boolean
    LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT url ~* '^https?://(www\.)?(github\.com|gitlab\.com|codeberg\.org|bitbucket\.org)/[^/?#]+/[^/?#]+'
       AND url !~* '^https?://(www\.)?(github\.com|gitlab\.com|codeberg\.org|bitbucket\.org)/(about|apps|collections|contact|enterprise|explore|features|join|login|marketplace|notifications|orgs|pricing|pulls|readme|security|settings|sponsors|topics|trending|-)(/|\?|#|$)';
$$;

-- Scoped to 'article' so the backfill cannot overwrite an image or PDF
-- classification that the pipeline determined by other means.
UPDATE items SET card_type = 'repo'
WHERE card_type = 'article' AND item_url_is_repo(url);
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/api && go test -p 1 ./internal/store/ -run TestRepoURLSQLMatchesGoClassify -v`

Expected: PASS, all 17 fixture rows.

- [ ] **Step 5: Verify the backfill against your real dev database**

Apply the migration to the dev database and check the post-condition — no code-host URL is still filed as an article:

```bash
task migrate
psql "postgres://openmind:openmind@localhost:5433/openmind" -c \
  "SELECT card_type, count(*) FROM items WHERE item_url_is_repo(url) GROUP BY card_type;"
```

Expected: every returned row is `repo`. An `article` row in that output means the backfill missed something the predicate now matches — investigate before continuing.

If your dev library has no code-host saves the result is empty, which proves nothing; in that case save one github.com URL through the app first, then re-run.

- [ ] **Step 6: Run the whole Go suite**

Run: `cd apps/api && go test -p 1 ./...`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/api/internal/store/migrations/0021_repo_card_type.sql \
        apps/api/internal/store/repo_url_test.go
git commit -m "feat(store): backfill code-host items to the repo card type"
```

---

### Task 4: Web presentation

**Files:**
- Modify: `apps/web/lib/cards.ts` (`CardKind`, `KNOWN_KINDS`, `typeGradient`, `typeLabel`)
- Modify: `apps/web/components/LensForm.tsx:11-21` (`ALL_TYPES`)
- Create: `apps/web/lib/cards.test.ts`

**Interfaces:**
- Consumes: the API returning `cardType: "repo"` (Tasks 1-3).
- Produces: `cardKind("repo")` returns `"repo"`; `typeLabel.repo === "Repo"`; `typeGradient.repo` is a CSS gradient string.

- [ ] **Step 1: Write the failing test**

Create `apps/web/lib/cards.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { cardKind, typeGradient, typeLabel } from "./cards";

describe("repo card type", () => {
  it("normalises repo to itself", () => {
    expect(cardKind("repo")).toBe("repo");
  });
  it("labels it Repo", () => {
    expect(typeLabel.repo).toBe("Repo");
  });
  it("has a gradient", () => {
    expect(typeGradient.repo).toContain("linear-gradient");
  });
  it("still falls back to article for unknown types", () => {
    expect(cardKind("gizmo")).toBe("article");
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm turbo run test --filter=web`

Expected: FAIL — TypeScript rejects `typeLabel.repo` (property does not exist on `Record<CardKind, string>`), and `cardKind("repo")` returns `"article"`.

- [ ] **Step 3: Add the type to `apps/web/lib/cards.ts`**

Add `| "repo"` to the `CardKind` union (after `"recipe"`), then add `"repo",` to the end of `KNOWN_KINDS`, and one entry to each map:

```ts
export const typeGradient: Record<CardKind, string> = {
  article: `linear-gradient(120deg, ${color.cobalt}, ${color.cobaltDeep})`,
  quote: `linear-gradient(135deg, ${color.ink}, ${color.cobaltDeep})`,
  image: `linear-gradient(150deg, ${color.terracotta} 0%, ${color.gold} 55%, ${color.paper} 100%)`,
  product: `linear-gradient(135deg, ${color.green}, ${color.ink})`,
  note: `linear-gradient(135deg, ${color.gold}, ${color.noteSurface})`,
  video: `linear-gradient(135deg, ${color.ink}, rgba(0,0,0,1))`,
  tweet: `linear-gradient(135deg, ${color.cobalt}, ${color.green})`,
  book: `linear-gradient(160deg, ${color.terracotta}, ${color.ink})`,
  recipe: `linear-gradient(135deg, ${color.terracotta}, ${color.gold})`,
  repo: `linear-gradient(135deg, ${color.gold}, ${color.green})`,
};
```

```ts
export const typeLabel: Record<CardKind, string> = {
  article: "Article",
  quote: "Quote",
  image: "Image",
  product: "Product",
  note: "Note",
  video: "Video",
  tweet: "Post",
  book: "Book",
  recipe: "Recipe",
  repo: "Repo",
};
```

- [ ] **Step 4: Add the chip to the Lens form**

`apps/web/components/LensForm.tsx`, in `ALL_TYPES` (add after `"quote"`):

```ts
const ALL_TYPES: readonly CardKind[] = [
  "article",
  "product",
  "book",
  "recipe",
  "video",
  "tweet",
  "image",
  "note",
  "quote",
  "repo",
];
```

- [ ] **Step 5: Run the tests and typecheck**

Run: `pnpm turbo run test --filter=web && pnpm turbo run lint --filter=web`

Expected: PASS. If `tsc` complains about a missing `repo` key in any `Record<CardKind, …>` map elsewhere in the app, add the entry there too — the compiler is enumerating the remaining sites for you.

- [ ] **Step 6: Commit**

```bash
git add apps/web/lib/cards.ts apps/web/lib/cards.test.ts apps/web/components/LensForm.tsx
git commit -m "feat(web): render and filter the repo card type"
```

---

### Task 5: Mobile presentation

**Files:**
- Modify: `apps/mobile/lib/theme.ts:60-88` (`CardKind` union, `typeGradients`)
- Modify: `apps/mobile/lib/cards.ts` (`KNOWN_KINDS`, `typeLabel`, `typeLabelPlural`)

**Interfaces:**
- Consumes: the API returning `cardType: "repo"`.
- Produces: `cardKind("repo") === "repo"`, `typeLabel.repo === "Repo"`, `typeLabelPlural.repo === "Repos"`, `typeGradients.repo` as a `[string, string]` pair.

- [ ] **Step 1: Add `repo` to the theme union and gradient map**

`apps/mobile/lib/theme.ts` — add `| "repo"` after `| "recipe"` in the `CardKind` union, then add the gradient (matching web's `gold → green`):

```ts
export const typeGradients: Record<CardKind, [string, string]> = {
  article: [colors.cobalt, colors.cobaltDeep],
  quote: [colors.ink, colors.cobaltDeep],
  image: [colors.terracotta, colors.gold],
  product: [colors.green, colors.ink],
  note: [colors.gold, colors.note],
  video: [colors.ink, "#000000"],
  tweet: [colors.cobalt, colors.green],
  book: [colors.terracotta, colors.ink],
  recipe: [colors.terracotta, colors.gold],
  repo: [colors.gold, colors.green],
} as const;
```

- [ ] **Step 2: Add `repo` to the card helpers**

`apps/mobile/lib/cards.ts` — add `"repo",` to the end of `KNOWN_KINDS`, then:

```ts
export const typeLabel: Record<CardKind, string> = {
  article: "Article",
  quote: "Quote",
  image: "Image",
  product: "Product",
  note: "Note",
  video: "Video",
  tweet: "Post",
  book: "Book",
  recipe: "Recipe",
  repo: "Repo",
};

export const typeLabelPlural: Record<CardKind, string> = {
  article: "Articles",
  quote: "Quotes",
  image: "Images",
  product: "Products",
  note: "Notes",
  video: "Videos",
  tweet: "Posts",
  book: "Books",
  recipe: "Recipes",
  repo: "Repos",
};
```

- [ ] **Step 3: Typecheck**

Run: `pnpm turbo run lint --filter=mobile`

Expected: PASS. Any error of the form `Property 'repo' is missing in type` names another `Record<CardKind, …>` that needs the entry — add it and re-run.

- [ ] **Step 4: Run the mobile test suite**

Run: `pnpm turbo run test --filter=mobile`

Expected: PASS, unchanged.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile/lib/theme.ts apps/mobile/lib/cards.ts
git commit -m "feat(mobile): render the repo card type"
```

---

### Task 6: Documentation

**Files:**
- Modify: `CLAUDE.md:82` (product vocabulary card-type list)
- Modify: `TODO.md` (Later section)

**Interfaces:**
- Consumes: everything above.
- Produces: nothing code-facing.

- [ ] **Step 1: Update the card-type list in `CLAUDE.md`**

Line 82 becomes:

```markdown
- Card types: article, product, book, recipe, video, tweet, image, note, quote, repo
```

- [ ] **Step 2: Record the denylist debt in `TODO.md`**

Add to the **Later** section:

```markdown
- `repo` card type: the reserved-first-segment denylist in
  `apps/api/internal/enrich/classify.go` (and its SQL twin in migration 0021)
  is not exhaustive by construction. When a forge adds a reserved route, URLs
  under it misclassify as repos until someone notices. Revisit if it bites more
  than once; the alternative (repo-root-only matching) was rejected in
  `docs/superpowers/specs/20260801-repo-card-type-design.md` because it splits
  one project's saves across two card types.
```

- [ ] **Step 3: Full verification**

Run: `task test && task lint`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md TODO.md
git commit -m "docs: record the repo card type and its denylist debt"
```

---

## Manual verification (after Task 6)

Not automated because it needs a live enrichment run against the network:

1. `task dev`, then save `https://github.com/riverqueue/river` from the web capture box.
2. Wait for enrichment to finish (card stops showing *Enriching*).
3. Confirm the card renders with the `Repo` meta label and the gold→green gradient.
4. Create a Lens with only the **Repo** chip ticked; confirm the new item and any backfilled items appear.
5. Save `https://github.com/features/copilot` and confirm it stays an **Article**.
