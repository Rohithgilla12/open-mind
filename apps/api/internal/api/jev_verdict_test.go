package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/jev"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

func insertCaptureDecision(t *testing.T, q *db.Queries, itemID uuid.UUID, action string, applied, suggested []string) {
	t.Helper()
	if applied == nil {
		applied = []string{}
	}
	if suggested == nil {
		suggested = []string{}
	}
	_, err := q.InsertJevDecision(context.Background(), db.InsertJevDecisionParams{
		UserID:        api.DevUserID,
		ItemID:        pgtype.UUID{Bytes: itemID, Valid: true},
		Surface:       jev.SurfaceCapture,
		Model:         jev.DefaultModel,
		QuestionsV:    jev.QuestionsVersion,
		Answers:       []byte(`{"tag:go":{"type":"noul","noul":0.7}}`),
		Action:        action,
		AppliedTags:   applied,
		SuggestedTags: suggested,
		DismissedTags: []string{},
	})
	if err != nil {
		t.Fatalf("insert decision: %v", err)
	}
}

func postVerdict(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return resp
}

func TestPostJevVerdictAcceptAndDismiss(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	id := createNoteItem(t, srv.URL, "suggest me")
	itemUUID := uuid.MustParse(id)
	insertCaptureDecision(t, s.Queries, itemUUID, string(jev.ActionSuggested), nil, []string{"go", "rust"})

	resp := postVerdict(t, srv.URL+"/items/"+id+"/jev-verdict", `{"action":"accept","tag":"go"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("accept status = %d", resp.StatusCode)
	}
	var detail struct {
		UserTags       []string `json:"userTags"`
		JevSuggestions *struct {
			SuggestedTags []string `json:"suggestedTags"`
			AppliedTags   []string `json:"appliedTags"`
			UserVerdict   *string  `json:"userVerdict"`
		} `json:"jevSuggestions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(detail.UserTags) != 1 || detail.UserTags[0] != "go" {
		t.Fatalf("userTags = %v, want [go]", detail.UserTags)
	}
	if detail.JevSuggestions == nil || detail.JevSuggestions.UserVerdict == nil || *detail.JevSuggestions.UserVerdict != "kept" {
		t.Fatalf("verdict = %+v, want kept", detail.JevSuggestions)
	}
	// rust still pending; go accepted so gone from suggestions
	if len(detail.JevSuggestions.SuggestedTags) != 1 || detail.JevSuggestions.SuggestedTags[0] != "rust" {
		t.Fatalf("suggested = %v, want [rust]", detail.JevSuggestions.SuggestedTags)
	}

	resp2 := postVerdict(t, srv.URL+"/items/"+id+"/jev-verdict", `{"action":"dismiss","tag":"rust"}`)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("dismiss status = %d", resp2.StatusCode)
	}
	var after struct {
		JevSuggestions *struct {
			SuggestedTags []string `json:"suggestedTags"`
			UserVerdict   *string  `json:"userVerdict"`
		} `json:"jevSuggestions"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&after); err != nil {
		t.Fatalf("decode2: %v", err)
	}
	if after.JevSuggestions == nil || len(after.JevSuggestions.SuggestedTags) != 0 {
		t.Fatalf("suggested after dismiss = %+v", after.JevSuggestions)
	}
	if after.JevSuggestions.UserVerdict == nil || *after.JevSuggestions.UserVerdict != "removed" {
		t.Fatalf("verdict after dismiss = %+v", after.JevSuggestions.UserVerdict)
	}
}

func TestPostJevVerdictUndoApplied(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	id := createNoteItem(t, srv.URL, "undo me")
	itemUUID := uuid.MustParse(id)
	if _, err := s.Queries.SetUserTags(context.Background(), db.SetUserTagsParams{
		UserID: api.DevUserID, ID: itemUUID, UserTags: []string{"go", "keep"},
	}); err != nil {
		t.Fatalf("set tags: %v", err)
	}
	insertCaptureDecision(t, s.Queries, itemUUID, string(jev.ActionApplied), []string{"go"}, nil)

	resp := postVerdict(t, srv.URL+"/items/"+id+"/jev-verdict", `{"action":"undo","tag":"go"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("undo status = %d", resp.StatusCode)
	}
	var detail struct {
		UserTags       []string `json:"userTags"`
		JevSuggestions *struct {
			AppliedTags []string `json:"appliedTags"`
			UserVerdict *string  `json:"userVerdict"`
		} `json:"jevSuggestions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(detail.UserTags) != 1 || detail.UserTags[0] != "keep" {
		t.Fatalf("userTags = %v, want [keep]", detail.UserTags)
	}
	if detail.JevSuggestions == nil || len(detail.JevSuggestions.AppliedTags) != 0 {
		t.Fatalf("applied after undo = %+v", detail.JevSuggestions)
	}
	if detail.JevSuggestions.UserVerdict == nil || *detail.JevSuggestions.UserVerdict != "removed" {
		t.Fatalf("verdict = %+v", detail.JevSuggestions.UserVerdict)
	}
}

func TestPostJevVerdictNoDecision(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	id := createNoteItem(t, srv.URL, "no jev")
	resp := postVerdict(t, srv.URL+"/items/"+id+"/jev-verdict", `{"action":"accept","tag":"go"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestGetItemIncludesJevSuggestions(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	id := createNoteItem(t, srv.URL, "chips")
	itemUUID := uuid.MustParse(id)
	insertCaptureDecision(t, s.Queries, itemUUID, string(jev.ActionSuggested), nil, []string{"go"})

	resp, err := http.Get(srv.URL + "/items/" + id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	var detail struct {
		JevSuggestions *struct {
			Action        string   `json:"action"`
			SuggestedTags []string `json:"suggestedTags"`
		} `json:"jevSuggestions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.JevSuggestions == nil || detail.JevSuggestions.Action != "suggested" {
		t.Fatalf("jevSuggestions = %+v", detail.JevSuggestions)
	}
	if len(detail.JevSuggestions.SuggestedTags) != 1 || detail.JevSuggestions.SuggestedTags[0] != "go" {
		t.Fatalf("suggested = %v", detail.JevSuggestions.SuggestedTags)
	}
}

func TestPatchItemBackfillsJevVerdictOnRemove(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)
	id := createNoteItem(t, srv.URL, "edit tags")
	itemUUID := uuid.MustParse(id)
	if _, err := s.Queries.SetUserTags(context.Background(), db.SetUserTagsParams{
		UserID: api.DevUserID, ID: itemUUID, UserTags: []string{"go"},
	}); err != nil {
		t.Fatalf("set tags: %v", err)
	}
	insertCaptureDecision(t, s.Queries, itemUUID, string(jev.ActionApplied), []string{"go"}, nil)

	resp := patchJSON(t, srv.URL+"/items/"+id, `{"userTags":[]}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	dec, err := s.Queries.GetJevCaptureDecision(context.Background(), db.GetJevCaptureDecisionParams{
		UserID: api.DevUserID, ItemID: pgtype.UUID{Bytes: itemUUID, Valid: true},
	})
	if err != nil {
		t.Fatalf("get decision: %v", err)
	}
	if !dec.UserVerdict.Valid || dec.UserVerdict.String != "removed" {
		t.Fatalf("user_verdict = %+v, want removed", dec.UserVerdict)
	}
}
