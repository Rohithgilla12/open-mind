package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	appmcp "github.com/rohithgilla12/openmind/api/internal/mcp"
	"github.com/rohithgilla12/openmind/api/internal/reelmedia"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// mcpTestDeps spins up a *Server backed by real Postgres for mcpBackend
// tests. It deliberately avoids a river client (no worker runs in these
// tests), so items stay at their post-create status unless a test moves them
// along explicitly (e.g. via SetItemStatus) for drift fixtures.
func mcpTestDeps(t *testing.T) (*Server, *store.Store) {
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
	if _, err := pool.Exec(ctx, `TRUNCATE items, item_embeddings, lenses, feeds, river_job, api_keys, device_links CASCADE`); err != nil {
		t.Fatalf("truncating: %v", err)
	}
	s := store.New(pool)
	if err := s.Queries.EnsureUser(ctx, DevUserID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	otherUID := uuid.MustParse("00000000-0000-0000-0000-0000000000ff")
	if err := s.Queries.EnsureUser(ctx, otherUID); err != nil {
		t.Fatalf("ensure other user: %v", err)
	}
	srv := &Server{store: s, provider: ai.NewNoop()}
	return srv, s
}

func createItem(t *testing.T, s *store.Store, uid uuid.UUID, note string) db.Item {
	t.Helper()
	item, err := s.Queries.CreateItem(context.Background(), db.CreateItemParams{UserID: uid, Body: note})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	return item
}

func TestMCPBackendSetUserTags(t *testing.T) {
	srv, s := mcpTestDeps(t)
	b := mcpBackend{s: srv}
	item := createItem(t, s, DevUserID, "tag me")

	got, err := b.SetUserTags(context.Background(), DevUserID, item.ID, []string{"  Mine ", "mine", "FAVE"})
	if err != nil {
		t.Fatalf("set user tags: %v", err)
	}
	want := []string{"mine", "fave"}
	if len(got.UserTags) != len(want) {
		t.Fatalf("user tags = %v, want %v", got.UserTags, want)
	}
	for i, tag := range want {
		if got.UserTags[i] != tag {
			t.Fatalf("user tags = %v, want %v", got.UserTags, want)
		}
	}

	otherUID := uuid.MustParse("00000000-0000-0000-0000-0000000000ff")
	if _, err := b.SetUserTags(context.Background(), otherUID, item.ID, []string{"x"}); !errors.Is(err, appmcp.ErrNotFound) {
		t.Fatalf("cross-tenant set tags err = %v, want ErrNotFound", err)
	}
}

func TestMCPBackendSetPinned(t *testing.T) {
	srv, s := mcpTestDeps(t)
	b := mcpBackend{s: srv}
	item := createItem(t, s, DevUserID, "pin me")

	got, err := b.SetPinned(context.Background(), DevUserID, item.ID, true)
	if err != nil {
		t.Fatalf("pin: %v", err)
	}
	if !got.PinnedAt.Valid {
		t.Fatalf("pinned_at not valid after pin")
	}

	got, err = b.SetPinned(context.Background(), DevUserID, item.ID, false)
	if err != nil {
		t.Fatalf("unpin: %v", err)
	}
	if got.PinnedAt.Valid {
		t.Fatalf("pinned_at still valid after unpin")
	}

	if _, err := b.SetPinned(context.Background(), DevUserID, uuid.New(), true); !errors.Is(err, appmcp.ErrNotFound) {
		t.Fatalf("unknown id err = %v, want ErrNotFound", err)
	}
}

func TestMCPBackendDeleteItem(t *testing.T) {
	srv, s := mcpTestDeps(t)
	b := mcpBackend{s: srv}
	item := createItem(t, s, DevUserID, "delete me")

	got, err := b.DeleteItem(context.Background(), DevUserID, item.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got.ID != item.ID {
		t.Fatalf("deleted item id = %v, want %v", got.ID, item.ID)
	}

	if _, err := s.Queries.GetItem(context.Background(), db.GetItemParams{UserID: DevUserID, ID: item.ID}); err == nil {
		t.Fatalf("item still present after delete")
	}

	if _, err := b.DeleteItem(context.Background(), DevUserID, item.ID); !errors.Is(err, appmcp.ErrNotFound) {
		t.Fatalf("second delete err = %v, want ErrNotFound", err)
	}
}

func TestMCPBackendLenses(t *testing.T) {
	srv, s := mcpTestDeps(t)
	b := mcpBackend{s: srv}

	lens, err := b.CreateLens(context.Background(), DevUserID, "Go", appmcp.LensRule{Q: "golang"})
	if err != nil {
		t.Fatalf("create lens: %v", err)
	}
	rule := decodeStoredRule(lens.Rule)
	if rule.q != "golang" {
		t.Fatalf("stored rule q = %q, want golang", rule.q)
	}

	if _, err := b.CreateLens(context.Background(), DevUserID, "Empty", appmcp.LensRule{}); err == nil {
		t.Fatalf("create lens with empty rule: want error, got nil")
	}

	deleted, err := b.DeleteLens(context.Background(), DevUserID, lens.ID)
	if err != nil {
		t.Fatalf("delete lens: %v", err)
	}
	if deleted.ID != lens.ID {
		t.Fatalf("deleted lens id = %v, want %v", deleted.ID, lens.ID)
	}
	if _, err := s.Queries.GetLens(context.Background(), db.GetLensParams{UserID: DevUserID, ID: lens.ID}); err == nil {
		t.Fatalf("lens still present after delete")
	}

	otherUID := uuid.MustParse("00000000-0000-0000-0000-0000000000ff")
	lens2, err := b.CreateLens(context.Background(), DevUserID, "Go2", appmcp.LensRule{Q: "golang"})
	if err != nil {
		t.Fatalf("create lens 2: %v", err)
	}
	if _, err := b.DeleteLens(context.Background(), otherUID, lens2.ID); !errors.Is(err, appmcp.ErrNotFound) {
		t.Fatalf("cross-tenant delete lens err = %v, want ErrNotFound", err)
	}
}

func TestMCPBackendDeskDrift(t *testing.T) {
	srv, s := mcpTestDeps(t)
	b := mcpBackend{s: srv}
	ctx := context.Background()

	pinned := createItem(t, s, DevUserID, "pinned item")
	if _, err := s.Queries.SetItemPinned(ctx, db.SetItemPinnedParams{UserID: DevUserID, ID: pinned.ID, PinnedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}); err != nil {
		t.Fatalf("pin item: %v", err)
	}

	desk, err := b.GetDesk(ctx, DevUserID)
	if err != nil {
		t.Fatalf("get desk: %v", err)
	}
	if len(desk) != 1 || desk[0].ID != pinned.ID {
		t.Fatalf("desk = %v, want [%v]", desk, pinned.ID)
	}

	driftCandidate := createItem(t, s, DevUserID, "drift item")
	if err := s.Queries.SetItemStatus(ctx, db.SetItemStatusParams{UserID: DevUserID, ID: driftCandidate.ID, Status: "enriched"}); err != nil {
		t.Fatalf("set status: %v", err)
	}

	items, total, err := b.GetDrift(ctx, DevUserID)
	if err != nil {
		t.Fatalf("get drift: %v", err)
	}
	if total != 1 {
		t.Fatalf("drift total = %d, want 1", total)
	}
	if len(items) != 1 || items[0].ID != driftCandidate.ID {
		t.Fatalf("drift items = %v, want [%v]", items, driftCandidate.ID)
	}

	after, err := s.Queries.GetItem(ctx, db.GetItemParams{UserID: DevUserID, ID: driftCandidate.ID})
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if after.LastDriftedAt.Valid {
		t.Fatalf("GetDrift must not set last_drifted_at, but it is now valid")
	}
}

func TestMCPBackendRelated(t *testing.T) {
	srv, s := mcpTestDeps(t)
	b := mcpBackend{s: srv}
	ctx := context.Background()

	src := createItem(t, s, DevUserID, "source")
	near := createItem(t, s, DevUserID, "near")

	dims := 768
	vec := func(components ...float32) string {
		parts := make([]string, dims)
		for i := range parts {
			if i < len(components) {
				parts[i] = fmt.Sprintf("%v", components[i])
			} else {
				parts[i] = "0"
			}
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	insertEmbedding := func(itemID uuid.UUID, vecLit string) {
		_, err := s.Pool.Exec(ctx, `INSERT INTO item_embeddings (item_id, user_id, embedding) VALUES ($1, $2, $3::vector)`, itemID, DevUserID, vecLit)
		if err != nil {
			t.Fatalf("seeding embedding: %v", err)
		}
	}
	insertEmbedding(src.ID, vec(1, 0))
	insertEmbedding(near.ID, vec(1, 0.05))

	results, err := b.Related(ctx, DevUserID, src.ID)
	if err != nil {
		t.Fatalf("related: %v", err)
	}
	if len(results) != 1 || results[0].Item.ID != near.ID {
		t.Fatalf("related = %+v, want [%v]", results, near.ID)
	}

	otherUID := uuid.MustParse("00000000-0000-0000-0000-0000000000ff")
	if _, err := b.Related(ctx, otherUID, src.ID); !errors.Is(err, appmcp.ErrNotFound) {
		t.Fatalf("cross-tenant related err = %v, want ErrNotFound", err)
	}
}

// TestNewMCPBackendCaptureAndRead exercises the HTTP-free backend built by
// NewMCPBackend directly: no *api.Server, no router, just store + an
// insert-only River client + provider — the shape the stdio command will use.
func TestNewMCPBackendCaptureAndRead(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `TRUNCATE items, item_embeddings, lenses, feeds, river_job, api_keys, device_links CASCADE`); err != nil {
		t.Fatalf("truncating: %v", err)
	}
	s := store.New(pool)
	if err := s.Queries.EnsureUser(ctx, DevUserID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	riverClient, err := jobs.NewRiverClient(pool, nil, nil, jobs.KindleDeps{}, nil, reelmedia.ModeThumbnail, nil, false)
	if err != nil {
		t.Fatalf("new river client: %v", err)
	}

	backend := NewMCPBackend(s, riverClient, ai.NewNoop())

	item, err := backend.Save(ctx, DevUserID, "", "stdio backend note")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if item.Status != "pending" {
		t.Fatalf("status = %q, want pending", item.Status)
	}

	got, err := backend.GetItem(ctx, DevUserID, item.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if got.ID != item.ID || got.Body != "stdio backend note" {
		t.Fatalf("get item = %+v, want body %q", got, "stdio backend note")
	}

	var jobCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind = 'enrich_item'`).Scan(&jobCount); err != nil {
		t.Fatalf("counting river jobs: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("river_job count = %d, want 1", jobCount)
	}
}
