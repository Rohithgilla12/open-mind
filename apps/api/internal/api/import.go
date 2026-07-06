package api

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/rohithgilla12/openmind/api/internal/importer"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

const (
	// importMaxBytes caps the upload; bookmark/CSV exports are small even for
	// large libraries (a few MB), so this is generous while bounding memory.
	importMaxBytes = 16 << 20
	// importMaxLinks bounds how many items one import creates, so a runaway file
	// can't enqueue an unbounded number of enrichment jobs in a single request.
	importMaxLinks = 10000
)

// ImportItems bulk-imports saved items from an uploaded export file. It parses
// the file into candidate links, then creates a pending item per new, valid,
// not-already-saved URL and queues enrichment for it — reusing the normal
// capture path, so imported items flow through the same async pipeline. URLs
// already in the library (or repeated within the file) are skipped, making a
// re-import idempotent. It returns a per-file summary.
func (s *Server) ImportItems(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, importMaxBytes)

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

	data, err := io.ReadAll(file)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds size limit")
			return
		}
		writeError(w, http.StatusBadRequest, "could not read file")
		return
	}

	links, err := importer.Parse(header.Filename, data)
	if err != nil {
		if errors.Is(err, importer.ErrEmpty) {
			writeError(w, http.StatusBadRequest, "no importable links found in file")
			return
		}
		writeError(w, http.StatusBadRequest, "could not parse file")
		return
	}

	ctx := r.Context()
	uid := userID(ctx)

	// Existing URLs → skip set, so re-importing the same file is a no-op.
	existingURLs, err := s.store.Queries.ListItemURLs(ctx, uid)
	if err != nil {
		slog.Error("listing item urls for import", "err", err)
		writeError(w, http.StatusInternalServerError, "could not import")
		return
	}
	seen := make(map[string]bool, len(existingURLs)+len(links))
	for _, u := range existingURLs {
		seen[u] = true
	}

	result := ImportResult{Total: len(links)}
	for _, link := range links {
		if result.Imported >= importMaxLinks {
			// Remaining links are counted as skipped so the summary still balances.
			result.Skipped += result.Total - (result.Imported + result.Skipped + result.Failed)
			slog.Warn("import truncated at cap", "cap", importMaxLinks, "total", result.Total)
			break
		}
		if !validURL(link.URL) {
			result.Failed++
			continue
		}
		if seen[link.URL] {
			result.Skipped++
			continue
		}
		seen[link.URL] = true

		item, err := s.store.Queries.CreateItem(ctx, db.CreateItemParams{UserID: uid, Url: link.URL})
		if err != nil {
			slog.Error("creating imported item", "err", err)
			result.Failed++
			continue
		}
		// Preserve any tags the source file carried as user tags. Best-effort: a
		// failure here doesn't undo the save (enrichment never touches user_tags,
		// so order vs. enqueue is irrelevant).
		if tags := canonicalTags(link.Tags); len(tags) > 0 {
			if _, err := s.store.Queries.SetUserTags(ctx, db.SetUserTagsParams{UserID: uid, ID: item.ID, UserTags: tags}); err != nil {
				slog.Error("setting user tags for imported item", "item_id", item.ID, "err", err)
			}
		}
		if _, err := s.riverClient.Insert(ctx, jobs.EnrichArgs{UserID: uid, ItemID: item.ID}, nil); err != nil {
			// The item is saved; a failed enqueue can be re-run later. Don't count
			// it as failed — capture succeeded.
			slog.Error("enqueueing enrichment for imported item", "item_id", item.ID, "err", err)
		}
		result.Imported++
	}

	writeJSON(w, http.StatusOK, result)
}
