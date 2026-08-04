package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rohithgilla12/openmind/api/internal/api"
)

// itemPage mirrors the ItemPage envelope for assertions.
type itemPage struct {
	Items      []map[string]any `json:"items"`
	NextCursor *string          `json:"nextCursor"`
}

func getPage(t *testing.T, url string) itemPage {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d for %s, want 200", resp.StatusCode, url)
	}
	var page itemPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return page
}

func TestListItemsPagesWithoutDuplicatesOrGaps(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	const total = 5
	for i := 0; i < total; i++ {
		postJSON(t, srv.URL+"/items", `{"url":"https://example.com/`+string(rune('a'+i))+`"}`).Body.Close()
		// Distinct created_at values keep the assertion about ordering honest.
		time.Sleep(2 * time.Millisecond)
	}

	seen := map[string]int{}
	url := srv.URL + "/items?limit=2"
	for i := 0; i < 10; i++ {
		page := getPage(t, url)
		// Guards against a missing trim: without it, over-fetching limit+1 rows
		// and handing them all back still has no duplicates or gaps (it just
		// front-loads one extra row per page), so that property alone can't
		// catch a dropped `rows = rows[:limit]`.
		if len(page.Items) > 2 {
			t.Errorf("page %d had %d items, want at most the requested limit of 2", i, len(page.Items))
		}
		for _, it := range page.Items {
			seen[it["id"].(string)]++
		}
		if page.NextCursor == nil {
			// Last page must not be empty: the limit+1 lookahead means a
			// nextCursor is only emitted when a further row really exists.
			if len(page.Items) == 0 && i > 0 {
				t.Error("final request returned an empty page; lookahead should have withheld the cursor")
			}
			break
		}
		url = srv.URL + "/items?limit=2&cursor=" + *page.NextCursor
	}

	if len(seen) != total {
		t.Errorf("saw %d distinct items, want %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("item %s served %d times, want 1", id, n)
		}
	}
}

func TestListItemsRejectsMalformedCursor(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/items?cursor=!!!not-a-cursor!!!")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (a bad cursor must not silently serve page 1)", resp.StatusCode)
	}
}

func TestFeedItemsPaginate(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	page := getPage(t, srv.URL+"/feed?limit=2")
	// No feed items seeded, so this asserts the shape rather than the contents:
	// an empty list must be [] with no cursor, never null.
	if page.Items == nil {
		t.Error("items was null; want an empty array")
	}
	if page.NextCursor != nil {
		t.Errorf("nextCursor = %q on an empty feed, want absent", *page.NextCursor)
	}

	resp, err := http.Get(srv.URL + "/feed?cursor=!!!bad!!!")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestFeedItemsFilterPagesWithoutDuplicatesOrGaps proves /feed?feedId=X pages
// correctly when a cursor and the feed filter are both in play. Nothing else
// in the repo exercises filter_feed_id together with a cursor, and two later
// tasks paginate /feed?feedId=X from the web and mobile clients — if the two
// predicates were ever mis-grouped in the WHERE clause (e.g. the feed filter
// ORed instead of ANDed with the keyset seek), a page could leak another
// feed's rows or drop one of its own, and the rest of the suite would stay
// green because it never pages a filtered feed at all.
func TestFeedItemsFilterPagesWithoutDuplicatesOrGaps(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)
	ctx := context.Background()

	feedA := seedFeed(t, s, api.DevUserID, "https://a.example.com/paginate.xml")
	feedB := seedFeed(t, s, api.DevUserID, "https://b.example.com/paginate.xml")

	// Interleave feedA and feedB rows in created_at order (A, B, A, B, ...) so
	// that a mis-grouped filter would surface a feedB row in the middle of a
	// feedA page rather than being hidden by every feedB row happening to sort
	// outside the pages under test. seedFeedItem's created_at defaults to the
	// wall clock, which under fast, back-to-back inserts is not guaranteed to
	// give the two feeds distinct, deterministically-ordered timestamps — so,
	// as internal/store/items_page_test.go does, pin created_at explicitly via
	// the store's exported Pool after each insert.
	base := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	const totalA = 5
	aIDs := map[string]bool{}
	for i := 0; i < totalA; i++ {
		a := seedFeedItem(t, s, api.DevUserID, feedA.ID, "https://a.example.com/paginate/"+string(rune('a'+i)))
		aIDs[a.ID.String()] = true
		if _, err := s.Pool.Exec(ctx, `UPDATE items SET created_at = $2 WHERE id = $1`,
			a.ID, base.Add(time.Duration(2*i)*time.Minute)); err != nil {
			t.Fatalf("backdate feedA item %d: %v", i, err)
		}

		b := seedFeedItem(t, s, api.DevUserID, feedB.ID, "https://b.example.com/paginate/"+string(rune('a'+i)))
		if _, err := s.Pool.Exec(ctx, `UPDATE items SET created_at = $2 WHERE id = $1`,
			b.ID, base.Add(time.Duration(2*i+1)*time.Minute)); err != nil {
			t.Fatalf("backdate feedB item %d: %v", i, err)
		}
	}

	seen := map[string]int{}
	url := srv.URL + "/feed?feedId=" + feedA.ID.String() + "&limit=2"
	for i := 0; i < 10; i++ {
		page := getPage(t, url)
		if len(page.Items) > 2 {
			t.Errorf("page %d had %d items, want at most the requested limit of 2", i, len(page.Items))
		}
		for _, it := range page.Items {
			id, _ := it["id"].(string)
			if !aIDs[id] {
				t.Fatalf("page %d included item %s, which is not a feedA item — feedId filter leaked across the cursor seek", i, id)
			}
			seen[id]++
		}
		if page.NextCursor == nil {
			// Last page must not be empty: the limit+1 lookahead means a
			// nextCursor is only emitted when a further row really exists.
			if len(page.Items) == 0 && i > 0 {
				t.Error("final request returned an empty page; lookahead should have withheld the cursor")
			}
			break
		}
		url = srv.URL + "/feed?feedId=" + feedA.ID.String() + "&limit=2&cursor=" + *page.NextCursor
	}

	if len(seen) != totalA {
		t.Errorf("saw %d distinct feedA items, want %d", len(seen), totalA)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("item %s served %d times, want 1", id, n)
		}
	}
}
