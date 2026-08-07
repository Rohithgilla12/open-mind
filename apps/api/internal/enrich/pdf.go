package enrich

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rohithgilla12/openmind/api/internal/pdftext"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// PDFTexter is the slice of pdftext.Extractor the pipeline needs; tests
// substitute a fake so enrich tests never boot wasm.
type PDFTexter interface {
	Extract(ctx context.Context, data []byte) (pdftext.Result, error)
}

// isPDFURL sniffs whether url serves a PDF: a HEAD Content-Type of
// application/pdf, or (when HEAD is inconclusive) a ranged GET whose first
// bytes are "%PDF-". Inconclusive/never errors — falls through to normal
// extraction, mirroring isImageURL.
func isPDFURL(ctx context.Context, c *http.Client, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false
	}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/pdf") {
		return true
	}
	if ct != "" && ct != "application/octet-stream" {
		return false
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Range", "bytes=0-7")
	resp, err = c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	head, _ := io.ReadAll(io.LimitReader(resp.Body, 8))
	return bytes.HasPrefix(head, []byte("%PDF-"))
}

// pdfURLTitle derives a fallback title from a URL's filename stem, mirroring
// imageTitle, for use when neither the extracted PDF metadata nor the item
// already carries a title.
func pdfURLTitle(rawURL string) string {
	return imageTitle(rawURL)
}

// failItem marks the item failed and wraps err with stage context. It is
// shared by the PDF and document paths so all of them report failures the same
// way: the item and any already-created asset row survive, only the status
// flips to "failed".
func (p *Pipeline) failItem(ctx context.Context, userID uuid.UUID, itemID uuid.UUID, stage string, err error) error {
	if serr := p.Store.Queries.SetItemStatus(ctx, db.SetItemStatusParams{UserID: userID, ID: itemID, Status: "failed"}); serr != nil {
		return fmt.Errorf("marking failed after %s error %v: %w", stage, err, serr)
	}
	return fmt.Errorf("%s: %w", stage, err)
}

// persistPDF saves the extracted PDF result onto item (title/body/page count,
// card type article) and runs the shared summarise/tag/embed tail. Shared by
// runUploadedPDF and runURLPDF.
func (p *Pipeline) persistPDF(ctx context.Context, userID uuid.UUID, item db.Item, res pdftext.Result, title string) error {
	q := p.Store.Queries
	if err := q.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: item.ID,
		Title: title, Body: res.Text, LeadImageUrl: "", CardType: "article",
	}); err != nil {
		return fmt.Errorf("saving pdf extraction: %w", err)
	}
	if err := q.SetItemPageCount(ctx, db.SetItemPageCountParams{
		UserID: userID, ID: item.ID, PageCount: pgtype.Int4{Int32: int32(res.Pages), Valid: true},
	}); err != nil {
		return fmt.Errorf("saving pdf page count: %w", err)
	}
	return p.enrichText(ctx, userID, item.ID, title, res.Text)
}

// runUploadedPDF enriches an uploaded PDF item (url = "/assets/<uuid>"): read
// the stored blob, extract text, persist body + page count, then the shared
// enrichment tail. Idempotent — re-running re-extracts to the same state.
func (p *Pipeline) runUploadedPDF(ctx context.Context, userID uuid.UUID, item db.Item) error {
	if p.PDF == nil || p.Assets == nil {
		return p.failItem(ctx, userID, item.ID, "pdf extraction", fmt.Errorf("pdf support not configured"))
	}
	// Defence in depth: resolve the asset via the user-scoped store query
	// rather than trusting the UUID parsed straight out of item.Url. Today
	// item.Url is always this item's own "/assets/<uuid>", but if a future
	// item type ever carried a foreign /assets/<uuid> URL, parsing it
	// directly would let this job read another tenant's blob.
	asset, err := p.Store.Queries.GetAssetByItem(ctx, db.GetAssetByItemParams{
		UserID: userID, ItemID: pgtype.UUID{Bytes: item.ID, Valid: true},
	})
	if err != nil {
		return p.failItem(ctx, userID, item.ID, "looking up pdf asset", err)
	}
	assetID := asset.ID
	rc, err := p.Assets.Open(assetID)
	if err != nil {
		return p.failItem(ctx, userID, item.ID, "reading pdf asset", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return p.failItem(ctx, userID, item.ID, "reading pdf asset", err)
	}
	res, err := p.PDF.Extract(ctx, data)
	if err != nil {
		return p.failItem(ctx, userID, item.ID, "extracting pdf text", err)
	}
	title := item.Title
	if res.Title != "" {
		title = res.Title
	}
	return p.persistPDF(ctx, userID, item, res, title)
}

// storePDFBlob clears any leftover partial blob for assetID (Put uses
// O_EXCL and would otherwise fail on a stale file from a prior failed run),
// writes data, and records the byte size. Used both for a freshly created
// asset row and for self-healing one left byte_size==0 by a prior failure.
func (p *Pipeline) storePDFBlob(ctx context.Context, userID uuid.UUID, assetID uuid.UUID, data []byte) error {
	if err := p.Assets.Delete(assetID); err != nil {
		return fmt.Errorf("clearing stale pdf asset blob: %w", err)
	}
	size, err := p.Assets.Put(assetID, bytes.NewReader(data), maxResponseBytes)
	if err != nil {
		return fmt.Errorf("writing pdf asset blob: %w", err)
	}
	if err := p.Store.Queries.SetAssetByteSize(ctx, db.SetAssetByteSizeParams{UserID: userID, ID: assetID, ByteSize: size}); err != nil {
		return fmt.Errorf("recording pdf asset size: %w", err)
	}
	return nil
}

// runURLPDF enriches a saved URL that serves a PDF: fetch (size-capped), store
// as an asset (re-using this item's existing pdf asset on re-run), extract,
// persist. The original URL stays the item's url; the asset is reachable from
// the web detail page via the item→asset relation.
func (p *Pipeline) runURLPDF(ctx context.Context, userID uuid.UUID, item db.Item) error {
	if p.PDF == nil || p.Assets == nil {
		return p.failItem(ctx, userID, item.ID, "pdf extraction", fmt.Errorf("pdf support not configured"))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.Url, nil)
	if err != nil {
		return p.failItem(ctx, userID, item.ID, "fetching pdf", err)
	}
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return p.failItem(ctx, userID, item.ID, "fetching pdf", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return p.failItem(ctx, userID, item.ID, "reading pdf response", err)
	}
	if len(data) > maxResponseBytes {
		return p.failItem(ctx, userID, item.ID, "reading pdf response", fmt.Errorf("response exceeds %d bytes", maxResponseBytes))
	}
	// TOCTOU guard: isPDFURL's HEAD/ranged-GET check ran before this fetch and
	// can no longer be trusted (the server may have swapped what it serves in
	// between). Re-verify the actual fetched body before it ever reaches
	// storage — a mismatch fails the item, no asset row created.
	if resp.StatusCode != http.StatusOK {
		return p.failItem(ctx, userID, item.ID, "reading pdf response", fmt.Errorf("unexpected status %d", resp.StatusCode))
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return p.failItem(ctx, userID, item.ID, "reading pdf response", fmt.Errorf("response is not a pdf"))
	}

	q := p.Store.Queries
	itemIDPg := pgtype.UUID{Bytes: item.ID, Valid: true}
	asset, err := q.GetAssetByItem(ctx, db.GetAssetByItemParams{UserID: userID, ItemID: itemIDPg})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		asset, err = q.CreateAsset(ctx, db.CreateAssetParams{
			UserID:           userID,
			ItemID:           itemIDPg,
			ContentType:      "application/pdf",
			OriginalFilename: path.Base(item.Url),
		})
		if err != nil {
			return p.failItem(ctx, userID, item.ID, "creating pdf asset", err)
		}
		if err := p.storePDFBlob(ctx, userID, asset.ID, data); err != nil {
			// Best-effort cleanup: drop the asset row so a retry doesn't find a
			// row pointing at a blob that was never written and skip storage
			// (which would otherwise leave the item enriched with a 404ing
			// asset forever).
			if delErr := q.DeleteAsset(ctx, db.DeleteAssetParams{UserID: userID, ID: asset.ID}); delErr != nil {
				return p.failItem(ctx, userID, item.ID, "storing pdf asset", fmt.Errorf("%w (cleanup also failed: %v)", err, delErr))
			}
			return p.failItem(ctx, userID, item.ID, "storing pdf asset", err)
		}
	case err != nil:
		return p.failItem(ctx, userID, item.ID, "looking up pdf asset", err)
	default:
		// Self-heal: a prior run created the asset row but failed before (or
		// during) writing the blob or recording its size — byte_size is still
		// 0, or the row is fine but the blob is missing from the FSStore.
		// Re-Put instead of skipping storage, so the item never enriches
		// while pointing at an asset that 404s forever.
		if asset.ByteSize == 0 || !p.Assets.Exists(asset.ID) {
			if err := p.storePDFBlob(ctx, userID, asset.ID, data); err != nil {
				return p.failItem(ctx, userID, item.ID, "storing pdf asset", err)
			}
		}
	}

	res, err := p.PDF.Extract(ctx, data)
	if err != nil {
		return p.failItem(ctx, userID, item.ID, "extracting pdf text", err)
	}
	title := item.Title
	if res.Title != "" {
		title = res.Title
	} else if title == "" {
		title = pdfURLTitle(item.Url)
	}
	return p.persistPDF(ctx, userID, item, res, title)
}
