package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/rohithgilla12/openmind/api/internal/jev"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// PostJevVerdict records a user reaction to a Phase 2 capture suggestion or
// auto-applied tag, backfilling jev_decisions.user_verdict. Accept adds the
// tag to userTags; undo removes it; dismiss hides a suggestion chip.
func (s *Server) PostJevVerdict(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req JevVerdictRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	tag := strings.ToLower(strings.TrimSpace(req.Tag))
	if tag == "" {
		writeError(w, http.StatusBadRequest, "tag is required")
		return
	}
	switch req.Action {
	case Accept, Dismiss, Undo, Keep:
	default:
		writeError(w, http.StatusBadRequest, "invalid action")
		return
	}

	ctx := r.Context()
	uid := userID(ctx)

	item, err := s.store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "item not found")
			return
		}
		slog.Error("jev verdict: get item", "err", err)
		writeError(w, http.StatusInternalServerError, "could not fetch item")
		return
	}

	dec, err := s.store.Queries.GetJevCaptureDecision(ctx, db.GetJevCaptureDecisionParams{
		UserID: uid, ItemID: pgtype.UUID{Bytes: item.ID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "no capture decision for item")
			return
		}
		slog.Error("jev verdict: get decision", "err", err)
		writeError(w, http.StatusInternalServerError, "could not fetch decision")
		return
	}

	verdict := ""
	switch req.Action {
	case Accept:
		if !slices.Contains(dec.SuggestedTags, tag) {
			writeError(w, http.StatusBadRequest, "tag is not a pending suggestion")
			return
		}
		if !slices.Contains(item.UserTags, tag) {
			next := append(append([]string{}, item.UserTags...), tag)
			if _, err := s.store.Queries.SetUserTags(ctx, db.SetUserTagsParams{
				UserID: uid, ID: id, UserTags: canonicalTags(next),
			}); err != nil {
				slog.Error("jev verdict: accept tag", "err", err)
				writeError(w, http.StatusInternalServerError, "could not update tags")
				return
			}
		}
		verdict = jev.VerdictKept
	case Dismiss:
		if !slices.Contains(dec.SuggestedTags, tag) {
			writeError(w, http.StatusBadRequest, "tag is not a suggestion")
			return
		}
		if _, err := s.store.Queries.AddJevDismissedTag(ctx, db.AddJevDismissedTagParams{
			UserID: uid, ID: dec.ID, Tag: tag,
		}); err != nil {
			slog.Error("jev verdict: dismiss", "err", err)
			writeError(w, http.StatusInternalServerError, "could not dismiss suggestion")
			return
		}
		verdict = jev.VerdictRemoved
	case Undo:
		if !slices.Contains(dec.AppliedTags, tag) {
			writeError(w, http.StatusBadRequest, "tag was not auto-applied")
			return
		}
		next := make([]string, 0, len(item.UserTags))
		for _, t := range item.UserTags {
			if t != tag {
				next = append(next, t)
			}
		}
		if _, err := s.store.Queries.SetUserTags(ctx, db.SetUserTagsParams{
			UserID: uid, ID: id, UserTags: next,
		}); err != nil {
			slog.Error("jev verdict: undo tag", "err", err)
			writeError(w, http.StatusInternalServerError, "could not update tags")
			return
		}
		verdict = jev.VerdictRemoved
	case Keep:
		if !slices.Contains(dec.AppliedTags, tag) {
			writeError(w, http.StatusBadRequest, "tag was not auto-applied")
			return
		}
		verdict = jev.VerdictKept
	}

	if _, err := s.store.Queries.SetJevUserVerdict(ctx, db.SetJevUserVerdictParams{
		UserID: uid, ID: dec.ID, UserVerdict: pgtype.Text{String: verdict, Valid: true},
	}); err != nil {
		slog.Error("jev verdict: set verdict", "err", err)
		writeError(w, http.StatusInternalServerError, "could not record verdict")
		return
	}

	item, err = s.store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: id})
	if err != nil {
		slog.Error("jev verdict: reload item", "err", err)
		writeError(w, http.StatusInternalServerError, "could not fetch item")
		return
	}
	writeJSON(w, http.StatusOK, s.itemDetailWithJev(ctx, uid, item))
}

// itemDetailWithJev maps an item and attaches pending Jev suggestion chips when
// a capture decision exists.
func (s *Server) itemDetailWithJev(ctx context.Context, uid uuid.UUID, item db.Item) ItemDetail {
	out := toAPIItemDetail(item)
	dec, err := s.store.Queries.GetJevCaptureDecision(ctx, db.GetJevCaptureDecisionParams{
		UserID: uid, ItemID: pgtype.UUID{Bytes: item.ID, Valid: true},
	})
	if err != nil {
		return out
	}
	userTags := item.UserTags
	if userTags == nil {
		userTags = []string{}
	}
	sug := JevSuggestions{
		Action:        dec.Action,
		SuggestedTags: jev.PendingSuggestions(dec.SuggestedTags, dec.DismissedTags, userTags),
		AppliedTags:   jev.RemovableApplied(dec.AppliedTags, userTags),
	}
	if sug.SuggestedTags == nil {
		sug.SuggestedTags = []string{}
	}
	if sug.AppliedTags == nil {
		sug.AppliedTags = []string{}
	}
	if dec.UserVerdict.Valid && dec.UserVerdict.String != "" {
		v := dec.UserVerdict.String
		sug.UserVerdict = &v
	}
	// Omit the block when there is nothing for the UI to show and no prior
	// verdict — keeps list/detail payloads small for items without chips.
	if len(sug.SuggestedTags) == 0 && len(sug.AppliedTags) == 0 && sug.UserVerdict == nil {
		if dec.Action == string(jev.ActionSkipped) || dec.Action == string(jev.ActionShadow) {
			return out
		}
	}
	out.JevSuggestions = &sug
	return out
}

// maybeBackfillJevTagEdit records changed/removed when PATCH userTags alters
// tags that Jev auto-applied. Best-effort; never fails the patch.
func (s *Server) maybeBackfillJevTagEdit(ctx context.Context, uid, itemID uuid.UUID, before, after []string) {
	dec, err := s.store.Queries.GetJevCaptureDecision(ctx, db.GetJevCaptureDecisionParams{
		UserID: uid, ItemID: pgtype.UUID{Bytes: itemID, Valid: true},
	})
	if err != nil || len(dec.AppliedTags) == 0 {
		return
	}
	beforeSet := make(map[string]struct{}, len(before))
	for _, t := range before {
		beforeSet[t] = struct{}{}
	}
	afterSet := make(map[string]struct{}, len(after))
	for _, t := range after {
		afterSet[t] = struct{}{}
	}
	removedApplied := false
	for _, t := range dec.AppliedTags {
		_, had := beforeSet[t]
		_, has := afterSet[t]
		if had && !has {
			removedApplied = true
			break
		}
	}
	if !removedApplied {
		return
	}
	verdict := jev.VerdictRemoved
	// If any applied tag remains or other tags were added, call it changed.
	for _, t := range dec.AppliedTags {
		if _, has := afterSet[t]; has {
			verdict = jev.VerdictChanged
			break
		}
	}
	if _, err := s.store.Queries.SetJevUserVerdict(ctx, db.SetJevUserVerdictParams{
		UserID: uid, ID: dec.ID, UserVerdict: pgtype.Text{String: verdict, Valid: true},
	}); err != nil {
		slog.Warn("jev verdict: backfill from tag edit", "err", err)
	}
}
