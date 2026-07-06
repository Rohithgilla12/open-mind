package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"
)

// pollInterval is how stale a feed's last poll must be before the periodic job
// refreshes it. It matches the periodic schedule, so each run picks up feeds
// that have gone a full interval without a poll.
const pollInterval = 30 * time.Minute

// FeedRefresher is the slice of the feed service the poll worker needs. It is
// declared here (rather than importing internal/feeds) because internal/feeds
// already imports this package for EnrichArgs; the interface breaks that cycle.
// *feeds.Service satisfies it.
type FeedRefresher interface {
	RefreshDue(ctx context.Context, olderThan time.Duration) error
}

// PollFeedsArgs is the River job payload for the periodic feed poll. It carries
// no state: the worker lists every due feed itself.
type PollFeedsArgs struct{}

// Kind identifies the job type in River.
func (PollFeedsArgs) Kind() string { return "poll_feeds" }

// PollFeedsWorker refreshes every feed due for a poll. It runs on the periodic
// schedule registered in NewRiverClient.
type PollFeedsWorker struct {
	river.WorkerDefaults[PollFeedsArgs]
	Service FeedRefresher
}

// Work refreshes all due feeds. A single broken feed never fails the job (the
// service records the error in last_status and continues), so River only retries
// on an infrastructure error such as the initial due-list query failing.
func (w *PollFeedsWorker) Work(ctx context.Context, _ *river.Job[PollFeedsArgs]) error {
	return w.Service.RefreshDue(ctx, pollInterval)
}
