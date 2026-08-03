package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// repoURLFixtures pin the two implementations of one rule together: the Go
// classifier that types new saves, and the SQL twin that backfilled the old
// ones. If they ever disagree, a library contains two definitions of "repo".
//
// The reserved-owner rows below (about, apps, collections, ... trending, and
// "-" via the GitLab row) deliberately cover all 21 reserved first-path
// segments from enrich.reservedOwners by name: the SQL migration hand-
// transcribes that list into a regex, and a typo in any single word there
// would otherwise go uncaught.
//
// Scope note: this parity guarantee holds for http/https URLs without
// percent-encoded path separators. `https://github.com/o%2Fr` diverges — Go's
// url.Parse decodes `%2F` into a path separator (two segments), while the SQL
// regex sees one raw `%2F` token (one segment). Vanishingly rare in practice
// and self-corrects for new saves, so not worth code changes, but the parity
// claim above shouldn't be read as covering that case.
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

	// Remaining reserved segments not already exercised above (features,
	// topics, and "-" are covered by the rows above).
	{"https://github.com/about/team", false},
	{"https://github.com/apps/dependabot", false},
	{"https://github.com/collections/deep-learning", false},
	{"https://github.com/contact/us", false},
	{"https://github.com/enterprise/server", false},
	{"https://github.com/explore/repositories", false},
	{"https://github.com/join/waitlist", false},
	{"https://github.com/login/oauth", false},
	{"https://github.com/marketplace/actions", false},
	{"https://github.com/notifications/inbox", false},
	{"https://github.com/orgs/acme", false},
	{"https://github.com/pricing/plans", false},
	{"https://github.com/pulls/review", false},
	{"https://github.com/readme/guide", false},
	{"https://github.com/security/advisories", false},
	{"https://github.com/settings/profile", false},
	{"https://github.com/sponsors/community", false},
	{"https://github.com/trending/go", false},

	// Authority variations: everything above varies path shape but keeps the
	// same plain "https://host/owner/repo" authority, so it cannot catch a
	// regex that only handles that one shape. Go's url.Parse + Hostname()
	// strip userinfo and port before enrich.Classify ever sees the host, and
	// silently drops empty path segments before counting them, so the SQL
	// twin must handle all of these the same way.
	{"https://user@bitbucket.org/workspace/project", true}, // userinfo (a real Bitbucket HTTPS clone URL)
	{"https://github.com:443/o/r", true},                   // explicit port
	{"https://github.com//o/r", true},                      // doubled slash after host
	{"https://WWW.GITHUB.COM/O/R", true},                   // uppercase "WWW." + host
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

// TestBackfillUpdatesOnlyRepoShapedArticles exercises the migration's UPDATE
// statement directly: a repo-shaped article must flip to "repo", while an
// article whose URL only looks repo-like because of a reserved path segment,
// and an item already classified as something else, must both stay put.
func TestBackfillUpdatesOnlyRepoShapedArticles(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	repoArticle, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
		UserID: userID,
		Url:    "https://github.com/sqlc-dev/sqlc",
		Body:   "",
	})
	if err != nil {
		t.Fatalf("create repo article: %v", err)
	}

	reservedSegmentArticle, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
		UserID: userID,
		Url:    "https://github.com/features/copilot",
		Body:   "",
	})
	if err != nil {
		t.Fatalf("create control article: %v", err)
	}

	preTypedImage, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
		UserID: userID,
		Url:    "https://example.com/photo.png",
		Body:   "",
	})
	if err != nil {
		t.Fatalf("create control image: %v", err)
	}
	// CreateItem always defaults card_type to 'article'; force this one to
	// 'image' so the backfill's card_type = 'article' scope has something to
	// respect.
	if _, err := s.Pool.Exec(ctx, `UPDATE items SET card_type = 'image' WHERE id = $1`, preTypedImage.ID); err != nil {
		t.Fatalf("seeding pre-typed image: %v", err)
	}

	// The migration's exact backfill statement.
	const backfillSQL = `UPDATE items SET card_type = 'repo'
WHERE card_type = 'article' AND item_url_is_repo(url)`
	if _, err := s.Pool.Exec(ctx, backfillSQL); err != nil {
		t.Fatalf("running backfill: %v", err)
	}

	cardType := func(id uuid.UUID) string {
		t.Helper()
		var ct string
		if err := s.Pool.QueryRow(ctx, `SELECT card_type FROM items WHERE id = $1`, id).Scan(&ct); err != nil {
			t.Fatalf("querying card_type: %v", err)
		}
		return ct
	}

	if got := cardType(repoArticle.ID); got != "repo" {
		t.Errorf("repo article card_type = %q, want repo", got)
	}
	if got := cardType(reservedSegmentArticle.ID); got != "article" {
		t.Errorf("reserved-segment article card_type = %q, want article (unchanged)", got)
	}
	if got := cardType(preTypedImage.ID); got != "image" {
		t.Errorf("pre-typed image card_type = %q, want image (unchanged)", got)
	}
}
