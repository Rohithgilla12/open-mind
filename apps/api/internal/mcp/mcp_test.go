package mcp_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appmcp "github.com/rohithgilla12/openmind/api/internal/mcp"
	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

type fakeBackend struct {
	saved    []db.Item
	items    []db.Item
	lenses   []db.Lense
	failLens bool
	notFound bool
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func (f *fakeBackend) Save(_ context.Context, _ uuid.UUID, url, note string) (db.Item, error) {
	it := db.Item{ID: uuid.New(), Url: url, Body: note, Status: "pending", CreatedAt: ts(time.Unix(0, 0))}
	f.saved = append(f.saved, it)
	return it, nil
}
func (f *fakeBackend) Search(_ context.Context, _ uuid.UUID, q, color string, parse bool) (appmcp.SearchOutcome, error) {
	rs := make([]search.Result, 0, len(f.items))
	for _, it := range f.items {
		rs = append(rs, search.Result{Item: it, Score: 1})
	}
	u := ""
	if parse {
		u = "text " + q
	}
	return appmcp.SearchOutcome{Results: rs, Understood: u}, nil
}
func (f *fakeBackend) ListRecent(_ context.Context, _ uuid.UUID, limit int) ([]db.Item, error) {
	if limit < len(f.items) {
		return f.items[:limit], nil
	}
	return f.items, nil
}
func (f *fakeBackend) GetItem(_ context.Context, _ uuid.UUID, id uuid.UUID) (db.Item, error) {
	if f.notFound {
		return db.Item{}, appmcp.ErrNotFound
	}
	return db.Item{ID: id, Url: "https://x", Status: "enriched", Body: "hello body", CreatedAt: ts(time.Unix(0, 0))}, nil
}
func (f *fakeBackend) ListLenses(_ context.Context, _ uuid.UUID) ([]db.Lense, error) {
	return f.lenses, nil
}
func (f *fakeBackend) RunLens(_ context.Context, _ uuid.UUID, id uuid.UUID) ([]search.Result, error) {
	if f.failLens {
		return nil, appmcp.ErrNotFound
	}
	return []search.Result{{Item: db.Item{ID: id, Url: "https://l", Status: "enriched", CreatedAt: ts(time.Unix(0, 0))}, Score: 2}}, nil
}

// connect spins the MCP handler on an httptest server and returns a connected
// client session that talks the real Streamable HTTP transport.
func connect(t *testing.T, b appmcp.Backend) *sdk.ClientSession {
	t.Helper()
	h := appmcp.NewHandler(b, func(context.Context) uuid.UUID { return uuid.New() })
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	sess, err := client.Connect(context.Background(), &sdk.StreamableClientTransport{Endpoint: srv.URL}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

func call(t *testing.T, sess *sdk.ClientSession, name string, args map[string]any) *sdk.CallToolResult {
	t.Helper()
	res, err := sess.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return res
}

func TestToolsListHasSix(t *testing.T) {
	sess := connect(t, &fakeBackend{})
	res, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 6 {
		t.Fatalf("want 6 tools, got %d", len(res.Tools))
	}
}

func TestSaveItemRequiresExactlyOne(t *testing.T) {
	sess := connect(t, &fakeBackend{})
	// neither
	if r := call(t, sess, "save_item", map[string]any{}); !r.IsError {
		t.Fatal("expected error when neither url nor note given")
	}
	// both
	if r := call(t, sess, "save_item", map[string]any{"url": "https://x", "note": "n"}); !r.IsError {
		t.Fatal("expected error when both url and note given")
	}
	// exactly one
	if r := call(t, sess, "save_item", map[string]any{"url": "https://x"}); r.IsError {
		t.Fatal("unexpected error saving a url")
	}
}

func TestGetItemNotFound(t *testing.T) {
	sess := connect(t, &fakeBackend{notFound: true})
	r := call(t, sess, "get_item", map[string]any{"id": uuid.New().String()})
	if !r.IsError {
		t.Fatal("expected not-found tool error")
	}
}

func TestGetItemBadUUID(t *testing.T) {
	sess := connect(t, &fakeBackend{})
	r := call(t, sess, "get_item", map[string]any{"id": "not-a-uuid"})
	if !r.IsError {
		t.Fatal("expected bad-uuid tool error")
	}
}

func TestSearchEmpty(t *testing.T) {
	sess := connect(t, &fakeBackend{})
	r := call(t, sess, "search_items", map[string]any{})
	if !r.IsError {
		t.Fatal("expected error when no query or color")
	}
}
