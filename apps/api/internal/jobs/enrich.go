// Package jobs defines River job workers for the enrichment pipeline.
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
	"github.com/rohithgilla12/openmind/api/internal/geo"
	"github.com/rohithgilla12/openmind/api/internal/reelmedia"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// EnrichArgs is the River job payload for enriching a saved item. It carries
// only identifiers; the worker fetches fresh state inside the job.
type EnrichArgs struct {
	UserID uuid.UUID `json:"user_id"`
	ItemID uuid.UUID `json:"item_id"`
}

// Kind identifies the job type in River.
func (EnrichArgs) Kind() string { return "enrich_item" }

// EnrichWorker runs the enrichment pipeline for an item.
type EnrichWorker struct {
	river.WorkerDefaults[EnrichArgs]
	Pipeline *enrich.Pipeline
	// River enqueues the follow-up extract_places job for social-video items.
	// Set after client construction (same pattern as ScanDigestsWorker.River);
	// nil skips the follow-up, which keeps enrichment self-contained in tests.
	River *river.Client[pgx.Tx]
}

// Work executes the enrichment pipeline, then chains the extract_places job
// for social-video items. The follow-up is separate work: its failure must
// not re-run enrichment, and a failed enqueue is only logged — the enrichment
// itself succeeded, and a manual re-enrich re-offers the chance to enqueue.
func (w *EnrichWorker) Work(ctx context.Context, job *river.Job[EnrichArgs]) error {
	if err := w.Pipeline.Run(ctx, job.Args.UserID, job.Args.ItemID); err != nil {
		return err
	}
	if w.River == nil {
		return nil
	}
	item, err := w.Pipeline.Store.Queries.GetItem(ctx, db.GetItemParams{UserID: job.Args.UserID, ID: job.Args.ItemID})
	if err != nil || !enrich.IsSocialVideoURL(item.Url) {
		return nil
	}
	if _, err := w.River.Insert(ctx, ExtractPlacesArgs{UserID: job.Args.UserID, ItemID: job.Args.ItemID}, nil); err != nil {
		slog.Warn("enqueueing extract_places", "item_id", job.Args.ItemID, "err", err)
	}
	return nil
}

// digestScanInterval is how often the periodic scan_digests job runs. It is
// hourly rather than matching the coarsest schedule (weekly) so a daily
// digest lens is checked often enough that digestDue's 20h threshold is
// meaningful.
const digestScanInterval = time.Hour

// pollInterval is how often the periodic poll_feeds job is enqueued. Each run
// only refreshes feeds whose own adaptive schedule (next_poll_at) has come
// due, so this just needs to be at least as frequent as the shortest possible
// per-feed interval (pollFloor, 30m) for a feed to be picked up promptly.
const pollInterval = 30 * time.Minute

// NewRiverClient builds a River client over the given pool. When workersOn is
// true it registers the enrichment, feed-poll, send-kindle, and scan-digests
// workers, a default queue, and the periodic feed-poll and scan-digests jobs;
// otherwise it returns an insert-only client (for the API process), which
// enqueues jobs but runs none. feedService is only used when workersOn (the
// poll worker + periodic job); the insert-only path ignores it and may be
// passed nil. kindleDeps is likewise only exercised by the worker process;
// the insert-only path still accepts it (unused) so callers don't need two
// signatures. reelMode selects the reel-media ladder ceiling (off, thumbnail,
// video) for the extract-places worker; reelExtractor is the optional
// yt-dlp/ffmpeg extractor used to satisfy the video rung, and is nil when
// deep media is unavailable (binaries not on PATH) or the ceiling doesn't
// require it.
func NewRiverClient(pool *pgxpool.Pool, p *enrich.Pipeline, feedService FeedRefresher, kindleDeps KindleDeps, geocoder geo.Geocoder, reelMode reelmedia.Mode, reelExtractor *reelmedia.Extractor, workersOn bool) (*river.Client[pgx.Tx], error) {
	cfg := &river.Config{}
	// scanWorker and enrichWorker are registered before the client exists
	// (AddWorker needs a worker instance up front), but their River fields —
	// used to enqueue follow-up jobs — can only be set once the client is
	// built. Since AddWorker takes a pointer and River is only read inside
	// Work (called later, after NewRiverClient returns), setting the fields
	// after construction is safe.
	var scanWorker *ScanDigestsWorker
	var enrichWorker *EnrichWorker
	if workersOn {
		workers := river.NewWorkers()
		scanWorker = &ScanDigestsWorker{Store: p.Store, Provider: p.AI, Deps: kindleDeps}
		enrichWorker = &EnrichWorker{Pipeline: p}
		river.AddWorker(workers, enrichWorker)
		epw := &ExtractPlacesWorker{Store: p.Store, Provider: p.AI, Geocoder: geocoder, Mode: reelMode}
		if reelExtractor != nil {
			epw.Extractor = reelExtractor
		}
		river.AddWorker(workers, epw)
		river.AddWorker(workers, &PollFeedsWorker{Service: feedService})
		river.AddWorker(workers, &SendKindleWorker{Store: p.Store, Provider: p.AI, Deps: kindleDeps})
		river.AddWorker(workers, scanWorker)
		cfg.Workers = workers
		cfg.Queues = map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 5}}
		cfg.PeriodicJobs = []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(pollInterval),
				func() (river.JobArgs, *river.InsertOpts) { return PollFeedsArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true},
			),
			river.NewPeriodicJob(
				river.PeriodicInterval(digestScanInterval),
				func() (river.JobArgs, *river.InsertOpts) { return ScanDigestsArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true},
			),
		}
	}
	client, err := river.NewClient(riverpgxv5.New(pool), cfg)
	if err != nil {
		return nil, fmt.Errorf("creating river client: %w", err)
	}
	if workersOn {
		scanWorker.River = client
		enrichWorker.River = client
	}
	return client, nil
}
