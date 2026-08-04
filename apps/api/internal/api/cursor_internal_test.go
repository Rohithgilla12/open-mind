package api

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCursorRoundTrip(t *testing.T) {
	// Microsecond precision: Postgres stores microseconds, so a cursor built
	// from a row read back out must survive the round trip exactly.
	want := pageCursor{
		CreatedAt: time.Date(2026, 8, 4, 12, 34, 56, 123456000, time.UTC),
		ID:        uuid.MustParse("11111111-2222-3333-4444-555555555555"),
	}
	tok := encodeCursor(want)
	if tok == "" {
		t.Fatal("encodeCursor returned empty string")
	}
	for _, c := range tok {
		if c == '=' || c == '+' || c == '/' {
			t.Errorf("token contains %q; must be URL-safe and unpadded: %s", c, tok)
		}
	}

	got, err := decodeCursor(&tok)
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if got == nil {
		t.Fatal("decodeCursor returned nil for a valid token")
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, want.CreatedAt)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %v, want %v", got.ID, want.ID)
	}
}

func TestDecodeCursorAbsent(t *testing.T) {
	empty := ""
	for name, in := range map[string]*string{"nil": nil, "empty": &empty} {
		t.Run(name, func(t *testing.T) {
			got, err := decodeCursor(in)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if got != nil {
				t.Errorf("got %+v, want nil (meaning first page)", got)
			}
		})
	}
}

func TestDecodeCursorInvalid(t *testing.T) {
	cases := map[string]string{
		"not base64":   "!!!not-base64!!!",
		"no separator": "MjAyNi0wOC0wNA",
		"bad time":     encodeRaw("not-a-time|11111111-2222-3333-4444-555555555555"),
		"bad uuid":     encodeRaw("2026-08-04T12:34:56Z|not-a-uuid"),
		"empty both":   encodeRaw("|"),
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			tok := tok
			got, err := decodeCursor(&tok)
			if !errors.Is(err, errInvalidCursor) {
				t.Errorf("err = %v, want errInvalidCursor", err)
			}
			if got != nil {
				t.Errorf("got %+v, want nil on error", got)
			}
		})
	}
}

func encodeRaw(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}
