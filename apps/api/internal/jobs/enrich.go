// Package jobs defines River job workers for the enrichment pipeline.
package jobs

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
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
}

// Work executes the enrichment pipeline. River retries on error.
func (w *EnrichWorker) Work(ctx context.Context, job *river.Job[EnrichArgs]) error {
	return w.Pipeline.Run(ctx, job.Args.UserID, job.Args.ItemID)
}

// NewRiverClient builds a River client over the given pool. When workersOn is
// true it registers the enrichment, feed-poll, and send-kindle workers, a
// default queue, and the periodic feed-poll job; otherwise it returns an
// insert-only client (for the API process), which enqueues jobs but runs
// none. feedService is only used when workersOn (the poll worker + periodic
// job); the insert-only path ignores it and may be passed nil. kindleDeps is
// likewise only exercised by the worker process; the insert-only path still
// accepts it (unused) so callers don't need two signatures.
func NewRiverClient(pool *pgxpool.Pool, p *enrich.Pipeline, feedService FeedRefresher, kindleDeps KindleDeps, workersOn bool) (*river.Client[pgx.Tx], error) {
	cfg := &river.Config{}
	if workersOn {
		workers := river.NewWorkers()
		river.AddWorker(workers, &EnrichWorker{Pipeline: p})
		river.AddWorker(workers, &PollFeedsWorker{Service: feedService})
		river.AddWorker(workers, &SendKindleWorker{Store: p.Store, Provider: p.AI, Deps: kindleDeps})
		cfg.Workers = workers
		cfg.Queues = map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 5}}
		cfg.PeriodicJobs = []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(pollInterval),
				func() (river.JobArgs, *river.InsertOpts) { return PollFeedsArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true},
			),
		}
	}
	client, err := river.NewClient(riverpgxv5.New(pool), cfg)
	if err != nil {
		return nil, fmt.Errorf("creating river client: %w", err)
	}
	return client, nil
}
