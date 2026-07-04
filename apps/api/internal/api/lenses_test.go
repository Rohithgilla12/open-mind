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
	CreatedAt string `json:"createdAt"`
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
