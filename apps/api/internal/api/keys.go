package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/rohithgilla12/openmind/api/internal/auth"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

const (
	// maxKeyNameRunes bounds both an API key's name and a claimed device's
	// name: trimmed, 1..80 runes.
	maxKeyNameRunes = 80
	// deviceLinkTTL is how long a minted device-link code stays claimable.
	deviceLinkTTL = 10 * time.Minute
)

// ListApiKeys returns the caller's API keys (including revoked ones, for
// history), newest first. Full key secrets are never returned after
// creation — only id/name/prefix and timestamps.
func (s *Server) ListApiKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keys, err := s.store.Queries.ListAPIKeys(ctx, userID(ctx))
	if err != nil {
		slog.Error("listing api keys", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list api keys")
		return
	}
	out := make([]ApiKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, toAPIKey(k))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateApiKey mints a new API key for the caller and returns it, including
// the full secret, exactly once.
func (s *Server) CreateApiKey(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req CreateApiKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name, ok := validateName(w, req.Name)
	if !ok {
		return
	}

	full, hash, prefix, err := auth.GenerateKey()
	if err != nil {
		slog.Error("generating api key", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create api key")
		return
	}

	row, err := s.store.Queries.CreateAPIKey(r.Context(), db.CreateAPIKeyParams{
		UserID:  userID(r.Context()),
		Name:    name,
		KeyHash: hash,
		Prefix:  prefix,
	})
	if err != nil {
		slog.Error("creating api key", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create api key")
		return
	}

	writeJSON(w, http.StatusCreated, ApiKeyCreated{
		Key:       full,
		Id:        openapi_types.UUID(row.ID),
		Name:      row.Name,
		Prefix:    row.Prefix,
		CreatedAt: row.CreatedAt.Time,
	})
}

// RevokeApiKey revokes a key owned by the caller and returns 204. An unknown
// id or another user's key both resolve to 404 because the update is
// user-scoped and affects no rows.
func (s *Server) RevokeApiKey(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	rows, err := s.store.Queries.RevokeAPIKey(r.Context(), db.RevokeAPIKeyParams{UserID: userID(r.Context()), ID: id})
	if err != nil {
		slog.Error("revoking api key", "err", err)
		writeError(w, http.StatusInternalServerError, "could not revoke api key")
		return
	}
	if rows == 0 {
		writeError(w, http.StatusNotFound, "api key not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateDeviceLink mints a short-lived device-link code the caller can hand
// to a second device; that device redeems it via ClaimDeviceLink for its own
// API key. The request body is optional (deviceHint is advisory only).
func (s *Server) CreateDeviceLink(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var req CreateDeviceLinkRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	var hint string
	if req.DeviceHint != nil {
		hint = strings.TrimSpace(*req.DeviceHint)
	}

	code, hash, err := auth.GenerateCode()
	if err != nil {
		slog.Error("generating device link code", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create device link")
		return
	}

	link, err := s.store.Queries.CreateDeviceLink(r.Context(), db.CreateDeviceLinkParams{
		CodeHash:   hash,
		UserID:     userID(r.Context()),
		DeviceHint: hint,
		ExpiresAt:  pgtype.Timestamptz{Time: time.Now().Add(deviceLinkTTL), Valid: true},
	})
	if err != nil {
		slog.Error("creating device link", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create device link")
		return
	}

	writeJSON(w, http.StatusCreated, DeviceLinkCreated{Code: code, ExpiresAt: link.ExpiresAt.Time})
}

// ClaimDeviceLink redeems a device-link code for a fresh API key. It runs
// without bearer auth (the code itself is the credential) but sits behind its
// own strict per-IP rate bucket (see claimRateLimit). Unknown, expired, and
// already-claimed codes are all indistinguishable 404s so a prober can't tell
// which case applied.
func (s *Server) ClaimDeviceLink(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req ClaimDeviceLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	deviceName, ok := validateName(w, req.DeviceName)
	if !ok {
		return
	}
	normalized := auth.NormalizeCode(req.Code)
	if normalized == "" {
		writeError(w, http.StatusNotFound, "device link not found")
		return
	}

	ctx := r.Context()
	uid, err := s.store.Queries.ClaimDeviceLink(ctx, auth.HashCode(normalized))
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("claiming device link", "err", err)
		}
		writeError(w, http.StatusNotFound, "device link not found")
		return
	}

	full, hash, prefix, err := auth.GenerateKey()
	if err != nil {
		slog.Error("generating api key for claimed device", "err", err)
		writeError(w, http.StatusInternalServerError, "could not claim device link")
		return
	}
	if _, err := s.store.Queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		UserID:  uid,
		Name:    deviceName,
		KeyHash: hash,
		Prefix:  prefix,
	}); err != nil {
		slog.Error("creating api key for claimed device", "err", err)
		writeError(w, http.StatusInternalServerError, "could not claim device link")
		return
	}

	writeJSON(w, http.StatusCreated, DeviceLinkClaimed{Key: full, Name: deviceName})
}

// validateName trims raw and enforces the 1..80 rune bound shared by API key
// names and claimed-device names, writing a 400 and returning ok=false on
// violation.
func validateName(w http.ResponseWriter, raw string) (name string, ok bool) {
	name = strings.TrimSpace(raw)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return "", false
	}
	if utf8.RuneCountInString(name) > maxKeyNameRunes {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("name too long (max %d chars)", maxKeyNameRunes))
		return "", false
	}
	return name, true
}

// toAPIKey maps a stored key row to the API model, omitting unset timestamps.
func toAPIKey(k db.ApiKey) ApiKey {
	out := ApiKey{
		Id:        openapi_types.UUID(k.ID),
		Name:      k.Name,
		Prefix:    k.Prefix,
		CreatedAt: k.CreatedAt.Time,
	}
	if k.LastUsedAt.Valid {
		t := k.LastUsedAt.Time
		out.LastUsedAt = &t
	}
	if k.RevokedAt.Valid {
		t := k.RevokedAt.Time
		out.RevokedAt = &t
	}
	return out
}
