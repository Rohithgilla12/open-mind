package enrich

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rohithgilla12/openmind/api/internal/docmd"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// DocConverter is the slice of docmd.Converter the pipeline needs; tests
// substitute a fake so enrich tests never boot wasm.
type DocConverter interface {
	Convert(ctx context.Context, data []byte, format docmd.Format) (docmd.Result, error)
}

// docFormats maps a stored asset's content type to the docmd format that reads
// it. Membership doubles as the routing predicate: an asset whose content type
// is absent here is not a document, so the map is the single place that
// decides what "document" means on the enrichment side.
var docFormats = map[string]docmd.Format{
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": docmd.FormatDocx,
	"application/vnd.oasis.opendocument.text":                                 docmd.FormatODT,
	"application/rtf":      docmd.FormatRTF,
	"application/epub+zip": docmd.FormatEPUB,
}

// DocFormatFor returns the docmd format for a stored asset content type, and
// whether that content type is a document Openmind converts.
func DocFormatFor(contentType string) (docmd.Format, bool) {
	f, ok := docFormats[contentType]
	return f, ok
}

// docCardType maps a document format onto a card type. An EPUB is a book; a
// word-processor file reads as an article, matching how uploaded PDFs are
// already classified.
func docCardType(format docmd.Format) string {
	if format == docmd.FormatEPUB {
		return "book"
	}
	return "article"
}

// persistDoc saves the converted document onto item (title/body/card type plus
// the structured Markdown) and runs the shared summarise/tag/embed tail.
func (p *Pipeline) persistDoc(ctx context.Context, userID uuid.UUID, item db.Item, res docmd.Result, title, cardType string) error {
	q := p.Store.Queries
	if err := q.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: item.ID,
		Title: title, Body: res.Text, LeadImageUrl: "", CardType: cardType,
	}); err != nil {
		return fmt.Errorf("saving document extraction: %w", err)
	}
	// Nothing reads body_markdown yet; it is kept so a future Markdown-aware
	// reader or EPUB export needs no re-extraction pass.
	if err := q.SetItemBodyMarkdown(ctx, db.SetItemBodyMarkdownParams{
		UserID: userID, ID: item.ID,
		BodyMarkdown: pgtype.Text{String: res.Markdown, Valid: true},
	}); err != nil {
		return fmt.Errorf("saving document markdown: %w", err)
	}
	return p.enrichText(ctx, userID, item.ID, title, res.Text)
}

// runUploadedDoc enriches an uploaded office document (url = "/assets/<uuid>"):
// read the stored blob, convert it to Markdown, persist the flattened text plus
// the Markdown, then the shared enrichment tail. Idempotent — conversion and
// flattening are pure functions of the stored bytes, so a re-run reproduces
// byte-identical state.
func (p *Pipeline) runUploadedDoc(ctx context.Context, userID uuid.UUID, item db.Item, asset db.Asset) error {
	if p.Doc == nil || p.Assets == nil {
		return p.failItem(ctx, userID, item.ID, "document conversion", fmt.Errorf("document support not configured"))
	}
	format, ok := DocFormatFor(asset.ContentType)
	if !ok {
		return p.failItem(ctx, userID, item.ID, "document conversion", fmt.Errorf("unsupported document type %q", asset.ContentType))
	}

	rc, err := p.Assets.Open(asset.ID)
	if err != nil {
		return p.failItem(ctx, userID, item.ID, "reading document asset", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return p.failItem(ctx, userID, item.ID, "reading document asset", err)
	}

	res, err := p.Doc.Convert(ctx, data, format)
	if err != nil {
		return p.failItem(ctx, userID, item.ID, "converting document", err)
	}

	// The upload handler already set the title to the filename stem; a heading
	// found inside the document is the better name when there is one.
	title := item.Title
	if res.Title != "" {
		title = res.Title
	}
	return p.persistDoc(ctx, userID, item, res, title, docCardType(format))
}
