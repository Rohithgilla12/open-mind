package feeds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// ErrAlreadySubscribed is returned by Add when the user is already subscribed to
// the given feed URL. Handlers map it to 409.
var ErrAlreadySubscribed = errors.New("already subscribed to feed")

const (
	// fetchTimeout bounds a single feed fetch.
	fetchTimeout = 20 * time.Second
	// maxFeedBytes caps how much of a feed body we read, bounding memory for a
	// hostile or runaway feed.
	maxFeedBytes = 8 << 20
	// maxEntriesPerPoll caps how many new items one feed contributes per poll, so
	// a huge feed can't enqueue an unbounded number of enrichment jobs at once.
	maxEntriesPerPoll = 100
	// maxStatusLen keeps last_status short enough to display in the UI.
	maxStatusLen = 200
)

// Service turns feed subscriptions into saved items. It fetches feeds with an
// SSRF-safe client, parses them, and creates pending article items through the
// normal enrichment pipeline. It is the single place feed polling logic lives;
// the API handlers and the periodic poll job both drive it.
type Service struct {
	Store      *store.Store
	HTTPClient *http.Client
	// River enqueues enrichment jobs. It may be nil (e.g. before the worker
	// binary wires it in, or in tests): when nil, item creation still succeeds
	// and enqueue is skipped.
	River *river.Client[pgx.Tx]
}

// NewService builds a Service with an SSRF-safe HTTP client. Callers set River
// afterwards (the worker binary threads it in; the API leaves it nil-or-set as
// needed).
func NewService(s *store.Store) *Service {
	return &Service{Store: s, HTTPClient: enrich.SafeHTTPClient(fetchTimeout)}
}

// Add subscribes the user to feedURL: it validates the URL, fetches and parses
// the feed, backfills the feed's current entries as pending items, and only
// then persists the feed row. Backfilled items are standalone (deduped by URL,
// idempotent) and don't need the feed row to exist, so ordering backfill before
// persistence means a failure at any point before CreateFeed leaves no feed row
// behind — a retry starts clean rather than tripping ErrAlreadySubscribed on a
// feed that never backfilled. A fetch, parse, or backfill failure returns an
// error and the feed is NOT persisted. A URL the user is already subscribed to
// returns ErrAlreadySubscribed.
func (s *Service) Add(ctx context.Context, userID uuid.UUID, feedURL string) (db.Feed, int, error) {
	feedURL = strings.TrimSpace(feedURL)
	if !validFeedURL(feedURL) {
		return db.Feed{}, 0, fmt.Errorf("invalid feed url %q", feedURL)
	}

	existing, err := s.Store.Queries.ListFeeds(ctx, userID)
	if err != nil {
		return db.Feed{}, 0, fmt.Errorf("listing feeds: %w", err)
	}
	for _, f := range existing {
		if f.Url == feedURL {
			return db.Feed{}, 0, ErrAlreadySubscribed
		}
	}

	parsed, err := s.fetchAndParse(ctx, feedURL)
	if err != nil {
		return db.Feed{}, 0, fmt.Errorf("fetching feed: %w", err)
	}

	added, err := s.saveEntries(ctx, userID, parsed.Entries)
	if err != nil {
		return db.Feed{}, 0, fmt.Errorf("backfilling feed: %w", err)
	}

	feed, err := s.Store.Queries.CreateFeed(ctx, db.CreateFeedParams{
		UserID:  userID,
		Url:     feedURL,
		Title:   parsed.Title,
		SiteUrl: parsed.SiteURL,
	})
	if err != nil {
		return db.Feed{}, 0, fmt.Errorf("creating feed: %w", err)
	}

	polled := nowTS()
	if err := s.Store.Queries.SetFeedPolled(ctx, db.SetFeedPolledParams{
		UserID: userID, ID: feed.ID, LastPolledAt: polled, LastStatus: "ok",
	}); err != nil {
		slog.Error("recording feed poll status", "feed_id", feed.ID, "err", err)
	}
	feed.LastPolledAt, feed.LastStatus = polled, "ok"
	return feed, added, nil
}

// Refresh re-polls one feed and saves any new entries. It never returns a fatal
// error for a bad feed: a fetch or parse failure is recorded in last_status
// ("error: …") and returns (0, nil) so a single broken feed can't break the
// poll loop.
func (s *Service) Refresh(ctx context.Context, feed db.Feed) (int, error) {
	parsed, err := s.fetchAndParse(ctx, feed.Url)
	if err != nil {
		s.recordStatus(ctx, feed, "error: "+shortErr(err))
		slog.Warn("feed refresh failed", "feed_id", feed.ID, "url", feed.Url, "err", err)
		return 0, nil
	}
	added, err := s.saveEntries(ctx, feed.UserID, parsed.Entries)
	if err != nil {
		s.recordStatus(ctx, feed, "error: "+shortErr(err))
		slog.Warn("feed refresh failed", "feed_id", feed.ID, "url", feed.Url, "err", err)
		return 0, nil
	}
	s.recordStatus(ctx, feed, "ok")
	return added, nil
}

// RefreshDue refreshes every feed whose last poll is null or older than
// olderThan. It is the periodic poller's entry point and continues past an
// individual feed's error.
func (s *Service) RefreshDue(ctx context.Context, olderThan time.Duration) error {
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-olderThan), Valid: true}
	due, err := s.Store.Queries.ListFeedsDue(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("listing due feeds: %w", err)
	}
	for _, feed := range due {
		if _, err := s.Refresh(ctx, feed); err != nil {
			slog.Error("refreshing feed", "feed_id", feed.ID, "err", err)
		}
	}
	return nil
}

// saveEntries creates a pending article item for each entry URL that is not
// already an item for this user (and not repeated within the batch), enqueuing
// enrichment for each. Enqueue is best-effort — a failed enqueue is logged, not
// fatal, since the item is already saved and can be re-enriched later.
func (s *Service) saveEntries(ctx context.Context, userID uuid.UUID, entries []Entry) (int, error) {
	existing, err := s.Store.Queries.ListItemURLs(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("listing item urls: %w", err)
	}
	seen := make(map[string]bool, len(existing)+len(entries))
	for _, u := range existing {
		seen[u] = true
	}

	added := 0
	for _, e := range entries {
		if added >= maxEntriesPerPoll {
			slog.Warn("feed entries truncated at cap", "cap", maxEntriesPerPoll, "user_id", userID)
			break
		}
		if e.URL == "" || seen[e.URL] {
			continue
		}
		seen[e.URL] = true

		item, err := s.Store.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: e.URL})
		if err != nil {
			return added, fmt.Errorf("creating item: %w", err)
		}
		if s.River != nil {
			if _, err := s.River.Insert(ctx, jobs.EnrichArgs{UserID: userID, ItemID: item.ID}, nil); err != nil {
				slog.Error("enqueueing enrichment for feed item", "item_id", item.ID, "err", err)
			}
		} else {
			slog.Debug("river client not set; skipping enrichment enqueue", "item_id", item.ID)
		}
		added++
	}
	return added, nil
}

// fetchAndParse fetches feedURL with the SSRF-safe client and parses the body.
func (s *Service) fetchAndParse(ctx context.Context, feedURL string) (Feed, error) {
	client := s.HTTPClient
	if client == nil {
		client = enrich.SafeHTTPClient(fetchTimeout)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return Feed{}, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml;q=0.9, */*;q=0.8")
	req.Header.Set("User-Agent", "openmind-feeds/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return Feed{}, fmt.Errorf("requesting feed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Feed{}, fmt.Errorf("feed returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes))
	if err != nil {
		return Feed{}, fmt.Errorf("reading feed body: %w", err)
	}
	parsed, err := Parse(data, feedURL)
	if err != nil {
		return Feed{}, fmt.Errorf("parsing feed: %w", err)
	}
	return parsed, nil
}

// recordStatus stamps last_polled_at (now) and last_status on a feed. A write
// failure is logged, not returned — the poll loop must keep going.
func (s *Service) recordStatus(ctx context.Context, feed db.Feed, status string) {
	if err := s.Store.Queries.SetFeedPolled(ctx, db.SetFeedPolledParams{
		UserID: feed.UserID, ID: feed.ID, LastPolledAt: nowTS(), LastStatus: status,
	}); err != nil {
		slog.Error("recording feed poll status", "feed_id", feed.ID, "err", err)
	}
}

// validFeedURL accepts only absolute http(s) URLs, mirroring the API's URL check.
func validFeedURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// shortErr trims an error message so it fits in last_status.
func shortErr(err error) string {
	msg := err.Error()
	if len(msg) > maxStatusLen {
		msg = msg[:maxStatusLen]
	}
	return msg
}

func nowTS() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now(), Valid: true}
}
