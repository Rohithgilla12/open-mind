package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/geo"
	"github.com/rohithgilla12/openmind/api/internal/reelmedia"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// framer samples video frames for the deep-media rung; *reelmedia.Extractor
// satisfies it. nil means the rung is unavailable.
type framer interface {
	Frames(ctx context.Context, url string) ([][]byte, error)
}

// ExtractPlacesArgs is the River job payload for extracting places from an
// enriched social-video item. IDs only; the worker fetches fresh state.
type ExtractPlacesArgs struct {
	UserID uuid.UUID `json:"user_id"`
	ItemID uuid.UUID `json:"item_id"`
}

// Kind identifies the job type in River.
func (ExtractPlacesArgs) Kind() string { return "extract_places" }

// ExtractPlacesWorker pulls visitable places out of an item's caption text
// and, when a lead thumbnail is present, from on-screen text via vision. It
// optionally geocodes them and replaces the item's place rows. It runs as its
// own job, after enrichment, so a slow geocoder or model never blocks or
// retries the core enrichment pipeline.
type ExtractPlacesWorker struct {
	river.WorkerDefaults[ExtractPlacesArgs]
	Store    *store.Store
	Provider ai.Provider
	// Geocoder is optional (nil = geocoding off): places are then stored by
	// name with no coordinates.
	Geocoder geo.Geocoder
	// HTTPClient fetches lead-image thumbnails for vision. Nil defaults to
	// enrich.SafeHTTPClient inside fetchLeadImage.
	HTTPClient *http.Client
	// Mode is the resolved REEL_MEDIA ceiling. The zero value is ModeOff
	// (caption + location only, no vision calls) — callers that want the
	// thumbnail-vision rung must set it explicitly. reelmedia.ModeFromEnv is
	// what defaults to ModeThumbnail; production wiring in cmd/openmind uses
	// that, so Off only applies when a caller (e.g. a test) leaves Mode unset.
	Mode reelmedia.Mode
	// Extractor runs the deep-media rung. Nil (or Mode != Video) disables it.
	Extractor framer
}

// Work extracts and stores places for one item. Idempotent: it replaces the
// item's full place set, so a re-run reproduces the same rows. A provider
// that cannot extract places (noop) or an item with neither caption text nor
// a fetchable thumbnail is a clean no-op, never an error.
func (w *ExtractPlacesWorker) Work(ctx context.Context, job *river.Job[ExtractPlacesArgs]) error {
	q := w.Store.Queries
	item, err := q.GetItem(ctx, db.GetItemParams{UserID: job.Args.UserID, ID: job.Args.ItemID})
	if err != nil {
		return fmt.Errorf("loading item %s: %w", job.Args.ItemID, err)
	}

	hasText := strings.TrimSpace(item.Title+item.Body) != ""
	hasImage := strings.HasPrefix(item.LeadImageUrl, "http://") || strings.HasPrefix(item.LeadImageUrl, "https://")
	if !hasText && !hasImage {
		return nil
	}

	// locationPlaces come from the user-tagged location (e.g. Instagram's own
	// location tag) — the highest-confidence signal since it is not inferred
	// at all. ran tracks whether any provider call succeeded (including an
	// empty list). ErrNotSupported / fetch misses do not count — those leave
	// any existing rows alone, matching Phase 1 noop behaviour. A successful
	// empty extraction must still replace the place set so re-runs stay
	// idempotent when the model finds nothing.
	loc := strings.TrimSpace(item.TaggedLocation)
	var locationPlaces, captionPlaces, visionPlaces, videoPlaces []ai.Place
	if loc != "" {
		locationPlaces = []ai.Place{{Name: loc, Confidence: 0.98}}
	}
	ran := false

	if hasText {
		places, err := w.Provider.ExtractPlaces(ctx, item.Title, item.Body)
		switch {
		case errors.Is(err, ai.ErrNotSupported):
			// noop / text-incapable: leave captionPlaces empty
		case err != nil:
			return fmt.Errorf("extracting places: %w", err)
		default:
			captionPlaces = places
			ran = true
		}
	}

	if hasImage && w.Mode >= reelmedia.ModeThumbnail {
		data, _ := fetchLeadImage(ctx, w.HTTPClient, item.LeadImageUrl)
		if len(data) > 0 {
			places, err := w.Provider.ExtractPlacesVision(ctx, item.Title, item.Body, data)
			switch {
			case errors.Is(err, ai.ErrNotSupported):
				// Text-only provider: vision rung is simply off.
			case err != nil:
				// Vision is a best-effort bonus rung — never fail the job
				// (and re-run caption extraction) because a thumbnail call
				// hiccuped. A later re-run can still pick it up.
				slog.Warn("vision place extraction failed, keeping caption results",
					"item_id", item.ID, "err", err)
			default:
				visionPlaces = places
				ran = true
			}
		}
	}

	merged := ai.MergePlaces(
		ai.PlaceGroup{Places: locationPlaces, Source: "location", DefaultConf: 0.98},
		ai.PlaceGroup{Places: captionPlaces, Source: "caption", DefaultConf: 0.85},
		ai.PlaceGroup{Places: visionPlaces, Source: "vision", DefaultConf: 0.70},
	)

	// Deep-media rung: escalate only when the cheaper rungs found nothing.
	if w.Mode == reelmedia.ModeVideo && w.Extractor != nil && len(merged) == 0 {
		frames, ferr := w.Extractor.Frames(ctx, item.Url)
		switch {
		case ferr != nil:
			slog.Warn("reel frame extraction failed", "item_id", item.ID, "err", ferr)
		case len(frames) > 0:
			places, verr := w.Provider.ExtractPlacesVisionFrames(ctx, item.Title, item.Body, frames)
			switch {
			case errors.Is(verr, ai.ErrNotSupported):
			case verr != nil:
				slog.Warn("frame place extraction failed", "item_id", item.ID, "err", verr)
			default:
				videoPlaces, ran = places, true
				merged = ai.MergePlaces(
					ai.PlaceGroup{Places: locationPlaces, Source: "location", DefaultConf: 0.98},
					ai.PlaceGroup{Places: captionPlaces, Source: "caption", DefaultConf: 0.85},
					ai.PlaceGroup{Places: videoPlaces, Source: "video", DefaultConf: 0.75},
					ai.PlaceGroup{Places: visionPlaces, Source: "vision", DefaultConf: 0.70},
				)
			}
		}
	}

	// Write when any provider rung ran, or when a location tag exists (so a
	// noop instance still records the tagged place). Nothing at all → leave
	// existing rows untouched.
	if !ran && len(locationPlaces) == 0 {
		return nil
	}

	rows := make([]db.InsertItemPlaceParams, 0, len(merged))
	for _, p := range merged {
		row := db.InsertItemPlaceParams{
			UserID: item.UserID, ItemID: item.ID,
			Name: p.Name, Hint: p.Hint, Source: p.Source,
		}
		// Geocoding is best-effort decoration: a miss or error leaves the
		// place coordinate-less rather than failing (and re-running) the
		// whole extraction.
		if w.Geocoder != nil {
			query := p.Name
			if p.Hint != "" {
				query += ", " + p.Hint
			}
			res, ok, gerr := w.Geocoder.Geocode(ctx, query)
			switch {
			case gerr != nil:
				slog.Warn("geocoding place failed, storing without coordinates",
					"item_id", item.ID, "place", p.Name, "err", gerr)
			case ok:
				row.Address = res.Address
				row.Lat = pgtype.Float8{Float64: res.Lat, Valid: true}
				row.Lng = pgtype.Float8{Float64: res.Lng, Valid: true}
			}
		}
		rows = append(rows, row)
	}

	// Replace the item's place set atomically so a mid-write crash or retry
	// can never leave a mix of old and new rows. An empty merged set clears
	// prior rows (extraction ran and found nothing).
	return pgx.BeginFunc(ctx, w.Store.Pool, func(tx pgx.Tx) error {
		qtx := q.WithTx(tx)
		if err := qtx.DeleteItemPlaces(ctx, db.DeleteItemPlacesParams{UserID: item.UserID, ItemID: item.ID}); err != nil {
			return fmt.Errorf("clearing places: %w", err)
		}
		for _, row := range rows {
			if err := qtx.InsertItemPlace(ctx, row); err != nil {
				return fmt.Errorf("inserting place %q: %w", row.Name, err)
			}
		}
		return nil
	})
}
