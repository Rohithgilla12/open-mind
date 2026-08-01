// Package search implements hybrid retrieval over saved items, fusing
// Postgres full-text search with pgvector similarity via Reciprocal Rank
// Fusion (RRF). It degrades gracefully to FTS-only when the AI provider
// cannot produce a query embedding.
package search

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// Result is a single ranked hit: the stored item and its fused RRF score.
type Result struct {
	Item  db.Item
	Score float64
}

// rrfK is the Reciprocal Rank Fusion constant: each ranked list contributes
// 1/(k+rank+1) to an item's fused score, so top ranks dominate while lower
// ranks still nudge the total.
const rrfK = 60

// ruleResultLimit is the number of ranked results a Lens rule returns
// (mirrors the API package's defaultListLimit).
const ruleResultLimit = 50

// Hybrid runs FTS and (when available) vector search for the user's query,
// fuses the two rankings with RRF (k=60), and returns up to limit results
// ordered by descending fused score. Every query is scoped to userID.
//
// It is a text-only shortcut for Run; pass a colour term to Run directly for
// colour-proximity or combined search.
func Hybrid(ctx context.Context, s *store.Store, p ai.Provider, userID uuid.UUID, q string, limit int) ([]Result, error) {
	return Run(ctx, s, p, userID, q, "", nil, limit)
}

// Run is the home /search entrypoint: free-text and/or colour with an optional
// types filter, spanning the full river (scope=all). Prefer RunQuery for new
// callers that need domains or library scope.
func Run(ctx context.Context, s *store.Store, p ai.Provider, userID uuid.UUID, q, color string, types []string, limit int) ([]Result, error) {
	return RunQuery(ctx, s, p, userID, Query{
		Text: q, Color: color, Types: types, Scope: ScopeAll,
	}, limit)
}

// RunQuery executes q. Caller must set Scope (Lens: library; /search: all).
// Returns ErrBadColor for invalid colour; empty results if HasMatchSignal is false.
//
// Soft rank signals (Text, Color) fuse via RRF. Hard filters (Types, Domains,
// library scope) are applied in SQL before ranking. When Text and Color are
// both empty, results come from ListItemsMatching (newest first, unscored).
//
// With ScopeAll, results are partitioned library-first: Mind items always
// rank ahead of unkept feed-river matches. Within each partition ordering is
// by descending fused score. The limit is applied after the partition so
// library matches win the available slots.
func RunQuery(ctx context.Context, s *store.Store, p ai.Provider, userID uuid.UUID, q Query, limit int) ([]Result, error) {
	if !q.HasMatchSignal() {
		return nil, nil
	}

	libraryOnly := q.LibraryOnly()
	filterTypes := stringsOrNil(q.Types)
	filterDomains := stringsOrNil(q.Domains)

	// Filter-only path: types and/or domains, no text/colour rank signal.
	if q.Text == "" && q.Color == "" {
		rows, err := s.Queries.ListItemsMatching(ctx, db.ListItemsMatchingParams{
			UserID:        userID,
			LibraryOnly:   libraryOnly,
			FilterTypes:   filterTypes,
			FilterDomains: filterDomains,
			LimitCount:    int32(limit),
		})
		if err != nil {
			return nil, fmt.Errorf("list matching: %w", err)
		}
		results := make([]Result, 0, len(rows))
		for _, it := range rows {
			results = append(results, Result{Item: it})
		}
		return results, nil
	}

	// Resolve the colour up front so a bad term fails fast, before any query.
	var target rgb
	var haveColor bool
	if q.Color != "" {
		c, ok := parseColor(q.Color)
		if !ok {
			return nil, ErrBadColor
		}
		target, haveColor = c, true
	}

	scores := map[uuid.UUID]float64{}
	items := map[uuid.UUID]db.Item{}

	if q.Text != "" {
		fts, err := s.Queries.SearchFTS(ctx, db.SearchFTSParams{
			UserID:             userID,
			WebsearchToTsquery: q.Text,
			Limit:              int32(limit * 2),
			LibraryOnly:        libraryOnly,
			FilterTypes:        filterTypes,
			FilterDomains:      filterDomains,
		})
		if err != nil {
			return nil, fmt.Errorf("fts search: %w", err)
		}
		for rank, row := range fts {
			scores[row.ID] += 1.0 / float64(rrfK+rank+1)
			items[row.ID] = ftsRowToItem(row)
		}

		if vec, err := p.Embed(ctx, q.Text); err == nil {
			vres, err := s.Queries.SearchVector(ctx, db.SearchVectorParams{
				UserID:        userID,
				Embedding:     pgvector.NewVector(vec),
				Limit:         int32(limit * 2),
				LibraryOnly:   libraryOnly,
				FilterTypes:   filterTypes,
				FilterDomains: filterDomains,
			})
			if err != nil {
				// Degrade to FTS-only results rather than failing the request,
				// mirroring the embed-failure fallback below.
				slog.Warn("vector search failed; falling back to FTS only", "err", err)
			} else {
				for rank, row := range vres {
					scores[row.ID] += 1.0 / float64(rrfK+rank+1)
					items[row.ID] = vecRowToItem(row)
				}
			}
		} else if !errors.Is(err, ai.ErrNotSupported) {
			slog.Warn("query embedding failed; falling back to FTS only", "err", err)
		}
	}

	if haveColor {
		palette, err := s.Queries.ListItemsWithPalette(ctx, db.ListItemsWithPaletteParams{
			UserID:        userID,
			LibraryOnly:   libraryOnly,
			FilterTypes:   filterTypes,
			FilterDomains: filterDomains,
		})
		if err != nil {
			return nil, fmt.Errorf("palette search: %w", err)
		}
		ranked := rankByColor(palette, target)
		if len(ranked) > limit*2 {
			ranked = ranked[:limit*2]
		}
		for rank, it := range ranked {
			scores[it.ID] += 1.0 / float64(rrfK+rank+1)
			items[it.ID] = it
		}
	}

	ids := make([]uuid.UUID, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	// Library items first (meaningful under ScopeAll), then descending fused
	// score, with a deterministic tiebreak (newest first, then ID) so
	// equal-scored results order stably across requests.
	sort.SliceStable(ids, func(i, j int) bool {
		if q.Scope == ScopeAll {
			li, lj := inLibrary(items[ids[i]]), inLibrary(items[ids[j]])
			if li != lj {
				return li
			}
		}
		si, sj := scores[ids[i]], scores[ids[j]]
		if si != sj {
			return si > sj
		}
		ci, cj := items[ids[i]].CreatedAt, items[ids[j]].CreatedAt
		if !ci.Time.Equal(cj.Time) {
			return ci.Time.After(cj.Time)
		}
		return ids[i].String() > ids[j].String()
	})
	if len(ids) > limit {
		ids = ids[:limit]
	}
	results := make([]Result, 0, len(ids))
	for _, id := range ids {
		results = append(results, Result{Item: items[id], Score: scores[id]})
	}
	return results, nil
}

// RunLensRule executes a Lens Query and returns up to ruleResultLimit matches.
// Empty Scope defaults to ScopeLibrary (Mind only). Shared by GetLensItems and
// the send-to-Kindle Lens digest job so both see identical matches.
func RunLensRule(ctx context.Context, s *store.Store, p ai.Provider, userID uuid.UUID, q Query) ([]Result, error) {
	if q.Scope == "" {
		q.Scope = ScopeLibrary
	}
	return RunQuery(ctx, s, p, userID, q, ruleResultLimit)
}

// stringsOrNil returns nil when s is empty so sqlc narg skips the filter.
func stringsOrNil(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

// inLibrary reports whether an item belongs to the user's library (the Mind):
// anything saved directly, plus feed items the user explicitly kept. Unkept
// feed-river items are still searchable under ScopeAll but rank after every
// library match.
func inLibrary(it db.Item) bool {
	return !it.FeedID.Valid || it.KeptAt.Valid
}

func ftsRowToItem(r db.SearchFTSRow) db.Item {
	return db.Item{
		ID: r.ID, UserID: r.UserID, Url: r.Url, Title: r.Title, Body: r.Body,
		LeadImageUrl: r.LeadImageUrl, Summary: r.Summary, Tags: r.Tags,
		UserTags: r.UserTags, Palette: r.Palette,
		CardType: r.CardType, Status: r.Status, SearchTsv: r.SearchTsv,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		PinnedAt: r.PinnedAt, LastDriftedAt: r.LastDriftedAt,
		FeedID: r.FeedID, KeptAt: r.KeptAt,
		TaggedLocation: r.TaggedLocation, UrlHost: r.UrlHost,
		// PageCount is left as its zero value (invalid pgtype.Int4), so search
		// results always render pageCount as null even for PDF items.
	}
}

func vecRowToItem(r db.SearchVectorRow) db.Item {
	return db.Item{
		ID: r.ID, UserID: r.UserID, Url: r.Url, Title: r.Title, Body: r.Body,
		LeadImageUrl: r.LeadImageUrl, Summary: r.Summary, Tags: r.Tags,
		UserTags: r.UserTags, Palette: r.Palette,
		CardType: r.CardType, Status: r.Status, SearchTsv: r.SearchTsv,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		PinnedAt: r.PinnedAt, LastDriftedAt: r.LastDriftedAt,
		FeedID: r.FeedID, KeptAt: r.KeptAt,
		TaggedLocation: r.TaggedLocation, UrlHost: r.UrlHost,
		// PageCount is left as its zero value (invalid pgtype.Int4), so search
		// results always render pageCount as null even for PDF items.
	}
}
