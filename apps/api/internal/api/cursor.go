package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// errInvalidCursor marks a cursor we cannot decode. Handlers turn it into a
// 400 rather than falling back to page 1: silently serving the top of the list
// to a client that believes it paged forward would hide the bug and duplicate
// rows in its list.
var errInvalidCursor = errors.New("invalid cursor")

// pageCursor is the keyset position of the last row of a page — the sort key of
// ORDER BY created_at DESC, id DESC. id breaks ties because created_at is not
// unique.
type pageCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// encodeCursor renders a keyset position as an opaque token. The timestamp is
// formatted from the value read out of the row, so it round-trips exactly
// against Postgres's microsecond storage. RawURLEncoding keeps the token free
// of '=' and '+', so it needs no escaping in a query string.
func encodeCursor(c pageCursor) string {
	raw := c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor parses a token produced by encodeCursor. A nil cursor with a nil
// error means no cursor was supplied, i.e. the caller wants the first page.
func decodeCursor(s *string) (*pageCursor, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*s)
	if err != nil {
		return nil, fmt.Errorf("decoding cursor: %w", errInvalidCursor)
	}
	// RFC3339Nano contains no '|', so a left split of two is unambiguous.
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("cursor missing separator: %w", errInvalidCursor)
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, fmt.Errorf("parsing cursor timestamp: %w", errInvalidCursor)
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, fmt.Errorf("parsing cursor id: %w", errInvalidCursor)
	}
	return &pageCursor{CreatedAt: ts, ID: id}, nil
}

// listLimit clamps a client-supplied limit to the house range.
func listLimit(v *int) int {
	limit := defaultListLimit
	if v != nil {
		limit = *v
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	return limit
}

// cursorTimestamp and cursorUUID render a decoded cursor as the nullable query
// args. A nil cursor yields invalid (NULL) values, which the queries read as
// "start at the newest row".
func cursorTimestamp(c *pageCursor) pgtype.Timestamptz {
	if c == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: c.CreatedAt, Valid: true}
}

func cursorUUID(c *pageCursor) pgtype.UUID {
	if c == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: c.ID, Valid: true}
}

// toItemPage trims an over-fetched row set to limit and derives nextCursor from
// the last row actually returned, so the token round-trips exactly.
func toItemPage(rows []db.Item, limit int) ItemPage {
	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		tok := encodeCursor(pageCursor{CreatedAt: last.CreatedAt.Time, ID: last.ID})
		next = &tok
	}
	items := make([]Item, 0, len(rows))
	for _, it := range rows {
		items = append(items, toAPIItem(it))
	}
	return ItemPage{Items: items, NextCursor: next}
}
