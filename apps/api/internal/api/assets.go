package api

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/rohithgilla12/openmind/api/internal/assets"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// sniffLen is the number of leading bytes http.DetectContentType inspects.
const sniffLen = 512

// allowedImageTypes is the content-type allowlist for uploads. SVG is
// deliberately excluded: it can carry script and would enable stored XSS.
var allowedImageTypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/gif":  {},
	"image/webp": {},
	"image/avif": {},
}

// detectImageType returns the sniffed content-type of buf, restricted to image
// formats we can identify. Go's http.DetectContentType has no AVIF rule, so we
// add a minimal ISOBMFF ftyp probe to honour the allowlist. It never trusts the
// client-supplied part header.
func detectImageType(buf []byte) string {
	if ct := detectAVIF(buf); ct != "" {
		return ct
	}
	ct := http.DetectContentType(buf)
	// DetectContentType returns e.g. "image/png; charset=..." only for text;
	// image types come back bare, but strip params defensively.
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct
}

// detectAVIF reports "image/avif" when buf is an ISOBMFF file whose ftyp box
// carries an AVIF brand. Returns "" otherwise.
func detectAVIF(buf []byte) string {
	if len(buf) < 12 || string(buf[4:8]) != "ftyp" {
		return ""
	}
	brand := string(buf[8:12])
	if brand == "avif" || brand == "avis" {
		return "image/avif"
	}
	return ""
}

// isPDF reports whether the sniffed content-type identifies a PDF. PDFs are
// stored verbatim (no metadata stripping): that's out of scope, documented in
// self-hosting.
func isPDF(contentType string) bool { return contentType == "application/pdf" }

// CreateAsset accepts a multipart image upload (field "file"), validates it by
// sniffing the leading bytes against an allowlist (never the client-supplied
// type), stores the blob on disk under a UUID filename, creates a linked image
// item, and queues enrichment. Capture stays fast: no AI runs inline.
func (s *Server) CreateAsset(w http.ResponseWriter, r *http.Request) {
	// Cap the whole request body. MaxBytesReader makes reads past the cap fail,
	// which surfaces as a 413 below.
	r.Body = http.MaxBytesReader(w, r.Body, s.assetMaxByte)

	file, header, err := r.FormFile("file")
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds size limit")
			return
		}
		writeError(w, http.StatusBadRequest, "missing multipart file field")
		return
	}
	defer func() { _ = file.Close() }()

	// Read the whole (MaxBytesReader-bounded) upload so we can sniff, validate,
	// and strip metadata before touching the database. Exceeding the cap yields
	// an *http.MaxBytesError → 413.
	data, err := io.ReadAll(file)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds size limit")
			return
		}
		writeError(w, http.StatusBadRequest, "could not read upload")
		return
	}

	head := data
	if len(head) > sniffLen {
		head = head[:sniffLen]
	}
	contentType := detectImageType(head)
	_, isImage := allowedImageTypes[contentType]
	// Office documents are ZIP containers or RTF, neither of which
	// detectImageType can name, so they are sniffed separately over the whole
	// upload: telling .docx from .odt needs the archive's central directory,
	// which does not live in the first 512 bytes.
	if !isImage && !isPDF(contentType) {
		if docType := detectDocType(data); docType != "" {
			contentType = docType
		}
	}
	if !isImage && !isPDF(contentType) && !isDocument(contentType) {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported file type")
		return
	}

	// Strip metadata (EXIF/XMP/IPTC/comments) losslessly before any row is
	// created, so a malformed image never leaves orphan rows behind. PDFs are
	// stored verbatim.
	stripped := data
	if isImage {
		var err error
		stripped, err = assets.StripMetadata(contentType, data)
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not process image")
			return
		}
	}

	ctx := r.Context()
	uid := userID(ctx)

	// Create the item first (url="" so the pipeline's uploaded-image branch runs),
	// then the asset row to obtain its UUID (== on-disk filename), then persist
	// the blob, then set the item's lead image + title. Order matters so the
	// enqueued job sees a fully-formed item.
	item, err := s.store.Queries.CreateItem(ctx, db.CreateItemParams{UserID: uid})
	if err != nil {
		slog.Error("creating image item", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save item")
		return
	}

	asset, err := s.store.Queries.CreateAsset(ctx, db.CreateAssetParams{
		UserID:           uid,
		ItemID:           pgtype.UUID{Bytes: item.ID, Valid: true},
		ContentType:      contentType,
		OriginalFilename: header.Filename,
	})
	if err != nil {
		slog.Error("creating asset row", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save asset")
		return
	}

	size, err := s.assetStore.Put(asset.ID, bytes.NewReader(stripped), s.assetMaxByte)
	if err != nil {
		if errors.Is(err, assets.ErrTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds size limit")
			return
		}
		slog.Error("writing asset blob", "asset_id", asset.ID, "err", err)
		// Best-effort cleanup: drop the item so no ghost card is left behind.
		// The asset row is removed via ON DELETE CASCADE. Failure here is logged
		// but does not change the original 500 the caller receives.
		if _, delErr := s.store.Queries.DeleteItem(ctx, db.DeleteItemParams{UserID: uid, ID: item.ID}); delErr != nil {
			slog.Error("cleaning up orphan item after blob-write failure", "item_id", item.ID, "err", delErr)
		}
		writeError(w, http.StatusInternalServerError, "could not store file")
		return
	}
	if err := s.store.Queries.SetAssetByteSize(ctx, db.SetAssetByteSizeParams{UserID: uid, ID: asset.ID, ByteSize: size}); err != nil {
		slog.Error("recording asset size", "asset_id", asset.ID, "err", err)
	}

	title := filenameStem(header.Filename)
	// PDFs and office documents both carry their asset path in the item's URL,
	// which is what routes them to the right enrichment path. Only the card
	// type differs: an EPUB is a book, everything else reads as an article.
	if isPDF(contentType) || isDocument(contentType) {
		cardType := "article"
		if isDocument(contentType) {
			cardType = docCardTypeFor(contentType)
		}
		if err := s.store.Queries.SetItemURL(ctx, db.SetItemURLParams{
			UserID: uid, ID: item.ID, Url: "/assets/" + asset.ID.String(),
		}); err != nil {
			slog.Error("setting document item url", "item_id", item.ID, "err", err)
			writeError(w, http.StatusInternalServerError, "could not save item")
			return
		}
		if err := s.store.Queries.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
			UserID: uid, ID: item.ID,
			Title: title, Body: "", LeadImageUrl: "", CardType: cardType,
		}); err != nil {
			slog.Error("setting document item metadata", "item_id", item.ID, "err", err)
			writeError(w, http.StatusInternalServerError, "could not save item")
			return
		}
	} else {
		if err := s.store.Queries.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
			UserID: uid, ID: item.ID,
			Title: title, Body: "", LeadImageUrl: "/assets/" + asset.ID.String(), CardType: "image",
		}); err != nil {
			slog.Error("setting image item metadata", "item_id", item.ID, "err", err)
			writeError(w, http.StatusInternalServerError, "could not save item")
			return
		}
	}

	// Best-effort enqueue: a failed insert must never fail the save.
	if _, err := s.riverClient.Insert(ctx, jobs.EnrichArgs{UserID: uid, ItemID: item.ID}, nil); err != nil {
		slog.Error("enqueueing enrichment job", "item_id", item.ID, "err", err)
	}

	// Reload so the response reflects the lead image + title just written.
	item, err = s.store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: item.ID})
	if err != nil {
		slog.Error("reloading image item", "err", err)
		writeError(w, http.StatusInternalServerError, "could not fetch item")
		return
	}
	writeJSON(w, http.StatusCreated, toAPIItem(item))
}

// GetAsset streams a stored image owned by the caller. Cross-tenant and unknown
// ids both resolve to 404 (query is user-scoped). The stored content-type is
// already allowlist-constrained at upload; nosniff blocks browser reinterpretation.
func (s *Server) GetAsset(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	asset, err := s.store.Queries.GetAsset(ctx, db.GetAssetParams{UserID: userID(ctx), ID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "asset not found")
			return
		}
		slog.Error("getting asset", "err", err)
		writeError(w, http.StatusInternalServerError, "could not fetch asset")
		return
	}

	rc, err := s.assetStore.Open(id)
	if err != nil {
		slog.Error("opening asset blob", "asset_id", id, "err", err)
		writeError(w, http.StatusNotFound, "asset not found")
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", asset.ContentType)
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	// sandbox: allows inline rendering (e.g. the browser's built-in PDF
	// viewer) while neutering scripts/forms/embedded actions the asset
	// itself might carry, since it's served from the app origin.
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, rc); err != nil {
		slog.Error("streaming asset", "asset_id", id, "err", err)
	}
}

// filenameStem returns the base filename without its extension, for use as the
// item title. Empty input yields "image".
func filenameStem(name string) string {
	base := path.Base(name)
	base = strings.TrimSuffix(base, path.Ext(base))
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == "/" {
		return "image"
	}
	return base
}
