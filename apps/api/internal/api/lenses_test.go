package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// lensResp mirrors the API Lens shape for decoding in tests.
type lensResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Rule struct {
		Q     string   `json:"q"`
		Color string   `json:"color"`
		Types []string `json:"types"`
	} `json:"rule"`
	CreatedAt      string  `json:"createdAt"`
	DigestSchedule string  `json:"digestSchedule"`
	LastDigestAt   *string `json:"lastDigestAt"`
}

func doJSON(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("content-type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func TestLensCRUD(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	// Create.
	resp := postJSON(t, srv.URL+"/lenses", `{"name":"Running gear","rule":{"q":"running shoes","types":["product"]}}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created lensResp
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	resp.Body.Close()
	if created.ID == "" || created.Name != "Running gear" {
		t.Fatalf("unexpected created lens: %+v", created)
	}
	if created.Rule.Q != "running shoes" || len(created.Rule.Types) != 1 || created.Rule.Types[0] != "product" {
		t.Errorf("rule not persisted: %+v", created.Rule)
	}

	// List returns it.
	resp = doJSON(t, http.MethodGet, srv.URL+"/lenses", "")
	var list []lensResp
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v, want the one created lens", list)
	}

	// Get by id.
	resp = doJSON(t, http.MethodGet, srv.URL+"/lenses/"+created.ID, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Update (rename + re-rule).
	resp = doJSON(t, http.MethodPatch, srv.URL+"/lenses/"+created.ID, `{"name":"Trail gear","rule":{"color":"terracotta"}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, want 200", resp.StatusCode)
	}
	var updated lensResp
	json.NewDecoder(resp.Body).Decode(&updated)
	resp.Body.Close()
	if updated.Name != "Trail gear" || updated.Rule.Color != "terracotta" || updated.Rule.Q != "" {
		t.Errorf("update not applied: %+v", updated)
	}

	// Delete then confirm 404 on subsequent get + delete.
	resp = doJSON(t, http.MethodDelete, srv.URL+"/lenses/"+created.ID, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, srv.URL+"/lenses/"+created.ID, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("get after delete = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
	resp = doJSON(t, http.MethodDelete, srv.URL+"/lenses/"+created.ID, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete after delete = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestLensCreateValidation(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	for _, body := range []string{
		`{"name":"","rule":{"q":"x"}}`,                           // empty name
		`{"name":"Empty","rule":{}}`,                             // empty rule
		`{"name":"Bad colour","rule":{"color":"chartreuseish"}}`, // unknown colour
		`{"name":"Bad type","rule":{"types":["gizmo"]}}`,         // unknown type
	} {
		resp := postJSON(t, srv.URL+"/lenses", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// TestLensCreateWithDigestSchedule verifies a digestSchedule supplied at
// creation time is validated, persisted, and echoed immediately (not just
// settable via a subsequent PATCH).
func TestLensCreateWithDigestSchedule(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/lenses", `{"name":"Weekly at create","rule":{"q":"news"},"digestSchedule":"weekly:4"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created lensResp
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	resp.Body.Close()
	if created.DigestSchedule != "weekly:4" {
		t.Errorf("digestSchedule echo = %q, want weekly:4", created.DigestSchedule)
	}

	got := doJSON(t, http.MethodGet, srv.URL+"/lenses/"+created.ID, "")
	var refetched lensResp
	if err := json.NewDecoder(got.Body).Decode(&refetched); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	got.Body.Close()
	if refetched.DigestSchedule != "weekly:4" {
		t.Errorf("persisted digestSchedule = %q, want weekly:4", refetched.DigestSchedule)
	}
}

// TestLensUpdateDigestSchedule covers valid weekly schedule persistence,
// rejection of an out-of-range weekday and an unrecognised schedule keyword,
// and clearing back to disabled via an explicit empty string.
func TestLensUpdateDigestSchedule(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	created := postJSON(t, srv.URL+"/lenses", `{"name":"Weekly digest","rule":{"q":"news"}}`)
	var lens lensResp
	if err := json.NewDecoder(created.Body).Decode(&lens); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	created.Body.Close()
	if lens.DigestSchedule != "" {
		t.Errorf("default digestSchedule = %q, want empty", lens.DigestSchedule)
	}

	resp := doJSON(t, http.MethodPatch, srv.URL+"/lenses/"+lens.ID, `{"name":"Weekly digest","rule":{"q":"news"},"digestSchedule":"weekly:3"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch weekly:3 status = %d, want 200", resp.StatusCode)
	}
	var updated lensResp
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	resp.Body.Close()
	if updated.DigestSchedule != "weekly:3" {
		t.Errorf("digestSchedule echo = %q, want weekly:3", updated.DigestSchedule)
	}

	got := doJSON(t, http.MethodGet, srv.URL+"/lenses/"+lens.ID, "")
	var refetched lensResp
	if err := json.NewDecoder(got.Body).Decode(&refetched); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	got.Body.Close()
	if refetched.DigestSchedule != "weekly:3" {
		t.Errorf("persisted digestSchedule = %q, want weekly:3", refetched.DigestSchedule)
	}

	for _, bad := range []string{"weekly:7", "hourly", "weekly:"} {
		badResp := doJSON(t, http.MethodPatch, srv.URL+"/lenses/"+lens.ID, `{"name":"Weekly digest","rule":{"q":"news"},"digestSchedule":"`+bad+`"}`)
		if badResp.StatusCode != http.StatusBadRequest {
			t.Errorf("digestSchedule %q status = %d, want 400", bad, badResp.StatusCode)
		}
		badResp.Body.Close()
	}

	cleared := doJSON(t, http.MethodPatch, srv.URL+"/lenses/"+lens.ID, `{"name":"Weekly digest","rule":{"q":"news"},"digestSchedule":""}`)
	if cleared.StatusCode != http.StatusOK {
		t.Fatalf("clear status = %d, want 200", cleared.StatusCode)
	}
	var clearedLens lensResp
	if err := json.NewDecoder(cleared.Body).Decode(&clearedLens); err != nil {
		t.Fatalf("decode clear: %v", err)
	}
	cleared.Body.Close()
	if clearedLens.DigestSchedule != "" {
		t.Errorf("cleared digestSchedule = %q, want empty", clearedLens.DigestSchedule)
	}
}

// TestLensItemsIsLiveView verifies a Lens applies its type filter as a live
// view: an item saved after the Lens exists still appears through it.
func TestLensItemsIsLiveView(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)
	ctx := context.Background()

	// A note item (matches) and an article item (does not) both under dev user.
	note, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: api.DevUserID, Body: "a passing thought"})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := s.Queries.SetItemStatus(ctx, db.SetItemStatusParams{UserID: api.DevUserID, ID: note.ID, Status: "enriched"}); err != nil {
		t.Fatalf("set status: %v", err)
	}
	art, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: api.DevUserID, Url: "https://example.com"})
	if err != nil {
		t.Fatalf("create article: %v", err)
	}
	// note defaults to card_type 'article'; make card types explicit.
	if _, err := s.Pool.Exec(ctx, `UPDATE items SET card_type='note' WHERE id=$1`, note.ID); err != nil {
		t.Fatalf("set note type: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE items SET card_type='article' WHERE id=$1`, art.ID); err != nil {
		t.Fatalf("set article type: %v", err)
	}

	resp := postJSON(t, srv.URL+"/lenses", `{"name":"Notes","rule":{"types":["note"]}}`)
	var lens lensResp
	json.NewDecoder(resp.Body).Decode(&lens)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/lenses/"+lens.ID+"/items", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("items status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Results []struct {
			Item struct {
				Id       string `json:"id"`
				CardType string `json:"cardType"`
			} `json:"item"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	resp.Body.Close()
	if len(out.Results) != 1 {
		t.Fatalf("results = %d, want 1 (only the note)", len(out.Results))
	}
	if out.Results[0].Item.Id != note.ID.String() {
		t.Errorf("result id = %s, want note %s", out.Results[0].Item.Id, note.ID)
	}

	// Items for an unknown lens id → 404.
	resp = doJSON(t, http.MethodGet, srv.URL+"/lenses/"+api.DevUserID.String()+"/items", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("items for unknown lens = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestLensTypesOnlyExcludesUnkeptFeedItem proves Lenses default to library
// scope: unkept feed-river items are absent from lens matches. GET /items
// (the Mind) still excludes the same unkept feed item.
func TestLensTypesOnlyExcludesUnkeptFeedItem(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)
	ctx := context.Background()

	feed := seedFeed(t, s, api.DevUserID, "https://example.com/rss.xml")
	feedItem := seedFeedItem(t, s, api.DevUserID, feed.ID, "https://example.com/unkept-post")
	if _, err := s.Pool.Exec(ctx, `UPDATE items SET card_type='article' WHERE id=$1`, feedItem.ID); err != nil {
		t.Fatalf("set feed item type: %v", err)
	}

	resp := postJSON(t, srv.URL+"/lenses", `{"name":"Articles","rule":{"types":["article"]}}`)
	var lens lensResp
	json.NewDecoder(resp.Body).Decode(&lens)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/lenses/"+lens.ID+"/items", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("items status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Results []struct {
			Item struct {
				Id string `json:"id"`
			} `json:"item"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	resp.Body.Close()
	for _, r := range out.Results {
		if r.Item.Id == feedItem.ID.String() {
			t.Fatalf("lens results included unkept feed item %s, want excluded (library default)", feedItem.ID)
		}
	}

	// GET /items (the Mind) must still exclude the unkept feed item.
	itemsResp := doJSON(t, http.MethodGet, srv.URL+"/items", "")
	if itemsResp.StatusCode != http.StatusOK {
		t.Fatalf("items status = %d, want 200", itemsResp.StatusCode)
	}
	var homeItems []struct {
		Id string `json:"id"`
	}
	if err := json.NewDecoder(itemsResp.Body).Decode(&homeItems); err != nil {
		t.Fatalf("decode home items: %v", err)
	}
	itemsResp.Body.Close()
	for _, r := range homeItems {
		if r.Id == feedItem.ID.String() {
			t.Fatalf("GET /items included unkept feed item %s, want excluded", feedItem.ID)
		}
	}
}

// TestLensScopeAllIncludesUnkeptFeedItem proves an explicit scope:all Lens
// still surfaces unkept feed-river items.
func TestLensScopeAllIncludesUnkeptFeedItem(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)
	ctx := context.Background()

	feed := seedFeed(t, s, api.DevUserID, "https://example.com/rss-scope-all.xml")
	feedItem := seedFeedItem(t, s, api.DevUserID, feed.ID, "https://example.com/unkept-scope-all")
	if _, err := s.Pool.Exec(ctx, `UPDATE items SET card_type='article' WHERE id=$1`, feedItem.ID); err != nil {
		t.Fatalf("set feed item type: %v", err)
	}

	resp := postJSON(t, srv.URL+"/lenses", `{"name":"All articles","rule":{"types":["article"],"scope":"all"}}`)
	var lens lensResp
	json.NewDecoder(resp.Body).Decode(&lens)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/lenses/"+lens.ID+"/items", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("items status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Results []struct {
			Item struct {
				Id string `json:"id"`
			} `json:"item"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	resp.Body.Close()
	found := false
	for _, r := range out.Results {
		if r.Item.Id == feedItem.ID.String() {
			found = true
		}
	}
	if !found {
		t.Fatalf("lens results = %+v, want unkept feed item %s included under scope:all", out.Results, feedItem.ID)
	}
}

// TestLensDomainFilter verifies a domains-only Lens returns Mind items whose
// URL host matches (and excludes other hosts).
func TestLensDomainFilter(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)
	ctx := context.Background()

	xItem, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: api.DevUserID, Url: "https://x.com/status/1"})
	if err != nil {
		t.Fatalf("create x.com item: %v", err)
	}
	exItem, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: api.DevUserID, Url: "https://example.com/post"})
	if err != nil {
		t.Fatalf("create example.com item: %v", err)
	}

	resp := postJSON(t, srv.URL+"/lenses", `{"name":"X only","rule":{"domains":["x.com"]}}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create lens status = %d, want 201", resp.StatusCode)
	}
	var lens lensResp
	json.NewDecoder(resp.Body).Decode(&lens)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/lenses/"+lens.ID+"/items", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("items status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Results []struct {
			Item struct {
				Id string `json:"id"`
			} `json:"item"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	resp.Body.Close()

	foundX, foundEx := false, false
	for _, r := range out.Results {
		if r.Item.Id == xItem.ID.String() {
			foundX = true
		}
		if r.Item.Id == exItem.ID.String() {
			foundEx = true
		}
	}
	if !foundX {
		t.Fatalf("lens results = %+v, want x.com item %s", out.Results, xItem.ID)
	}
	if foundEx {
		t.Fatalf("lens results included example.com item %s, want only x.com", exItem.ID)
	}
}
