package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
