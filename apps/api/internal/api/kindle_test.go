package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/ai"
)

// kindleErrorBody is the exact 409 payload the handlers must return when
// Send-to-Kindle is unconfigured.
const kindleUnconfiguredBody = "kindle is not configured — set SMTP_HOST, SMTP_FROM and KINDLE_EMAIL"

func decodeError(t *testing.T, resp *http.Response) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	return body.Error
}

func decodeQueued(t *testing.T, resp *http.Response) bool {
	t.Helper()
	var body struct {
		Queued bool `json:"queued"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode queued body: %v", err)
	}
	return body.Queued
}

func TestKindleItemUnconfiguredReturns409(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrvWithKindle(t, s, rc, "", ai.NewNoop(), false))
	t.Cleanup(srv.Close)

	id := createNoteItem(t, srv.URL, "send me")

	resp := doJSON(t, http.MethodPost, srv.URL+"/items/"+id+"/kindle", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != kindleUnconfiguredBody {
		t.Errorf("error = %q, want %q", got, kindleUnconfiguredBody)
	}
}

// TestKindleItemUnknownEvenUnconfiguredNotFound asserts ownership
// is checked before the configured gate: an unknown item is 404 regardless of
// whether Send-to-Kindle is configured.
func TestKindleItemUnknownEvenUnconfiguredNotFound(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrvWithKindle(t, s, rc, "", ai.NewNoop(), false))
	t.Cleanup(srv.Close)

	resp := doJSON(t, http.MethodPost, srv.URL+"/items/11111111-1111-1111-1111-111111111111/kindle", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestKindleItemUnknownNotFound(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrvWithKindle(t, s, rc, "", ai.NewNoop(), true))
	t.Cleanup(srv.Close)

	resp := doJSON(t, http.MethodPost, srv.URL+"/items/11111111-1111-1111-1111-111111111111/kindle", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestKindleItemEmptyBodyReturns422(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrvWithKindle(t, s, rc, "", ai.NewNoop(), true))
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/items", `{"url":"https://example.com/no-body-yet"}`)
	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	resp.Body.Close()
	id := created["id"].(string)

	kindleResp := doJSON(t, http.MethodPost, srv.URL+"/items/"+id+"/kindle", "")
	defer kindleResp.Body.Close()
	if kindleResp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", kindleResp.StatusCode)
	}
}

func TestKindleItemHappyPathQueuesJob(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := httptest.NewServer(newSrvWithKindle(t, s, rc, "", ai.NewNoop(), true))
	t.Cleanup(srv.Close)

	id := createNoteItem(t, srv.URL, "send this note")

	resp := doJSON(t, http.MethodPost, srv.URL+"/items/"+id+"/kindle", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	if !decodeQueued(t, resp) {
		t.Error("queued = false, want true")
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = 'send_kindle'`).Scan(&count); err != nil {
		t.Fatalf("counting jobs: %v", err)
	}
	if count != 1 {
		t.Errorf("send_kindle job rows = %d, want 1", count)
	}
}

func TestKindleLensUnconfiguredReturns409(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrvWithKindle(t, s, rc, "", ai.NewNoop(), false))
	t.Cleanup(srv.Close)

	lensResp := postJSON(t, srv.URL+"/lenses", `{"name":"Everything","rule":{"types":["note"]}}`)
	var lens struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(lensResp.Body).Decode(&lens); err != nil {
		t.Fatalf("decode lens: %v", err)
	}
	lensResp.Body.Close()

	resp := doJSON(t, http.MethodPost, srv.URL+"/lenses/"+lens.ID+"/kindle", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != kindleUnconfiguredBody {
		t.Errorf("error = %q, want %q", got, kindleUnconfiguredBody)
	}
}

func TestKindleLensUnknownNotFound(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrvWithKindle(t, s, rc, "", ai.NewNoop(), true))
	t.Cleanup(srv.Close)

	resp := doJSON(t, http.MethodPost, srv.URL+"/lenses/11111111-1111-1111-1111-111111111111/kindle", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestKindleLensNoMatchesReturns422(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrvWithKindle(t, s, rc, "", ai.NewNoop(), true))
	t.Cleanup(srv.Close)

	// A rule that matches nothing at all.
	lensResp := postJSON(t, srv.URL+"/lenses", `{"name":"Nothing here","rule":{"types":["recipe"]}}`)
	var lens struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(lensResp.Body).Decode(&lens); err != nil {
		t.Fatalf("decode lens: %v", err)
	}
	lensResp.Body.Close()

	resp := doJSON(t, http.MethodPost, srv.URL+"/lenses/"+lens.ID+"/kindle", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}

func TestKindleLensHappyPathQueuesJob(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := httptest.NewServer(newSrvWithKindle(t, s, rc, "", ai.NewNoop(), true))
	t.Cleanup(srv.Close)

	// A note item has body set at creation, but card_type only becomes
	// "note" once enrichment classifies it — no worker runs in this test, so
	// set it directly (same trick lenses_test.go uses for a live-view test).
	noteID := createNoteItem(t, srv.URL, "a note with a body")
	if _, err := pool.Exec(context.Background(), `UPDATE items SET card_type='note' WHERE id=$1`, noteID); err != nil {
		t.Fatalf("set note type: %v", err)
	}

	lensResp := postJSON(t, srv.URL+"/lenses", `{"name":"Notes","rule":{"types":["note"]}}`)
	var lens struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(lensResp.Body).Decode(&lens); err != nil {
		t.Fatalf("decode lens: %v", err)
	}
	lensResp.Body.Close()

	resp := doJSON(t, http.MethodPost, srv.URL+"/lenses/"+lens.ID+"/kindle", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	if !decodeQueued(t, resp) {
		t.Error("queued = false, want true")
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = 'send_kindle'`).Scan(&count); err != nil {
		t.Fatalf("counting jobs: %v", err)
	}
	if count != 1 {
		t.Errorf("send_kindle job rows = %d, want 1", count)
	}
}
