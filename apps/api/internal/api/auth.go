package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rohithgilla12/openmind/api/internal/auth"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// Auth modes accepted by AuthConfig.Mode.
const (
	AuthModeToken = "token"
	AuthModeClerk = "clerk"
)

// apiKeyPrefix identifies a bearer value as an Openmind API key rather than a
// legacy token or Clerk JWT.
const apiKeyPrefix = "omk_"

// touchLastUsedInterval caps how often a used API key's last_used_at is
// written, so a hot key doesn't cost a write per request.
const touchLastUsedInterval = time.Minute

// AuthConfig selects how incoming requests are authenticated.
type AuthConfig struct {
	// Mode is AuthModeToken (default) or AuthModeClerk.
	Mode string
	// LegacyToken is the bearer value checked in token mode. Empty disables
	// auth entirely (single-user self-host): every request resolves to
	// DevUserID, preserving today's unauthenticated dev behaviour.
	LegacyToken string
	// Verifier validates Clerk session JWTs. Required when Mode == AuthModeClerk.
	Verifier *auth.JWTVerifier
}

// authenticate resolves the caller's identity for every request except
// /healthz and POST /device-links/claim (the device code itself is the
// credential for that one endpoint). Resolution order:
//  1. An "omk_"-prefixed bearer value is looked up as an API key, in either
//     auth mode. A revoked or unknown key is 401.
//  2. In clerk mode, the bearer value is verified as a Clerk session JWT; the
//     mapped user is looked up or JIT-provisioned.
//  3. In token mode, the bearer value is compared to LegacyToken in constant
//     time. If LegacyToken is empty, every request is the dev user (the
//     unauthenticated self-host default, with a startup warning).
//
// Anything that doesn't resolve is 401.
func authenticate(s *store.Store, cfg AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || (r.Method == http.MethodPost && r.URL.Path == "/device-links/claim") {
				next.ServeHTTP(w, r)
				return
			}

			bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

			if strings.HasPrefix(bearer, apiKeyPrefix) {
				uid, ok := resolveAPIKey(r.Context(), s, bearer)
				if !ok {
					unauthorized(w)
					return
				}
				next.ServeHTTP(w, r.WithContext(withUserID(r.Context(), uid)))
				return
			}

			if cfg.Mode == AuthModeClerk {
				uid, ok := resolveClerkJWT(r.Context(), s, cfg.Verifier, bearer)
				if !ok {
					unauthorized(w)
					return
				}
				next.ServeHTTP(w, r.WithContext(withUserID(r.Context(), uid)))
				return
			}

			if cfg.LegacyToken == "" {
				next.ServeHTTP(w, r.WithContext(withUserID(r.Context(), DevUserID)))
				return
			}
			if subtle.ConstantTimeCompare([]byte(bearer), []byte(cfg.LegacyToken)) != 1 {
				unauthorized(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(withUserID(r.Context(), DevUserID)))
		})
	}
}

// resolveAPIKey looks up the user owning a full API key. Revoked and unknown
// keys both fail the lookup (GetAPIKeyByHash filters out revoked_at IS NOT
// NULL), so both resolve to the same 401. A successful lookup touches
// last_used_at, throttled to at most once per touchLastUsedInterval.
func resolveAPIKey(ctx context.Context, s *store.Store, full string) (uuid.UUID, bool) {
	if s == nil {
		return uuid.UUID{}, false
	}
	row, err := s.Queries.GetAPIKeyByHash(ctx, auth.HashKey(full))
	if err != nil {
		return uuid.UUID{}, false
	}
	if !row.LastUsedAt.Valid || time.Since(row.LastUsedAt.Time) > touchLastUsedInterval {
		if err := s.Queries.TouchAPIKeyLastUsed(ctx, row.ApiKeyID); err != nil {
			slog.Error("touching api key last_used_at", "err", err)
		}
	}
	return row.UserID, true
}

// resolveClerkJWT verifies a Clerk session JWT and maps it to a user, JIT
// provisioning a new user row on first sight of a Clerk subject.
func resolveClerkJWT(ctx context.Context, s *store.Store, v *auth.JWTVerifier, token string) (uuid.UUID, bool) {
	if s == nil || v == nil || token == "" {
		return uuid.UUID{}, false
	}
	claims, err := v.Verify(ctx, token)
	if err != nil {
		return uuid.UUID{}, false
	}
	clerkID := pgtype.Text{String: claims.Subject, Valid: true}
	user, err := s.Queries.GetUserByClerkID(ctx, clerkID)
	if err == nil {
		return user.ID, true
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		slog.Error("looking up clerk user", "err", err)
		return uuid.UUID{}, false
	}
	user, err = s.Queries.CreateUserFromClerk(ctx, db.CreateUserFromClerkParams{ClerkUserID: clerkID, Email: claims.Email})
	if err != nil {
		slog.Error("provisioning clerk user from JWT", "err", err)
		return uuid.UUID{}, false
	}
	return user.ID, true
}

func unauthorized(w http.ResponseWriter) {
	http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
}
