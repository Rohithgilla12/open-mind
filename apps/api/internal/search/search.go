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

// ruleResultLimit and ruleListCap mirror the API package's defaultListLimit
// and maxListLimit: the number of ranked results a Lens rule returns and the
// number of recent items scanned for a types-only rule.
const (
	ruleResultLimit = 50
	ruleListCap     = 200
)

// Hybrid runs FTS and (when available) vector search for the user's query,
// fuses the two rankings with RRF (k=60), and returns up to limit results
// ordered by descending fused score. Every query is scoped to userID.
//
// It is a text-only shortcut for Run; pass a colour term to Run directly for
// colour-proximity or combined search.
func Hybrid(ctx context.Context, s *store.Store, p ai.Provider, userID uuid.UUID, q string, limit int) ([]Result, error) {
	return Run(ctx, s, p, userID, q, "", nil, limit)
}

// Run fuses up to three ranked signals with RRF and returns up to limit results
// ordered by descending fused score, scoped to userID:
//   - full-text search over q (when q is non-empty),
//   - pgvector similarity over q's embedding (when q is non-empty and the
//     provider can embed), and
//   - palette colour proximity to color (when color is non-empty).
//
// When types is non-empty the fused results are narrowed to items of those card
// types before the limit is applied, so ranking is computed over all matches
// and only then filtered.
//
// At least one of q or color should be non-empty; with both empty it returns no
// results. An unrecognised color yields ErrBadColor before any query runs.
func Run(ctx context.Context, s *store.Store, p ai.Provider, userID uuid.UUID, q, color string, types []string, limit int) ([]Result, error) {
	// Resolve the colour up front so a bad term fails fast, before any query.
	var target rgb
	var haveColor bool
	if color != "" {
		c, ok := parseColor(color)
		if !ok {
			return nil, ErrBadColor
		}
		target, haveColor = c, true
	}

	scores := map[uuid.UUID]float64{}
	items := map[uuid.UUID]db.Item{}

	if q != "" {
		fts, err := s.Queries.SearchFTS(ctx, db.SearchFTSParams{UserID: userID, WebsearchToTsquery: q, Limit: int32(limit * 2)})
		if err != nil {
			return nil, fmt.Errorf("fts search: %w", err)
		}
		for rank, row := range fts {
			scores[row.ID] += 1.0 / float64(rrfK+rank+1)
			items[row.ID] = ftsRowToItem(row)
		}

		if vec, err := p.Embed(ctx, q); err == nil {
			vres, err := s.Queries.SearchVector(ctx, db.SearchVectorParams{UserID: userID, Embedding: pgvector.NewVector(vec), Limit: int32(limit * 2)})
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
		palette, err := s.Queries.ListItemsWithPalette(ctx, userID)
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
	// Descending fused score, with a deterministic tiebreak (newest first,
	// then ID) so equal-scored results order stably across requests.
	sort.SliceStable(ids, func(i, j int) bool {
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
	if len(types) > 0 {
		allowed := make(map[string]bool, len(types))
		for _, t := range types {
			allowed[t] = true
		}
		filtered := make([]uuid.UUID, 0, len(ids))
		for _, id := range ids {
			if allowed[items[id].CardType] {
				filtered = append(filtered, id)
			}
		}
		ids = filtered
	}
	if len(ids) > limit {
		ids = ids[:limit]
	}
	results := make([]Result, 0, len(ids))
	for _, id := range ids {
		results = append(results, Result{Item: items[id], Score: scores[id]})
	}
	return results, nil
}

// RunLensRule executes a canonical Lens rule (q, colour, and/or card types)
// and returns up to ruleResultLimit matches. With a text or colour signal it
// delegates to Run, the same hybrid engine backing /search. A types-only rule
// has no ranking signal, so it falls back to the caller's most recent items,
// filtered to the allowed types. Shared by GetLensItems and the
// send-to-Kindle Lens digest job so both see identical matches.
func RunLensRule(ctx context.Context, s *store.Store, p ai.Provider, userID uuid.UUID, q, color string, types []string) ([]Result, error) {
	if q != "" || color != "" {
		return Run(ctx, s, p, userID, q, color, types, ruleResultLimit)
	}
	items, err := s.Queries.ListItems(ctx, db.ListItemsParams{UserID: userID, Limit: ruleListCap})
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, t := range types {
		allowed[t] = true
	}
	results := make([]Result, 0, ruleResultLimit)
	for _, it := range items {
		if !allowed[it.CardType] {
			continue
		}
		results = append(results, Result{Item: it})
		if len(results) >= ruleResultLimit {
			break
		}
	}
	return results, nil
}

func ftsRowToItem(r db.SearchFTSRow) db.Item {
	return db.Item{
		ID: r.ID, UserID: r.UserID, Url: r.Url, Title: r.Title, Body: r.Body,
		LeadImageUrl: r.LeadImageUrl, Summary: r.Summary, Tags: r.Tags,
		UserTags: r.UserTags, Palette: r.Palette,
		CardType: r.CardType, Status: r.Status, SearchTsv: r.SearchTsv,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		PinnedAt: r.PinnedAt, LastDriftedAt: r.LastDriftedAt,
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
	}
}
