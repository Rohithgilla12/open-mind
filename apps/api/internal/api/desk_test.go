package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// pinItem sets pinned_at on an item to an explicit time via the store, giving
// tests deterministic Desk ordering independent of wall-clock timing.
func pinItem(t *testing.T, s *store.Store, uid uuid.UUID, id string, at time.Time) {
	t.Helper()
	rows, err := s.Queries.SetItemPinned(context.Background(), db.SetItemPinnedParams{
		UserID:   uid,
		ID:       uuid.MustParse(id),
		PinnedAt: pgtype.Timestamptz{Time: at, Valid: true},
	})
	if err != nil {
		t.Fatalf("set pinned: %v", err)
	}
	if rows != 1 {
		t.Fatalf("set pinned affected %d rows, want 1", rows)
	}
}

func getDesk(t *testing.T, baseURL string) []map[string]any {
	t.Helper()
	resp, err := http.Get(baseURL + "/desk")
	if err != nil {
		t.Fatalf("get desk: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("desk status = %d, want 200", resp.StatusCode)
	}
	var items []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatalf("decode desk: %v", err)
	}
	return items
}

func TestGetDeskReturnsPinnedNewestFirst(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)

	older := createNoteItem(t, srv.URL, "older pin")
	newer := createNoteItem(t, srv.URL, "newer pin")
	createNoteItem(t, srv.URL, "not pinned")

	now := time.Now()
	pinItem(t, s, api.DevUserID, older, now.Add(-time.Hour))
	pinItem(t, s, api.DevUserID, newer, now)

	items := getDesk(t, srv.URL)
	if len(items) != 2 {
		t.Fatalf("desk has %d items, want 2 (unpinned excluded)", len(items))
	}
	if items[0]["id"] != newer {
		t.Errorf("first item id = %v, want newest-pinned %s", items[0]["id"], newer)
	}
	if items[1]["id"] != older {
		t.Errorf("second item id = %v, want %s", items[1]["id"], older)
	}
}

func TestGetDeskExcludesOtherUsers(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)

	mine := createNoteItem(t, srv.URL, "mine")
	pinItem(t, s, api.DevUserID, mine, time.Now())

	otherID := seedOtherUserItem(t, s, "theirs")
	other := uuid.MustParse("00000000-0000-0000-0000-0000000000ff")
	pinItem(t, s, other, otherID, time.Now())

	items := getDesk(t, srv.URL)
	if len(items) != 1 {
		t.Fatalf("desk has %d items, want 1 (other user's pin excluded)", len(items))
	}
	if items[0]["id"] != mine {
		t.Errorf("item id = %v, want %s", items[0]["id"], mine)
	}
}

func TestGetDeskEmptyReturnsArray(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)

	resp, err := http.Get(srv.URL + "/desk")
	if err != nil {
		t.Fatalf("get desk: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "[]" {
		t.Errorf("empty desk body = %q, want []", strings.TrimSpace(string(raw)))
	}
}
