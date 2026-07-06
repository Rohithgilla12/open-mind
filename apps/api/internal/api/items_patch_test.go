package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/rohithgilla12/openmind/api/internal/store"
)

// newHTTPTest starts an httptest server backed by a no-auth Server and registers
// its cleanup.
func newHTTPTest(t *testing.T, s *store.Store, rc *river.Client[pgx.Tx]) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)
	return srv
}

// patchJSON sends a PATCH with a JSON body and returns the response.
func patchJSON(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	return resp
}

// createNoteItem saves a note item via the API and returns its id.
func createNoteItem(t *testing.T, baseURL, note string) string {
	t.Helper()
	resp := postJSON(t, baseURL+"/items", `{"note":"`+note+`"}`)
	defer resp.Body.Close()
	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	return created["id"].(string)
}

func TestPatchItemSetsCanonicalUserTags(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	id := createNoteItem(t, srv.URL, "tag me")

	resp := patchJSON(t, srv.URL+"/items/"+id, `{"userTags":["A","a"," b "]}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var detail struct {
		UserTags []string `json:"userTags"`
		Body     string   `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(detail.UserTags) != 2 || detail.UserTags[0] != "a" || detail.UserTags[1] != "b" {
		t.Errorf("userTags = %v, want [a b]", detail.UserTags)
	}
	if detail.Body != "tag me" {
		t.Errorf("body = %q, want it preserved", detail.Body)
	}
}

func TestPatchItemEmptyArrayClearsTags(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	id := createNoteItem(t, srv.URL, "clear me")

	// Set then clear.
	patchJSON(t, srv.URL+"/items/"+id, `{"userTags":["keep"]}`).Body.Close()

	resp := patchJSON(t, srv.URL+"/items/"+id, `{"userTags":[]}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var detail struct {
		UserTags []string `json:"userTags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.UserTags == nil {
		t.Fatal("userTags = null, want empty array")
	}
	if len(detail.UserTags) != 0 {
		t.Errorf("userTags = %v, want empty", detail.UserTags)
	}
}

func TestPatchItemPinSetsPinnedAt(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	id := createNoteItem(t, srv.URL, "pin me")

	resp := patchJSON(t, srv.URL+"/items/"+id, `{"pinned":true}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var detail struct {
		PinnedAt *string `json:"pinnedAt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.PinnedAt == nil {
		t.Fatal("pinnedAt = null, want a timestamp")
	}
}

func TestPatchItemUnpinClearsPinnedAt(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	id := createNoteItem(t, srv.URL, "unpin me")

	patchJSON(t, srv.URL+"/items/"+id, `{"pinned":true}`).Body.Close()

	resp := patchJSON(t, srv.URL+"/items/"+id, `{"pinned":false}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var detail struct {
		PinnedAt *string `json:"pinnedAt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.PinnedAt != nil {
		t.Errorf("pinnedAt = %v, want null", *detail.PinnedAt)
	}
}

func TestPatchItemPinAndUserTagsTogether(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	id := createNoteItem(t, srv.URL, "both")

	resp := patchJSON(t, srv.URL+"/items/"+id, `{"userTags":["keep"],"pinned":true}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var detail struct {
		UserTags []string `json:"userTags"`
		PinnedAt *string  `json:"pinnedAt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.PinnedAt == nil {
		t.Error("pinnedAt = null, want a timestamp")
	}
	if len(detail.UserTags) != 1 || detail.UserTags[0] != "keep" {
		t.Errorf("userTags = %v, want [keep]", detail.UserTags)
	}
}

func TestPatchItemPinCrossTenant(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	otherID := seedOtherUserItem(t, s, "not yours")

	resp := patchJSON(t, srv.URL+"/items/"+otherID, `{"pinned":true}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant status = %d, want 404", resp.StatusCode)
	}
}

func TestPatchItemNotFound(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)

	resp := patchJSON(t, srv.URL+"/items/11111111-1111-1111-1111-111111111111", `{"userTags":["x"]}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestPatchItemCrossTenant(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	otherID := seedOtherUserItem(t, s, "not yours")

	resp := patchJSON(t, srv.URL+"/items/"+otherID, `{"userTags":["x"]}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant status = %d, want 404", resp.StatusCode)
	}
}

func TestPatchItemMalformedBody(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	id := createNoteItem(t, srv.URL, "bad body")

	resp := patchJSON(t, srv.URL+"/items/"+id, `{not json`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPatchItemMissingUserTagsKey(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	id := createNoteItem(t, srv.URL, "no fields")

	resp := patchJSON(t, srv.URL+"/items/"+id, `{}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
