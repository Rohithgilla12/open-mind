package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rohithgilla12/openmind/api/internal/jev"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

func TestInsertJevDecisionAndTagVocabulary(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `TRUNCATE items, jev_decisions, user_settings CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: "https://example.com/a", Body: ""})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if _, err := s.Queries.SetUserTags(ctx, db.SetUserTagsParams{
		UserID: userID, ID: item.ID, UserTags: []string{"go", "postgres", "go"},
	}); err != nil {
		t.Fatalf("set tags: %v", err)
	}
	other, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: "https://example.com/b", Body: ""})
	if err != nil {
		t.Fatalf("create item2: %v", err)
	}
	if _, err := s.Queries.SetUserTags(ctx, db.SetUserTagsParams{
		UserID: userID, ID: other.ID, UserTags: []string{"search", "postgres"},
	}); err != nil {
		t.Fatalf("set tags2: %v", err)
	}

	vocab, err := s.Queries.ListUserTagVocabulary(ctx, userID)
	if err != nil {
		t.Fatalf("vocab: %v", err)
	}
	want := []string{"go", "postgres", "search"}
	if len(vocab) != len(want) {
		t.Fatalf("vocab = %v, want %v", vocab, want)
	}
	for i := range want {
		if vocab[i] != want[i] {
			t.Fatalf("vocab = %v, want %v", vocab, want)
		}
	}

	dec, err := s.Queries.InsertJevDecision(ctx, db.InsertJevDecisionParams{
		UserID:     userID,
		ItemID:     pgtype.UUID{Bytes: item.ID, Valid: true},
		Surface:    jev.SurfaceCapture,
		Model:      jev.DefaultModel,
		QuestionsV: jev.QuestionsVersion,
		Answers:    []byte(`{"is_evergreen":{"type":"noul","noul":0.9}}`),
		Action:     string(jev.ActionShadow),
		LatencyMs:  pgtype.Int4{Int32: 12, Valid: true},
		InputTokens: pgtype.Int4{Int32: 100, Valid: true},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if dec.Action != string(jev.ActionShadow) || dec.ID == 0 {
		t.Fatalf("bad decision: %+v", dec)
	}

	got, err := s.Queries.GetJevCaptureDecision(ctx, db.GetJevCaptureDecisionParams{
		UserID: userID, ItemID: pgtype.UUID{Bytes: item.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != dec.ID {
		t.Fatalf("get id = %d, want %d", got.ID, dec.ID)
	}

	// Cross-tenant: another user must not see this decision.
	otherUser := uuid.New()
	if err := s.Queries.EnsureUser(ctx, otherUser); err != nil {
		t.Fatalf("ensure other: %v", err)
	}
	_, err = s.Queries.GetJevCaptureDecision(ctx, db.GetJevCaptureDecisionParams{
		UserID: otherUser, ItemID: pgtype.UUID{Bytes: item.ID, Valid: true},
	})
	if err == nil {
		t.Fatal("other user must not see decision")
	}
}
