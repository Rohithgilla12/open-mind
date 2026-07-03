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

// Hybrid runs FTS and (when available) vector search for the user's query,
// fuses the two rankings with RRF (k=60), and returns up to limit results
// ordered by descending fused score. Every query is scoped to userID.
func Hybrid(ctx context.Context, s *store.Store, p ai.Provider, userID uuid.UUID, q string, limit int) ([]Result, error) {
	const k = 60
	scores := map[uuid.UUID]float64{}
	items := map[uuid.UUID]db.Item{}

	fts, err := s.Queries.SearchFTS(ctx, db.SearchFTSParams{UserID: userID, WebsearchToTsquery: q, Limit: int32(limit * 2)})
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	for rank, row := range fts {
		scores[row.ID] += 1.0 / float64(k+rank+1)
		items[row.ID] = ftsRowToItem(row)
	}

	if vec, err := p.Embed(ctx, q); err == nil {
		vres, err := s.Queries.SearchVector(ctx, db.SearchVectorParams{UserID: userID, Embedding: pgvector.NewVector(vec), Limit: int32(limit * 2)})
		if err != nil {
			return nil, fmt.Errorf("vector search: %w", err)
		}
		for rank, row := range vres {
			scores[row.ID] += 1.0 / float64(k+rank+1)
			items[row.ID] = vecRowToItem(row)
		}
	} else if !errors.Is(err, ai.ErrNotSupported) {
		slog.Warn("query embedding failed; falling back to FTS only", "err", err)
	}

	ids := make([]uuid.UUID, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return scores[ids[i]] > scores[ids[j]] })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	results := make([]Result, 0, len(ids))
	for _, id := range ids {
		results = append(results, Result{Item: items[id], Score: scores[id]})
	}
	return results, nil
}

func ftsRowToItem(r db.SearchFTSRow) db.Item {
	return db.Item{
		ID: r.ID, UserID: r.UserID, Url: r.Url, Title: r.Title, Body: r.Body,
		LeadImageUrl: r.LeadImageUrl, Summary: r.Summary, Tags: r.Tags,
		CardType: r.CardType, Status: r.Status, SearchTsv: r.SearchTsv,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func vecRowToItem(r db.SearchVectorRow) db.Item {
	return db.Item{
		ID: r.ID, UserID: r.UserID, Url: r.Url, Title: r.Title, Body: r.Body,
		LeadImageUrl: r.LeadImageUrl, Summary: r.Summary, Tags: r.Tags,
		CardType: r.CardType, Status: r.Status, SearchTsv: r.SearchTsv,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
