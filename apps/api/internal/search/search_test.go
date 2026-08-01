package search_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/search"
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
	// No TRUNCATE: every test uses a fresh random user_id, and all queries are
	// tenant-scoped, so rows from other tests/packages are invisible here.
	// Avoiding TRUNCATE keeps these tests from racing other packages that share
	// the database when `go test ./...` runs package binaries in parallel.
	return store.New(pool)
}

// seedItem creates an enriched item with an embedding for the given user.
func seedItem(t *testing.T, s *store.Store, p ai.Provider, userID uuid.UUID, title, body string) db.Item {
	t.Helper()
	ctx := context.Background()
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: "https://example.com/" + title, Body: ""})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if err := s.Queries.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: item.ID, Title: title, Body: body, CardType: "article",
	}); err != nil {
		t.Fatalf("update extraction: %v", err)
	}
	if err := s.Queries.UpdateItemEnrichment(ctx, db.UpdateItemEnrichmentParams{
		UserID: userID, ID: item.ID, Summary: body, Tags: []string{title},
	}); err != nil {
		t.Fatalf("update enrichment: %v", err)
	}
	vec, err := p.Embed(ctx, body)
	if err == nil {
		if err := s.Queries.UpsertEmbedding(ctx, db.UpsertEmbeddingParams{
			ItemID: item.ID, UserID: userID, Embedding: pgvector.NewVector(vec),
		}); err != nil {
			t.Fatalf("upsert embedding: %v", err)
		}
	}
	return item
}

func TestHybridRanksMatchFirst(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p := ai.NewFake()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	bread := seedItem(t, s, p, userID, "sourdough", "a guide to sourdough fermentation and bread baking")
	seedItem(t, s, p, userID, "rustlang", "understanding the rust borrow checker and ownership")

	results, err := search.Hybrid(ctx, s, p, userID, "sourdough", 10)
	if err != nil {
		t.Fatalf("hybrid: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}
	if results[0].Item.ID != bread.ID {
		t.Errorf("first result = %v, want bread %v", results[0].Item.ID, bread.ID)
	}
}

func TestHybridNoopFTSOnly(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	// Noop provider: Embed unsupported, so no embeddings are written.
	np := ai.NewNoop()
	bread := seedItem(t, s, np, userID, "sourdough", "a guide to sourdough fermentation and bread baking")

	results, err := search.Hybrid(ctx, s, np, userID, "fermentation", 10)
	if err != nil {
		t.Fatalf("hybrid (noop): %v", err)
	}
	if len(results) != 1 || results[0].Item.ID != bread.ID {
		t.Fatalf("noop FTS results = %v, want [%v]", results, bread.ID)
	}
}

// seedColorItem creates an item with a fixed palette (no embedding needed).
func seedColorItem(t *testing.T, s *store.Store, userID uuid.UUID, title string, palette []string) db.Item {
	t.Helper()
	ctx := context.Background()
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: "https://example.com/" + title, Body: ""})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if err := s.Queries.SetItemPalette(ctx, db.SetItemPaletteParams{UserID: userID, ID: item.ID, Palette: palette}); err != nil {
		t.Fatalf("set palette: %v", err)
	}
	return item
}

func TestColorSearchRanksClosestPalette(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	np := ai.NewNoop()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	blue := seedColorItem(t, s, userID, "blue", []string{"#1B3FD1", "#F4F0E6"})
	seedColorItem(t, s, userID, "red", []string{"#D1291B", "#FCFBF6"})

	results, err := search.Run(ctx, s, np, userID, "", "cobalt", nil, 10)
	if err != nil {
		t.Fatalf("color search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].Item.ID != blue.ID {
		t.Errorf("closest to cobalt = %v, want blue %v", results[0].Item.ID, blue.ID)
	}
}

func TestColorSearchBadColor(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_, err := search.Run(ctx, s, ai.NewNoop(), uuid.New(), "", "chartreuse-ish", nil, 10)
	if !errors.Is(err, search.ErrBadColor) {
		t.Fatalf("err = %v, want ErrBadColor", err)
	}
}

func TestColorSearchTenantIsolation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	owner := uuid.New()
	other := uuid.New()
	for _, u := range []uuid.UUID{owner, other} {
		if err := s.Queries.EnsureUser(ctx, u); err != nil {
			t.Fatalf("ensure user: %v", err)
		}
	}
	seedColorItem(t, s, owner, "blue", []string{"#1B3FD1"})
	results, err := search.Run(ctx, s, ai.NewNoop(), other, "", "cobalt", nil, 10)
	if err != nil {
		t.Fatalf("color search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("cross-tenant results = %d, want 0", len(results))
	}
}

// seedTypedItem creates an enriched item with the given card type and an
// embedding, so type-filter tests can distinguish otherwise-identical matches.
func seedTypedItem(t *testing.T, s *store.Store, p ai.Provider, userID uuid.UUID, title, body, cardType string) db.Item {
	t.Helper()
	ctx := context.Background()
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: "https://example.com/" + title, Body: ""})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if err := s.Queries.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: item.ID, Title: title, Body: body, CardType: cardType,
	}); err != nil {
		t.Fatalf("update extraction: %v", err)
	}
	if err := s.Queries.UpdateItemEnrichment(ctx, db.UpdateItemEnrichmentParams{
		UserID: userID, ID: item.ID, Summary: body, Tags: []string{title},
	}); err != nil {
		t.Fatalf("update enrichment: %v", err)
	}
	if vec, err := p.Embed(ctx, body); err == nil {
		if err := s.Queries.UpsertEmbedding(ctx, db.UpsertEmbeddingParams{
			ItemID: item.ID, UserID: userID, Embedding: pgvector.NewVector(vec),
		}); err != nil {
			t.Fatalf("upsert embedding: %v", err)
		}
	}
	return item
}

func TestRunFiltersByType(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p := ai.NewFake()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	// Two items matching the same text, differing only by card type.
	bookID := seedTypedItem(t, s, p, userID, "bread book", "a book about baking bread", "book").ID
	seedTypedItem(t, s, p, userID, "bread article", "an article about baking bread", "article")

	all, err := search.Run(ctx, s, p, userID, "bread", "", nil, 10)
	if err != nil {
		t.Fatalf("run (unfiltered): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("unfiltered results = %d, want 2", len(all))
	}

	books, err := search.Run(ctx, s, p, userID, "bread", "", []string{"book"}, 10)
	if err != nil {
		t.Fatalf("run (book filter): %v", err)
	}
	if len(books) != 1 || books[0].Item.ID != bookID {
		t.Fatalf("book-filtered results = %v, want [%v]", books, bookID)
	}
}

// TestTagSearchStemsEnglishTags guards against a stemming mismatch between
// index time and query time: search_tsv historically indexed tags as literal
// lexemes (array_to_tsvector) while SearchFTS queries with
// websearch_to_tsquery('english', ...), which stems. A multi-morpheme tag
// like "favourite" stems to "favourit" on the query side but was indexed
// literally, so it never matched. Single-morpheme tags (e.g. "mine") happened
// to work regardless, which is why this went unnoticed. It also verifies
// PinnedAt survives into search results, since the FTS/vector row-to-item
// mappers historically dropped it.
func TestTagSearchStemsEnglishTags(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	np := ai.NewNoop()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	// user_tags case: a stemmable, user-supplied tag.
	favourite, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: "https://example.com/favourite", Body: ""})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if _, err := s.Queries.SetUserTags(ctx, db.SetUserTagsParams{UserID: userID, ID: favourite.ID, UserTags: []string{"favourite"}}); err != nil {
		t.Fatalf("set user tags: %v", err)
	}
	if _, err := s.Queries.SetItemPinned(ctx, db.SetItemPinnedParams{UserID: userID, ID: favourite.ID, PinnedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}); err != nil {
		t.Fatalf("set pinned: %v", err)
	}

	// tags (AI-set) case: a stemmable tag.
	running, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: "https://example.com/running", Body: ""})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if err := s.Queries.UpdateItemEnrichment(ctx, db.UpdateItemEnrichmentParams{
		UserID: userID, ID: running.ID, Summary: "", Tags: []string{"running"},
	}); err != nil {
		t.Fatalf("update enrichment: %v", err)
	}

	// Regression: a single-morpheme user tag must keep matching.
	mine, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: "https://example.com/mine", Body: ""})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if _, err := s.Queries.SetUserTags(ctx, db.SetUserTagsParams{UserID: userID, ID: mine.ID, UserTags: []string{"mine"}}); err != nil {
		t.Fatalf("set user tags: %v", err)
	}

	favResults, err := search.Run(ctx, s, np, userID, "favourite", "", nil, 10)
	if err != nil {
		t.Fatalf("search favourite: %v", err)
	}
	if len(favResults) != 1 || favResults[0].Item.ID != favourite.ID {
		t.Fatalf("search(favourite) = %v, want [%v]", favResults, favourite.ID)
	}
	if !favResults[0].Item.PinnedAt.Valid {
		t.Errorf("search result PinnedAt = %v, want a valid pinned timestamp", favResults[0].Item.PinnedAt)
	}

	runResults, err := search.Run(ctx, s, np, userID, "running", "", nil, 10)
	if err != nil {
		t.Fatalf("search running: %v", err)
	}
	if len(runResults) != 1 || runResults[0].Item.ID != running.ID {
		t.Fatalf("search(running) = %v, want [%v]", runResults, running.ID)
	}

	mineResults, err := search.Run(ctx, s, np, userID, "mine", "", nil, 10)
	if err != nil {
		t.Fatalf("search mine: %v", err)
	}
	if len(mineResults) != 1 || mineResults[0].Item.ID != mine.ID {
		t.Fatalf("search(mine) = %v, want [%v]", mineResults, mine.ID)
	}
}

// TestSearchRanksLibraryBeforeUnkeptFeed guards the library-first partition:
// an unkept feed-river item stays searchable but must trail every library
// match, even when its raw FTS rank is higher. Keeping the item promotes it
// into the library partition. It also verifies FeedID and KeptAt survive the
// FTS row-to-item mapping, which historically dropped them.
func TestSearchRanksLibraryBeforeUnkeptFeed(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	np := ai.NewNoop()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	// Library item: a single, weak mention of the query term.
	saved := seedItem(t, s, np, userID, "starter notes", "notes that mention sourdough once")

	// Feed item: term in title and repeatedly in body, so its ts_rank beats
	// the library item's — only the partition can put it second.
	feed, err := s.Queries.CreateFeed(ctx, db.CreateFeedParams{UserID: userID, Url: "https://example.com/feed.xml", Title: "baking feed"})
	if err != nil {
		t.Fatalf("create feed: %v", err)
	}
	riverItem, err := s.Queries.CreateFeedItem(ctx, db.CreateFeedItemParams{UserID: userID, Url: "https://example.com/river-sourdough", FeedID: pgtype.UUID{Bytes: feed.ID, Valid: true}})
	if err != nil {
		t.Fatalf("create feed item: %v", err)
	}
	if err := s.Queries.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: riverItem.ID, Title: "sourdough sourdough",
		Body: "sourdough sourdough sourdough — everything about sourdough", CardType: "article",
	}); err != nil {
		t.Fatalf("update extraction: %v", err)
	}

	results, err := search.Run(ctx, s, np, userID, "sourdough", "", nil, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2 (feed matches must stay included)", len(results))
	}
	if results[0].Item.ID != saved.ID || results[1].Item.ID != riverItem.ID {
		t.Fatalf("order = [%v %v], want library %v before feed %v",
			results[0].Item.ID, results[1].Item.ID, saved.ID, riverItem.ID)
	}
	if !results[1].Item.FeedID.Valid {
		t.Error("feed result FeedID invalid, want it carried through the row mapping")
	}

	// Keeping the feed item moves it into the library partition, where its
	// stronger match now ranks it first.
	if _, err := s.Queries.SetItemKept(ctx, db.SetItemKeptParams{
		UserID: userID, ID: riverItem.ID, KeptAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}); err != nil {
		t.Fatalf("set kept: %v", err)
	}
	results, err = search.Run(ctx, s, np, userID, "sourdough", "", nil, 10)
	if err != nil {
		t.Fatalf("search after keep: %v", err)
	}
	if len(results) != 2 || results[0].Item.ID != riverItem.ID {
		t.Fatalf("after keep, first result = %v, want kept feed item %v", results[0].Item.ID, riverItem.ID)
	}
	if !results[0].Item.KeptAt.Valid {
		t.Error("kept result KeptAt invalid, want it carried through the row mapping")
	}
}

func TestHybridTenantIsolation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p := ai.NewFake()
	owner := uuid.New()
	other := uuid.New()
	if err := s.Queries.EnsureUser(ctx, owner); err != nil {
		t.Fatalf("ensure owner: %v", err)
	}
	if err := s.Queries.EnsureUser(ctx, other); err != nil {
		t.Fatalf("ensure other: %v", err)
	}
	seedItem(t, s, p, owner, "sourdough", "a guide to sourdough fermentation and bread baking")

	results, err := search.Hybrid(ctx, s, p, other, "sourdough", 10)
	if err != nil {
		t.Fatalf("hybrid: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("cross-tenant results = %d, want 0", len(results))
	}
}

// seedURLItem creates an item with a specific URL and card type (no embedding).
func seedURLItem(t *testing.T, s *store.Store, userID uuid.UUID, rawURL, title, cardType string) db.Item {
	t.Helper()
	ctx := context.Background()
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: rawURL, Body: ""})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if err := s.Queries.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: item.ID, Title: title, Body: title, CardType: cardType,
	}); err != nil {
		t.Fatalf("update extraction: %v", err)
	}
	got, err := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	return got
}

func resultIDs(results []search.Result) []uuid.UUID {
	ids := make([]uuid.UUID, len(results))
	for i, r := range results {
		ids[i] = r.Item.ID
	}
	return ids
}

func containsID(results []search.Result, id uuid.UUID) bool {
	for _, r := range results {
		if r.Item.ID == id {
			return true
		}
	}
	return false
}

func TestDomainFilterMatchesHostAndSubdomain(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	np := ai.NewNoop()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	xcom := seedURLItem(t, s, userID, "https://x.com/a", "x post", "tweet")
	mobileTwitter := seedURLItem(t, s, userID, "https://mobile.twitter.com/b", "mobile tweet", "tweet")
	seedURLItem(t, s, userID, "https://example.com/c", "other", "article")

	onlyX, err := search.RunQuery(ctx, s, np, userID, search.Query{
		Domains: []string{"x.com"}, Scope: search.ScopeLibrary,
	}, 10)
	if err != nil {
		t.Fatalf("domains x.com: %v", err)
	}
	if len(onlyX) != 1 || onlyX[0].Item.ID != xcom.ID {
		t.Fatalf("domains x.com = %v, want [%v]", resultIDs(onlyX), xcom.ID)
	}

	onlyTwitter, err := search.RunQuery(ctx, s, np, userID, search.Query{
		Domains: []string{"twitter.com"}, Scope: search.ScopeLibrary,
	}, 10)
	if err != nil {
		t.Fatalf("domains twitter.com: %v", err)
	}
	if len(onlyTwitter) != 1 || onlyTwitter[0].Item.ID != mobileTwitter.ID {
		t.Fatalf("domains twitter.com = %v, want [%v]", resultIDs(onlyTwitter), mobileTwitter.ID)
	}

	both, err := search.RunQuery(ctx, s, np, userID, search.Query{
		Domains: []string{"x.com", "twitter.com"}, Scope: search.ScopeLibrary,
	}, 10)
	if err != nil {
		t.Fatalf("domains both: %v", err)
	}
	if len(both) != 2 || !containsID(both, xcom.ID) || !containsID(both, mobileTwitter.ID) {
		t.Fatalf("domains both = %v, want [%v %v]", resultIDs(both), xcom.ID, mobileTwitter.ID)
	}
}

func TestLibraryScopeExcludesUnkeptFeed(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	np := ai.NewNoop()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	saved := seedTypedItem(t, s, np, userID, "library article", "a saved library article about bread", "article")

	feed, err := s.Queries.CreateFeed(ctx, db.CreateFeedParams{UserID: userID, Url: "https://example.com/feed.xml", Title: "feed"})
	if err != nil {
		t.Fatalf("create feed: %v", err)
	}
	river, err := s.Queries.CreateFeedItem(ctx, db.CreateFeedItemParams{
		UserID: userID, Url: "https://example.com/unkept-article", FeedID: pgtype.UUID{Bytes: feed.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("create feed item: %v", err)
	}
	if err := s.Queries.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: river.ID, Title: "unkept feed article", Body: "feed article about bread", CardType: "article",
	}); err != nil {
		t.Fatalf("update extraction: %v", err)
	}

	libOnly, err := search.RunQuery(ctx, s, np, userID, search.Query{
		Types: []string{"article"}, Scope: search.ScopeLibrary,
	}, 10)
	if err != nil {
		t.Fatalf("library scope: %v", err)
	}
	if !containsID(libOnly, saved.ID) {
		t.Fatalf("library scope missing saved item; got %v", resultIDs(libOnly))
	}
	if containsID(libOnly, river.ID) {
		t.Fatalf("library scope included unkept feed item %v", river.ID)
	}

	all, err := search.RunQuery(ctx, s, np, userID, search.Query{
		Types: []string{"article"}, Scope: search.ScopeAll,
	}, 10)
	if err != nil {
		t.Fatalf("scope all: %v", err)
	}
	if !containsID(all, saved.ID) || !containsID(all, river.ID) {
		t.Fatalf("scope all = %v, want both %v and %v", resultIDs(all), saved.ID, river.ID)
	}
}

func TestFilterOnlyDomainsUsesListPath(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	np := ai.NewNoop()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	older := seedURLItem(t, s, userID, "https://x.com/older", "older", "tweet")
	newer := seedURLItem(t, s, userID, "https://x.com/newer", "newer", "tweet")
	seedURLItem(t, s, userID, "https://example.com/other", "other", "article")

	results, err := search.RunQuery(ctx, s, np, userID, search.Query{
		Domains: []string{"x.com"}, Scope: search.ScopeLibrary,
	}, 10)
	if err != nil {
		t.Fatalf("domains-only: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("domains-only results = %d, want 2", len(results))
	}
	// ListItemsMatching is newest-first; scores stay zero (no rank signal).
	if results[0].Item.ID != newer.ID || results[1].Item.ID != older.ID {
		t.Fatalf("order = [%v %v], want newer %v before older %v",
			results[0].Item.ID, results[1].Item.ID, newer.ID, older.ID)
	}
	if results[0].Score != 0 || results[1].Score != 0 {
		t.Errorf("filter-only scores = [%v %v], want unscored 0", results[0].Score, results[1].Score)
	}

	// Library scope still excludes unkept feed items on the list path.
	feed, err := s.Queries.CreateFeed(ctx, db.CreateFeedParams{UserID: userID, Url: "https://x.com/feed.xml", Title: "x feed"})
	if err != nil {
		t.Fatalf("create feed: %v", err)
	}
	river, err := s.Queries.CreateFeedItem(ctx, db.CreateFeedItemParams{
		UserID: userID, Url: "https://x.com/unkept", FeedID: pgtype.UUID{Bytes: feed.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("create feed item: %v", err)
	}
	lib, err := search.RunQuery(ctx, s, np, userID, search.Query{
		Domains: []string{"x.com"}, Scope: search.ScopeLibrary,
	}, 10)
	if err != nil {
		t.Fatalf("domains library: %v", err)
	}
	if containsID(lib, river.ID) {
		t.Fatalf("library domains included unkept feed item %v", river.ID)
	}
}
