package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

func TestListItemsKeysetPagesWholeSet(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	base := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	const total = 7
	for i := 0; i < total; i++ {
		it, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
			UserID: userID, Url: "https://example.com/" + string(rune('a'+i)), Body: "",
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		// Spread created_at so ordering is unambiguous, newest last-created.
		if _, err := s.Pool.Exec(ctx,
			`UPDATE items SET created_at = $2 WHERE id = $1`,
			it.ID, base.Add(time.Duration(i)*time.Minute),
		); err != nil {
			t.Fatalf("backdate %d: %v", i, err)
		}
	}

	// Page through in 3s and assert every row is seen exactly once.
	seen := map[uuid.UUID]int{}
	var cursorAt pgtype.Timestamptz
	var cursorID pgtype.UUID
	for page := 0; page < 10; page++ {
		rows, err := s.Queries.ListItems(ctx, db.ListItemsParams{
			UserID: userID, CursorCreatedAt: cursorAt, CursorID: cursorID, LimitCount: 3,
		})
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			seen[r.ID]++
		}
		last := rows[len(rows)-1]
		cursorAt = last.CreatedAt
		cursorID = pgtype.UUID{Bytes: last.ID, Valid: true}
	}

	if len(seen) != total {
		t.Errorf("saw %d distinct items, want %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("item %s served %d times, want exactly 1", id, n)
		}
	}
}

func TestListItemsKeysetBreaksTiesOnID(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	// Two items sharing created_at exactly, straddling a page boundary of 1.
	same := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	for _, u := range []string{"https://example.com/x", "https://example.com/y"} {
		it, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: u, Body: ""})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := s.Pool.Exec(ctx, `UPDATE items SET created_at = $2 WHERE id = $1`, it.ID, same); err != nil {
			t.Fatalf("backdate: %v", err)
		}
	}

	first, err := s.Queries.ListItems(ctx, db.ListItemsParams{UserID: userID, LimitCount: 1})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first page had %d rows, want 1", len(first))
	}
	second, err := s.Queries.ListItems(ctx, db.ListItemsParams{
		UserID:          userID,
		CursorCreatedAt: first[0].CreatedAt,
		CursorID:        pgtype.UUID{Bytes: first[0].ID, Valid: true},
		LimitCount:      1,
	})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second page had %d rows, want 1 (identical created_at must not swallow a row)", len(second))
	}
	if second[0].ID == first[0].ID {
		t.Error("second page repeated the first page's row")
	}
}

func TestListItemsKeysetStableWhenHeadGrows(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	base := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		it, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
			UserID: userID, Url: "https://example.com/old" + string(rune('a'+i)), Body: "",
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := s.Pool.Exec(ctx, `UPDATE items SET created_at = $2 WHERE id = $1`,
			it.ID, base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("backdate: %v", err)
		}
	}

	first, err := s.Queries.ListItems(ctx, db.ListItemsParams{UserID: userID, LimitCount: 2})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}

	// A capture lands at the head between requests. Under OFFSET 2 this would
	// shift the window and re-serve first[1]; keyset must be immune.
	newItem, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
		UserID: userID, Url: "https://example.com/brand-new", Body: "",
	})
	if err != nil {
		t.Fatalf("create new: %v", err)
	}
	// A wall-clock now() default is not guaranteed to sort after the fixture
	// rows above (they may be backdated ahead of the real clock), so the head
	// insert must be pinned explicitly to a timestamp later than all of them
	// or this test could pass without ever exercising a head insert.
	if _, err := s.Pool.Exec(ctx, `UPDATE items SET created_at = $2 WHERE id = $1`,
		newItem.ID, base.Add(10*time.Minute)); err != nil {
		t.Fatalf("backdate new: %v", err)
	}

	second, err := s.Queries.ListItems(ctx, db.ListItemsParams{
		UserID:          userID,
		CursorCreatedAt: first[len(first)-1].CreatedAt,
		CursorID:        pgtype.UUID{Bytes: first[len(first)-1].ID, Valid: true},
		LimitCount:      2,
	})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	for _, a := range first {
		for _, b := range second {
			if a.ID == b.ID {
				t.Errorf("item %s appeared on both pages after a head insert", a.ID)
			}
		}
	}
}
