package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/riverqueue/river"

	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// kindleNotConfiguredMsg is returned verbatim as the 409 body's "error"
// field, so operators get the exact env vars to set.
const kindleNotConfiguredMsg = "kindle is not configured — set SMTP_HOST, SMTP_FROM and KINDLE_EMAIL"

// kindleMaxAttempts caps River retries for a send_kindle job: SMTP failures
// are usually transient, but a job should eventually give up rather than
// retry forever.
const kindleMaxAttempts = 5

// SendItemToKindle queues an item to be built into an EPUB and e-mailed to
// the configured Kindle address. Checks run in order: ownership (404),
// feature configured (409), body present (422) — so the most specific,
// cheapest-to-explain failure wins.
func (s *Server) SendItemToKindle(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	uid := userID(ctx)
	item, err := s.store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "item not found")
			return
		}
		slog.Error("getting item", "err", err)
		writeError(w, http.StatusInternalServerError, "could not fetch item")
		return
	}
	if !s.kindleConfigured {
		writeError(w, http.StatusConflict, kindleNotConfiguredMsg)
		return
	}
	if item.Body == "" {
		writeError(w, http.StatusUnprocessableEntity, "item has no body to send")
		return
	}

	itemID := item.ID
	if _, err := s.riverClient.Insert(ctx, jobs.SendKindleArgs{UserID: uid, ItemID: &itemID}, &river.InsertOpts{MaxAttempts: kindleMaxAttempts}); err != nil {
		slog.Error("enqueueing send_kindle job", "item_id", item.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "could not queue kindle send")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
}

// SendLensToKindle queues a Lens's current matches to be built into a digest
// EPUB and e-mailed to the configured Kindle address. The emptiness check
// runs the Lens rule synchronously (same cost as GetLensItems) so a rule
// matching nothing with a body returns 422 immediately rather than after a
// round trip through the job queue.
func (s *Server) SendLensToKindle(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	uid := userID(ctx)
	l, err := s.store.Queries.GetLens(ctx, db.GetLensParams{UserID: uid, ID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "lens not found")
			return
		}
		slog.Error("getting lens", "err", err)
		writeError(w, http.StatusInternalServerError, "could not fetch lens")
		return
	}
	if !s.kindleConfigured {
		writeError(w, http.StatusConflict, kindleNotConfiguredMsg)
		return
	}

	rule := decodeStoredRule(l.Rule)
	results, err := s.runLensRule(ctx, uid, rule)
	if err != nil && !errors.Is(err, search.ErrBadColor) {
		// A stored colour became invalid is treated as an empty match set
		// (mirrors GetLensItems), not a 500 — any other error is infra.
		slog.Error("running lens rule", "err", err)
		writeError(w, http.StatusInternalServerError, "could not run lens")
		return
	}
	hasBody := false
	for _, res := range results {
		if res.Item.Body != "" {
			hasBody = true
			break
		}
	}
	if !hasBody {
		writeError(w, http.StatusUnprocessableEntity, "lens has no matching items with a body to send")
		return
	}

	lensID := l.ID
	if _, err := s.riverClient.Insert(ctx, jobs.SendKindleArgs{UserID: uid, LensID: &lensID}, &river.InsertOpts{MaxAttempts: kindleMaxAttempts}); err != nil {
		slog.Error("enqueueing send_kindle job", "lens_id", l.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "could not queue kindle send")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
}
