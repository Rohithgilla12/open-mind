package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://openmind:openmind@localhost:5433/openmind_test"
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	pool.Exec(ctx, `TRUNCATE items, item_embeddings CASCADE`)
	return store.New(pool)
}

func TestItemLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: "https://example.com", Body: ""})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.Status != "pending" {
		t.Errorf("status = %q, want pending", item.Status)
	}
	vec := make([]float32, 768)
	vec[0] = 1
	if err := s.Queries.UpsertEmbedding(ctx, db.UpsertEmbeddingParams{ItemID: item.ID, UserID: userID, Embedding: pgvector.NewVector(vec)}); err != nil {
		t.Fatalf("upsert embedding: %v", err)
	}
	otherUser := uuid.New()
	s.Queries.EnsureUser(ctx, otherUser)
	if _, err := s.Queries.GetItem(ctx, db.GetItemParams{UserID: otherUser, ID: item.ID}); err == nil {
		t.Error("cross-tenant read succeeded; want no rows")
	}
}
