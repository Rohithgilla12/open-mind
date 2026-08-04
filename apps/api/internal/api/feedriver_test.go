package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// pgtypeUUID converts a uuid.UUID into a valid pgtype.UUID.
func pgtypeUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// pgtypeNow returns a valid pgtype.Timestamptz set to the current time.
func pgtypeNow() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now(), Valid: true}
}

// seedFeed creates a feed subscription row directly via the store, bypassing
// the RSS-fetching CreateFeed handler so feed-item tests don't need a live
// feed server.
func seedFeed(t *testing.T, s *store.Store, uid uuid.UUID, url string) db.Feed {
	t.Helper()
	feed, err := s.Queries.CreateFeed(context.Background(), db.CreateFeedParams{
		UserID: uid, Url: url, Title: "Test Feed", SiteUrl: "https://example.com",
	})
	if err != nil {
		t.Fatalf("create feed: %v", err)
	}
	return feed
}

// seedFeedItem creates a feed-sourced item directly via the store.
func seedFeedItem(t *testing.T, s *store.Store, uid uuid.UUID, feedID uuid.UUID, itemURL string) db.Item {
	t.Helper()
	item, err := s.Queries.CreateFeedItem(context.Background(), db.CreateFeedItemParams{
		UserID: uid, Url: itemURL, FeedID: pgtypeUUID(feedID),
	})
	if err != nil {
		t.Fatalf("create feed item: %v", err)
	}
	return item
}

func getFeed(t *testing.T, url string) []map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("feed status = %d, want 200", resp.StatusCode)
	}
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode feed: %v", err)
	}
	return page.Items
}

func TestFeedItemsListing(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)

	feedA := seedFeed(t, s, api.DevUserID, "https://a.example.com/feed")
	feedB := seedFeed(t, s, api.DevUserID, "https://b.example.com/feed")

	a1 := seedFeedItem(t, s, api.DevUserID, feedA.ID, "https://a.example.com/1")
	a2 := seedFeedItem(t, s, api.DevUserID, feedA.ID, "https://a.example.com/2")
	b1 := seedFeedItem(t, s, api.DevUserID, feedB.ID, "https://b.example.com/1")

	// All items, newest first.
	all := getFeed(t, srv.URL+"/feed")
	if len(all) != 3 {
		t.Fatalf("feed items = %d, want 3", len(all))
	}
	if all[0]["id"] != b1.ID.String() || all[1]["id"] != a2.ID.String() || all[2]["id"] != a1.ID.String() {
		t.Errorf("feed order = %v %v %v, want newest-first b1,a2,a1", all[0]["id"], all[1]["id"], all[2]["id"])
	}

	// Filtered by feedId.
	filtered := getFeed(t, srv.URL+"/feed?feedId="+feedA.ID.String())
	if len(filtered) != 2 {
		t.Fatalf("filtered feed items = %d, want 2", len(filtered))
	}
	for _, it := range filtered {
		if it["feedId"] != feedA.ID.String() {
			t.Errorf("filtered item feedId = %v, want %v", it["feedId"], feedA.ID)
		}
	}

	// Another user sees none.
	other := uuid.MustParse("00000000-0000-0000-0000-0000000000ff")
	if err := s.Queries.EnsureUser(context.Background(), other); err != nil {
		t.Fatalf("ensure other user: %v", err)
	}
	otherFeed := seedFeed(t, s, other, "https://other.example.com/feed")
	seedFeedItem(t, s, other, otherFeed.ID, "https://other.example.com/1")

	// Dev user's view is unaffected by the other user's feed items.
	all = getFeed(t, srv.URL+"/feed")
	if len(all) != 3 {
		t.Fatalf("feed items after other user seed = %d, want 3 (other user's items excluded)", len(all))
	}
}

// TestFeedItemsFilterCrossTenant verifies GET /feed?feedId=<id> scopes by the
// caller's own feed items even when the id belongs to another user's feed:
// it must return an empty result, never the other user's items.
func TestFeedItemsFilterCrossTenant(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)

	other := uuid.MustParse("00000000-0000-0000-0000-0000000000fe")
	if err := s.Queries.EnsureUser(context.Background(), other); err != nil {
		t.Fatalf("ensure other user: %v", err)
	}
	otherFeed := seedFeed(t, s, other, "https://other-tenant.example.com/feed")
	seedFeedItem(t, s, other, otherFeed.ID, "https://other-tenant.example.com/1")

	// Dev user has feed items of their own, but none on otherFeed.
	devFeed := seedFeed(t, s, api.DevUserID, "https://own-tenant.example.com/feed")
	seedFeedItem(t, s, api.DevUserID, devFeed.ID, "https://own-tenant.example.com/1")

	filtered := getFeed(t, srv.URL+"/feed?feedId="+otherFeed.ID.String())
	if len(filtered) != 0 {
		t.Fatalf("filtered feed items for another user's feed id = %d, want 0", len(filtered))
	}
}

func TestFeedItemsLimitCap(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	feed := seedFeed(t, s, api.DevUserID, "https://c.example.com/feed")
	for i := 0; i < 3; i++ {
		seedFeedItem(t, s, api.DevUserID, feed.ID, "https://c.example.com/"+uuid.NewString())
	}

	// limit above the cap is clamped.
	resp, err := http.Get(srv.URL + "/feed?limit=99999")
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// limit=1 returns exactly one item (default is 50, so this proves limit is honored).
	limited := getFeed(t, srv.URL+"/feed?limit=1")
	if len(limited) != 1 {
		t.Fatalf("limited feed items = %d, want 1", len(limited))
	}
}

func TestFeedItemsExcludedFromHome(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	feed := seedFeed(t, s, api.DevUserID, "https://d.example.com/feed")
	unkept := seedFeedItem(t, s, api.DevUserID, feed.ID, "https://d.example.com/1")

	items := listItems(t, srv.URL)
	for _, it := range items {
		if it["id"] == unkept.ID.String() {
			t.Fatalf("unkept feed item %s appeared in home /items", unkept.ID)
		}
	}

	// Kept feed item appears in home.
	patchJSON(t, srv.URL+"/items/"+unkept.ID.String(), `{"kept":true}`).Body.Close()
	items = listItems(t, srv.URL)
	found := false
	for _, it := range items {
		if it["id"] == unkept.ID.String() {
			found = true
		}
	}
	if !found {
		t.Fatalf("kept feed item %s did not appear in home /items", unkept.ID)
	}
}

// listItems calls GET /items and decodes the ItemPage envelope's items.
func listItems(t *testing.T, baseURL string) []map[string]any {
	t.Helper()
	resp, err := http.Get(baseURL + "/items")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	defer resp.Body.Close()
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	return page.Items
}

func TestSaveExistingFeedURLPromotes(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	feed := seedFeed(t, s, api.DevUserID, "https://e.example.com/feed")

	// Unkept feed item: POST /items with the same URL promotes it rather than
	// inserting a duplicate.
	unkeptURL := "https://e.example.com/unkept"
	unkept := seedFeedItem(t, s, api.DevUserID, feed.ID, unkeptURL)

	resp := postJSON(t, srv.URL+"/items", `{"url":"`+unkeptURL+`"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var created struct {
		ID     string  `json:"id"`
		KeptAt *string `json:"keptAt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID != unkept.ID.String() {
		t.Errorf("id = %v, want existing id %v (promotion, not a new row)", created.ID, unkept.ID)
	}
	if created.KeptAt == nil {
		t.Error("keptAt = null, want a timestamp after promotion")
	}
	afterList, err := s.Queries.ListItems(context.Background(), db.ListItemsParams{UserID: api.DevUserID, LimitCount: 100})
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(afterList) != 1 {
		t.Errorf("item count = %d, want 1 (no duplicate row)", len(afterList))
	}

	// A brand-new URL still gets a fresh row (today's behaviour).
	newResp := postJSON(t, srv.URL+"/items", `{"url":"https://e.example.com/brand-new"}`)
	defer newResp.Body.Close()
	if newResp.StatusCode != http.StatusCreated {
		t.Fatalf("new url status = %d, want 201", newResp.StatusCode)
	}
	var newCreated struct{ ID string }
	if err := json.NewDecoder(newResp.Body).Decode(&newCreated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if newCreated.ID == unkept.ID.String() {
		t.Error("new url reused the existing feed item id")
	}

	// A KEPT feed item's URL is not re-promoted: POST /items inserts a fresh row.
	keptURL := "https://e.example.com/kept"
	kept := seedFeedItem(t, s, api.DevUserID, feed.ID, keptURL)
	if _, err := s.Queries.SetItemKept(context.Background(), db.SetItemKeptParams{
		UserID: api.DevUserID, ID: kept.ID, KeptAt: pgtypeNow(),
	}); err != nil {
		t.Fatalf("set kept: %v", err)
	}
	keptResp := postJSON(t, srv.URL+"/items", `{"url":"`+keptURL+`"}`)
	defer keptResp.Body.Close()
	if keptResp.StatusCode != http.StatusCreated {
		t.Fatalf("kept url status = %d, want 201", keptResp.StatusCode)
	}
	var keptCreated struct{ ID string }
	if err := json.NewDecoder(keptResp.Body).Decode(&keptCreated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if keptCreated.ID == kept.ID.String() {
		t.Error("already-kept feed item was reused instead of inserting a fresh row")
	}
}

func TestUnsubscribeKeepsItems(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	feed := seedFeed(t, s, api.DevUserID, "https://f.example.com/feed")

	unkept1 := seedFeedItem(t, s, api.DevUserID, feed.ID, "https://f.example.com/1")
	unkept2 := seedFeedItem(t, s, api.DevUserID, feed.ID, "https://f.example.com/2")
	kept := seedFeedItem(t, s, api.DevUserID, feed.ID, "https://f.example.com/3")
	if _, err := s.Queries.SetItemKept(context.Background(), db.SetItemKeptParams{
		UserID: api.DevUserID, ID: kept.ID, KeptAt: pgtypeNow(),
	}); err != nil {
		t.Fatalf("set kept: %v", err)
	}

	del := doReq(t, http.MethodDelete, srv.URL+"/feeds/"+feed.ID.String(), "")
	del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete feed status = %d, want 204", del.StatusCode)
	}

	items := listItems(t, srv.URL)
	want := map[string]bool{unkept1.ID.String(): false, unkept2.ID.String(): false, kept.ID.String(): false}
	for _, it := range items {
		if id, ok := it["id"].(string); ok {
			if _, tracked := want[id]; tracked {
				want[id] = true
			}
		}
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("item %s missing from home /items after unsubscribe", id)
		}
	}

	// All three now carry a kept_at timestamp, and feed_id has been nulled by
	// the FK ON DELETE SET NULL.
	for _, id := range []uuid.UUID{unkept1.ID, unkept2.ID, kept.ID} {
		item, err := s.Queries.GetItem(context.Background(), db.GetItemParams{UserID: api.DevUserID, ID: id})
		if err != nil {
			t.Fatalf("get item %s: %v", id, err)
		}
		if !item.KeptAt.Valid {
			t.Errorf("item %s kept_at not set after unsubscribe", id)
		}
		if item.FeedID.Valid {
			t.Errorf("item %s feed_id still set after feed delete", id)
		}
	}
}
