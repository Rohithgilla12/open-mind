package api

import (
	"context"

	"github.com/google/uuid"
)

// DevUserID is the fixed account used in single-user / self-hosted mode. It is
// EnsureUser-provisioned at startup and is the identity assigned to every
// request when auth is disabled (token mode with an empty LegacyToken).
var DevUserID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

type ctxKey int

const userIDKey ctxKey = iota

// withUserID returns a context carrying the authenticated user id, for the
// authenticate middleware to attach once it has resolved a caller.
func withUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

// userID returns the authenticated user ID from the request context, falling
// back to the dev user if the middleware did not run.
func userID(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(userIDKey).(uuid.UUID); ok {
		return id
	}
	return DevUserID
}
