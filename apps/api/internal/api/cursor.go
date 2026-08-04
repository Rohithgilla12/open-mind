package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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
