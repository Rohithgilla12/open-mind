package enrich

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/assets"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// Pipeline runs the staged enrichment for a saved item: extract → classify →
// summarise → tag → embed. Every stage is idempotent and safe to re-run; a
// failed stage never corrupts the saved item.
type Pipeline struct {
	Store     *store.Store
	AI        ai.Provider
	Extractor Extractor
	// HTTPClient is used for extractor-independent probes (the image-URL HEAD
	// sniff). When nil it defaults to a SafeHTTPClient; tests inject an
	// httptest client.
	HTTPClient *http.Client
	// Assets backs palette extraction for uploaded images: it reads the stored
	// blob referenced by an "/assets/<uuid>" lead image URL. When nil, palette
	// extraction is skipped entirely.
	Assets *assets.FSStore
}

// httpClient returns the pipeline's HTTP client, defaulting to a SafeHTTPClient
// so user-supplied URLs can never reach internal addresses.
func (p *Pipeline) httpClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return SafeHTTPClient(10 * time.Second)
}

// Run enriches the item identified by userID/itemID. It is safe to call more
// than once: a second run reproduces the same final state.
func (p *Pipeline) Run(ctx context.Context, userID, itemID uuid.UUID) error {
	q := p.Store.Queries
	item, err := q.GetItem(ctx, db.GetItemParams{UserID: userID, ID: itemID})
	if err != nil {
		return fmt.Errorf("loading item %s: %w", itemID, err)
	}

	// Uploaded images have no source URL but carry a local asset path in
	// LeadImageUrl. They must be routed before the note check (which also keys
	// on an empty URL) and must never touch the extractor or HTTP client — an
	// /assets/ path is not fetchable.
	if item.Url == "" && strings.HasPrefix(item.LeadImageUrl, "/assets/") {
		return p.runUploadedImage(ctx, userID, item)
	}

	if item.Url == "" {
		return p.runNote(ctx, userID, item)
	}

	// Image URLs bypass the extractor entirely: there is no article to pull, so
	// we render the image directly as an image card. The sniff never fails the
	// job — an inconclusive result falls through to normal extraction.
	if ok, _ := isImageURL(ctx, p.httpClient(), item.Url); ok {
		title := imageTitle(item.Url)
		if err := q.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
			UserID: userID, ID: itemID,
			Title: title, Body: "", LeadImageUrl: item.Url, CardType: "image",
		}); err != nil {
			return fmt.Errorf("saving image extraction: %w", err)
		}
		return p.enrichText(ctx, userID, itemID, title, title)
	}

	ex, err := p.Extractor.Extract(ctx, item.Url)
	if err != nil {
		if serr := q.SetItemStatus(ctx, db.SetItemStatusParams{UserID: userID, ID: itemID, Status: "failed"}); serr != nil {
			return fmt.Errorf("marking failed after extract error %v: %w", err, serr)
		}
		return fmt.Errorf("extracting %s: %w", item.Url, err)
	}
	cardType := Classify(item.Url, ex)
	if err := q.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: itemID,
		Title: ex.Title, Body: ex.Body, LeadImageUrl: ex.LeadImageURL, CardType: cardType,
	}); err != nil {
		return fmt.Errorf("saving extraction: %w", err)
	}

	return p.enrichText(ctx, userID, itemID, ex.Title, ex.Body)
}

// runNote enriches a note item (no URL): it skips extraction, classifies as a
// note, derives the title from the first line of the body, and runs the shared
// summarise/tag/embed path over the note text.
func (p *Pipeline) runNote(ctx context.Context, userID uuid.UUID, item db.Item) error {
	q := p.Store.Queries
	title := noteTitle(item.Body)
	if err := q.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: item.ID,
		Title: title, Body: item.Body, LeadImageUrl: "", CardType: "note",
	}); err != nil {
		return fmt.Errorf("saving note metadata: %w", err)
	}
	return p.enrichText(ctx, userID, item.ID, title, item.Body)
}

// runUploadedImage enriches a locally-uploaded image (no source URL): it skips
// extraction and any fetch, keeps the asset path in LeadImageUrl, classifies as
// an image card, and runs the shared enrichment tail over the title. The title
// is set at upload time; it falls back to "image" when absent. Idempotent.
func (p *Pipeline) runUploadedImage(ctx context.Context, userID uuid.UUID, item db.Item) error {
	q := p.Store.Queries
	title := item.Title
	if title == "" {
		title = "image"
	}
	if err := q.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: item.ID,
		Title: title, Body: "", LeadImageUrl: item.LeadImageUrl, CardType: "image",
	}); err != nil {
		return fmt.Errorf("saving uploaded-image metadata: %w", err)
	}
	p.extractPalette(ctx, userID, item.ID, item.LeadImageUrl)
	return p.enrichText(ctx, userID, item.ID, title, title)
}

// extractPalette reads the uploaded asset blob referenced by an "/assets/<uuid>"
// lead image URL and stores up to five dominant colours. It never fails the job:
// a nil asset store, unparseable path, unreadable blob, or undecodable image
// simply leaves the palette empty. Extraction is deterministic, so a re-run
// reproduces the same palette (idempotent).
//
// Only uploaded images are covered — their bytes are already on local disk.
// External image-URL cards are skipped: their bytes are not retained by the
// HEAD sniff, and adding a fetch here is out of scope.
func (p *Pipeline) extractPalette(ctx context.Context, userID, itemID uuid.UUID, leadImageURL string) {
	if p.Assets == nil {
		return
	}
	id, ok := assetIDFromURL(leadImageURL)
	if !ok {
		return
	}
	rc, err := p.Assets.Open(id)
	if err != nil {
		slog.Warn("palette: opening asset", "item_id", itemID, "err", err)
		return
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		slog.Warn("palette: reading asset", "item_id", itemID, "err", err)
		return
	}
	colours, err := DominantColors(data, 5)
	if err != nil || len(colours) == 0 {
		return
	}
	if err := p.Store.Queries.SetItemPalette(ctx, db.SetItemPaletteParams{UserID: userID, ID: itemID, Palette: colours}); err != nil {
		slog.Warn("palette: saving", "item_id", itemID, "err", err)
	}
}

// assetIDFromURL parses the asset UUID out of an "/assets/<uuid>" lead image URL.
func assetIDFromURL(leadImageURL string) (uuid.UUID, bool) {
	const prefix = "/assets/"
	if !strings.HasPrefix(leadImageURL, prefix) {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(strings.TrimPrefix(leadImageURL, prefix))
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// enrichText runs the summarise → tag → embed → status tail shared by the URL
// and note paths. Every stage is idempotent; the ErrNotSupported and dimension
// guards keep the noop provider and mismatched embeddings from failing the job.
func (p *Pipeline) enrichText(ctx context.Context, userID, itemID uuid.UUID, title, body string) error {
	q := p.Store.Queries
	summary, err := p.AI.Summarise(ctx, title, body)
	if err != nil {
		return fmt.Errorf("summarising: %w", err) // River retries; save stays intact
	}
	tags, err := p.AI.Tag(ctx, title, body)
	if err != nil {
		return fmt.Errorf("tagging: %w", err)
	}
	if tags == nil {
		tags = []string{} // tags column is NOT NULL; noop provider returns nil
	}
	if err := q.UpdateItemEnrichment(ctx, db.UpdateItemEnrichmentParams{UserID: userID, ID: itemID, Summary: summary, Tags: tags}); err != nil {
		return fmt.Errorf("saving enrichment: %w", err)
	}

	embedInput := title + "\n" + summary + "\n" + body
	vec, err := p.AI.Embed(ctx, embedInput)
	switch {
	case errors.Is(err, ai.ErrNotSupported):
		// noop provider: FTS-only mode, no embedding row.
	case err != nil:
		return fmt.Errorf("embedding: %w", err)
	case len(vec) != ai.EmbedDims:
		// Wrong dimensionality would break the pgvector column; skip the
		// embedding rather than failing the job. The item still ends enriched
		// (FTS-searchable), and a re-run can backfill the vector.
		slog.Warn("skipping embedding: unexpected dimension", "item_id", itemID, "got", len(vec), "want", ai.EmbedDims)
	default:
		if err := q.UpsertEmbedding(ctx, db.UpsertEmbeddingParams{ItemID: itemID, UserID: userID, Embedding: pgvector.NewVector(vec)}); err != nil {
			return fmt.Errorf("saving embedding: %w", err)
		}
	}
	return q.SetItemStatus(ctx, db.SetItemStatusParams{UserID: userID, ID: itemID, Status: "enriched"})
}

// noteTitle derives a card title from a note body: the first non-empty-trimmed
// line, truncated to 80 runes.
func noteTitle(body string) string {
	line := body
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		line = body[:i]
	}
	line = strings.TrimSpace(line)
	r := []rune(line)
	if len(r) > 80 {
		return string(r[:80])
	}
	return line
}
