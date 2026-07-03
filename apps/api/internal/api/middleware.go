package api

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// DevUserID is the fixed account used in single-user / self-hosted mode. It is
// EnsureUser-provisioned at startup and injected into every request context by
// the dev-user middleware until real auth lands.
var DevUserID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

type ctxKey int

const userIDKey ctxKey = iota

// devUser middleware injects the fixed dev user ID into the request context.
func devUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), userIDKey, DevUserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// userID returns the authenticated user ID from the request context, falling
// back to the dev user if the middleware did not run.
func userID(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(userIDKey).(uuid.UUID); ok {
		return id
	}
	return DevUserID
}
