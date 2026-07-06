package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// getLinks fetches the linked items for id and decodes them into a list of ids.
func getLinks(t *testing.T, baseURL, id string) []string {
	t.Helper()
	resp, err := http.Get(baseURL + "/items/" + id + "/links")
	if err != nil {
		t.Fatalf("get links: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var items []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatalf("decode links: %v", err)
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it["id"].(string))
	}
	return ids
}

// createLink POSTs a link from id to toID and returns the response.
func createLink(t *testing.T, baseURL, id, toID string) *http.Response {
	t.Helper()
	return postJSON(t, baseURL+"/items/"+id+"/links", `{"toId":"`+toID+`"}`)
}

func deleteLink(t *testing.T, baseURL, id, toID string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, baseURL+"/items/"+id+"/links/"+toID, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	return resp
}

func containsID(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func TestLinkCreateIsBidirectional(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	a := createNoteItem(t, srv.URL, "item a")
	b := createNoteItem(t, srv.URL, "item b")

	resp := createLink(t, srv.URL, a, b)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var created []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(created) != 1 || created[0]["id"] != b {
		t.Errorf("create response = %v, want [%s]", created, b)
	}

	bLinks := getLinks(t, srv.URL, b)
	if !containsID(bLinks, a) {
		t.Errorf("b's links = %v, want to contain a (%s)", bLinks, a)
	}
}

func TestLinkCreateIdempotentBothDirections(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	a := createNoteItem(t, srv.URL, "item a")
	b := createNoteItem(t, srv.URL, "item b")

	resp1 := createLink(t, srv.URL, a, b)
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first link status = %d, want 201", resp1.StatusCode)
	}

	// Re-linking A->B and B->A must both succeed idempotently and never
	// produce a second row.
	resp2 := createLink(t, srv.URL, a, b)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Errorf("re-link a->b status = %d, want 201", resp2.StatusCode)
	}
	resp3 := createLink(t, srv.URL, b, a)
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusCreated {
		t.Errorf("re-link b->a status = %d, want 201", resp3.StatusCode)
	}

	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM links`).Scan(&count); err != nil {
		t.Fatalf("counting links: %v", err)
	}
	if count != 1 {
		t.Errorf("links row count = %d, want 1", count)
	}
}

func TestLinkCreateSelfLinkRejected(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	a := createNoteItem(t, srv.URL, "item a")

	resp := createLink(t, srv.URL, a, a)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("self-link status = %d, want 400", resp.StatusCode)
	}
}

func TestLinkCreateToOtherUsersItemNotFound(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	a := createNoteItem(t, srv.URL, "item a")
	otherID := seedOtherUserItem(t, s, "not yours")

	resp := createLink(t, srv.URL, a, otherID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestLinkCreateFromMissingItemNotFound(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	b := createNoteItem(t, srv.URL, "item b")

	resp := createLink(t, srv.URL, "11111111-1111-1111-1111-111111111111", b)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestLinkListOnMissingItemNotFound(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/items/11111111-1111-1111-1111-111111111111/links")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestLinkListOnCrossTenantItemNotFound(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	otherID := seedOtherUserItem(t, s, "not yours")

	resp, err := http.Get(srv.URL + "/items/" + otherID + "/links")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestLinkDeleteThenRedeleteNotFound(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	a := createNoteItem(t, srv.URL, "item a")
	b := createNoteItem(t, srv.URL, "item b")
	createLink(t, srv.URL, a, b).Body.Close()

	resp := deleteLink(t, srv.URL, a, b)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", resp.StatusCode)
	}

	aLinks := getLinks(t, srv.URL, a)
	if containsID(aLinks, b) {
		t.Errorf("a's links = %v, want not to contain b after delete", aLinks)
	}

	resp2 := deleteLink(t, srv.URL, a, b)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("re-delete status = %d, want 404", resp2.StatusCode)
	}
}

func TestLinkDeleteCascadesOnItemDeletion(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	a := createNoteItem(t, srv.URL, "item a")
	b := createNoteItem(t, srv.URL, "item b")
	createLink(t, srv.URL, a, b).Body.Close()

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/items/"+a, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	del, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete item: %v", err)
	}
	del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete item status = %d, want 204", del.StatusCode)
	}

	// B's link list must be empty (no dangling row, no FK error surfacing).
	bLinks := getLinks(t, srv.URL, b)
	if len(bLinks) != 0 {
		t.Errorf("b's links after a deleted = %v, want empty", bLinks)
	}
}

func TestLinkCreateMalformedBody(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	a := createNoteItem(t, srv.URL, "item a")

	resp := postJSON(t, srv.URL+"/items/"+a+"/links", `{not json`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
