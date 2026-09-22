package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/rohithgilla12/openmind/api/internal/jev"
	"github.com/rohithgilla12/openmind/api/internal/notify"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// kindleSettingKey is the user_settings row key holding the per-user
// Send-to-Kindle destination address.
const kindleSettingKey = "kindle_email"

// maxKindleEmailRunes mirrors RFC 5321's 254-octet address limit.
const maxKindleEmailRunes = 254

// validKindleEmailRe is a permissive shape check (not full RFC 5322) — good
// enough to catch obvious typos without rejecting valid addresses.
var validKindleEmailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// validKindleEmail reports whether v looks like a usable e-mail address.
func validKindleEmail(v string) bool {
	if utf8.RuneCountInString(v) > maxKindleEmailRunes {
		return false
	}
	return validKindleEmailRe.MatchString(v)
}

// currentSettings loads the caller's settings row and maps it to the API
// shape, used to build both the GET response and the PATCH echo.
func (s *Server) currentSettings(w http.ResponseWriter, r *http.Request) (Settings, bool) {
	ctx := r.Context()
	rows, err := s.store.Queries.ListUserSettings(ctx, userID(ctx))
	if err != nil {
		slog.Error("listing user settings", "err", err)
		writeError(w, http.StatusInternalServerError, "could not fetch settings")
		return Settings{}, false
	}
	out := Settings{}
	for _, row := range rows {
		switch row.Key {
		case kindleSettingKey:
			email := openapi_types.Email(row.Value)
			out.KindleEmail = &email
		case jev.SettingKeyAIAssisted:
			v := strings.EqualFold(strings.TrimSpace(row.Value), "true")
			out.AiAssistedOrganisation = &v
		case notify.KeyDigest:
			v := SettingsNotifyDigest(row.Value)
			out.NotifyDigest = &v
		case notify.KeyFeedRiver:
			v := SettingsNotifyFeedRiver(row.Value)
			out.NotifyFeedRiver = &v
		case notify.KeyLifecycle:
			v := SettingsNotifyLifecycle(row.Value)
			out.NotifyLifecycle = &v
		case notify.KeyQuietHours:
			v := row.Value
			out.NotifyQuietHours = &v
		case notify.KeyTimezone:
			v := row.Value
			out.NotifyTimezone = &v
		case notify.KeyDailyCap:
			if n, err := strconv.Atoi(row.Value); err == nil {
				out.NotifyDailyCap = &n
			}
		}
	}
	return out, true
}

// GetSettings returns the caller's per-user settings.
func (s *Server) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, ok := s.currentSettings(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// PatchSettings applies a partial update to the caller's settings: kindleEmail
// plus the six notify.* preferences. A field omitted from the body is left
// untouched; an explicit empty string clears it back to its documented
// default; any other value must pass validation before anything is written.
// Always responds with the resulting settings object.
func (s *Server) PatchSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := userID(ctx)

	// Decode into a plain-string local shape rather than PatchSettingsRequest:
	// openapi_types.Email's UnmarshalJSON rejects "" as an invalid address,
	// which would turn the "empty string clears the setting" case into a 400
	// before validKindleEmail ever runs.
	var req struct {
		KindleEmail             *string `json:"kindleEmail"`
		AiAssistedOrganisation  *bool   `json:"aiAssistedOrganisation"`
		NotifyDigest            *string `json:"notifyDigest"`
		NotifyFeedRiver         *string `json:"notifyFeedRiver"`
		NotifyLifecycle         *string `json:"notifyLifecycle"`
		NotifyQuietHours        *string `json:"notifyQuietHours"`
		NotifyTimezone          *string `json:"notifyTimezone"`
		NotifyDailyCap          *int    `json:"notifyDailyCap"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Each field follows the same rule: an explicit empty string clears the
	// row (restoring the documented default), any other value must pass the
	// field's validator. Every provided field is validated up front, before
	// any store call runs — otherwise an early field could be written and
	// then a later field in the same body could fail validation, leaving a
	// partially-applied PATCH, which the API must never do.
	prefs := []struct {
		key   string
		value *string
		valid func(string) bool
	}{
		{kindleSettingKey, req.KindleEmail, validKindleEmail},
		{notify.KeyDigest, req.NotifyDigest, validChannelPref},
		{notify.KeyFeedRiver, req.NotifyFeedRiver, validChannelPref},
		{notify.KeyLifecycle, req.NotifyLifecycle, validChannelPref},
		{notify.KeyQuietHours, req.NotifyQuietHours, validQuietHours},
		{notify.KeyTimezone, req.NotifyTimezone, validTimezone},
	}
	for _, pref := range prefs {
		if pref.value != nil && *pref.value != "" && !pref.valid(*pref.value) {
			writeError(w, http.StatusBadRequest, "invalid value for "+pref.key)
			return
		}
	}
	if req.NotifyDailyCap != nil && (*req.NotifyDailyCap < 0 || *req.NotifyDailyCap > 200) {
		writeError(w, http.StatusBadRequest, "notifyDailyCap must be between 0 and 200")
		return
	}

	// The seven writes below (six prefs plus the daily cap) are independent
	// store calls, not one statement. Without a transaction, a store failure
	// partway through (a dropped connection, a context deadline, a deadlock)
	// would leave earlier fields committed and later ones missing — the same
	// "partially-applied PATCH" the up-front validation above was written to
	// prevent, just triggered by infrastructure rather than bad input. A
	// single transaction makes the whole write phase atomic: every field
	// commits together, or none do. Mirrors the pattern in
	// CreateItemHighlight (highlights.go).
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		slog.Error("beginning settings tx", "err", err)
		writeError(w, http.StatusInternalServerError, "could not update settings")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.store.Queries.WithTx(tx)

	for _, pref := range prefs {
		if pref.value == nil {
			continue
		}
		if !applyPref(ctx, w, q, uid, pref.key, *pref.value) {
			return
		}
	}
	if req.AiAssistedOrganisation != nil {
		if *req.AiAssistedOrganisation {
			if err := q.UpsertUserSetting(ctx, db.UpsertUserSettingParams{
				UserID: uid, Key: jev.SettingKeyAIAssisted, Value: "true",
			}); err != nil {
				slog.Error("upserting ai-assisted setting", "err", err)
				writeError(w, http.StatusInternalServerError, "could not update settings")
				return
			}
		} else {
			if _, err := q.DeleteUserSetting(ctx, db.DeleteUserSettingParams{
				UserID: uid, Key: jev.SettingKeyAIAssisted,
			}); err != nil {
				slog.Error("clearing ai-assisted setting", "err", err)
				writeError(w, http.StatusInternalServerError, "could not update settings")
				return
			}
		}
	}
	if req.NotifyDailyCap != nil {
		if err := q.UpsertUserSetting(ctx, db.UpsertUserSettingParams{
			UserID: uid, Key: notify.KeyDailyCap, Value: strconv.Itoa(*req.NotifyDailyCap),
		}); err != nil {
			slog.Error("upserting daily cap", "err", err)
			writeError(w, http.StatusInternalServerError, "could not update settings")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("committing settings tx", "err", err)
		writeError(w, http.StatusInternalServerError, "could not update settings")
		return
	}

	settings, ok := s.currentSettings(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// applyPref clears or upserts one setting row through q (either the
// server's ordinary Queries or a tx-scoped one — see PatchSettings). value
// has already passed its validator, so this only performs the write and
// never rejects for bad input. It reports false once it has already written
// an error response for a store failure, so the caller can stop applying
// further fields.
func applyPref(ctx context.Context, w http.ResponseWriter, q *db.Queries, uid uuid.UUID, key, value string) bool {
	if value == "" {
		if _, err := q.DeleteUserSetting(ctx, db.DeleteUserSettingParams{UserID: uid, Key: key}); err != nil {
			slog.Error("deleting setting", "key", key, "err", err)
			writeError(w, http.StatusInternalServerError, "could not update settings")
			return false
		}
		return true
	}
	if err := q.UpsertUserSetting(ctx, db.UpsertUserSettingParams{UserID: uid, Key: key, Value: value}); err != nil {
		slog.Error("upserting setting", "key", key, "err", err)
		writeError(w, http.StatusInternalServerError, "could not update settings")
		return false
	}
	return true
}

// validChannelPref accepts the four documented channel selections.
func validChannelPref(v string) bool {
	switch v {
	case "off", "push", "email", "both":
		return true
	default:
		return false
	}
}

// validQuietHours accepts an HH:MM-HH:MM wall-clock range.
func validQuietHours(v string) bool {
	from, to, found := strings.Cut(v, "-")
	if !found {
		return false
	}
	_, errFrom := time.Parse("15:04", from)
	_, errTo := time.Parse("15:04", to)
	return errFrom == nil && errTo == nil
}

// validTimezone accepts any IANA zone the runtime can load. "Local" is
// rejected even though time.LoadLocation accepts it: it resolves to the
// server's own system timezone, not the user's, so a user who stores it
// would silently have their quiet hours evaluated against wherever the
// server happens to be deployed rather than the zone they think they set.
func validTimezone(v string) bool {
	if v == "Local" {
		return false
	}
	_, err := time.LoadLocation(v)
	return err == nil
}
