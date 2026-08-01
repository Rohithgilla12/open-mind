package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/rohithgilla12/openmind/api/internal/importer"
)

// raindropBase is the Raindrop.io API origin. A var (not a const) so tests can
// point the handler at a fake server.
var raindropBase = "https://api.raindrop.io"

// raindropClient fetches the export with a generous timeout: Raindrop
// assembles the whole library into one CSV response, which can take a while
// for large accounts.
var raindropClient = &http.Client{Timeout: 60 * time.Second}

// ImportRaindrop imports bookmarks straight from Raindrop.io. The supplied API
// test token buys exactly one request — the account's full CSV export — which
// then flows through the same parse → dedupe → create-and-enrich path as an
// uploaded file. The token is used for that single call and is never stored or
// logged. Re-running is idempotent: already-saved URLs are skipped.
func (s *Server) ImportRaindrop(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req ImportRaindropJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	ctx := r.Context()

	// Collection 0 = every raindrop in the account (Trash excluded). The CSV
	// carries url, title, tags, and folder — everything the importer preserves.
	exportURL := raindropBase + "/rest/v1/raindrops/0/export.csv"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, exportURL, nil)
	if err != nil {
		slog.Error("building raindrop export request", "err", err)
		writeError(w, http.StatusInternalServerError, "could not import")
		return
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := raindropClient.Do(httpReq)
	if err != nil {
		slog.Warn("fetching raindrop export", "err", err)
		writeError(w, http.StatusBadGateway, "could not reach Raindrop.io — try again in a moment")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		writeError(w, http.StatusBadRequest, "Raindrop.io rejected the token — copy the test token from your app under Settings → Integrations")
		return
	case resp.StatusCode != http.StatusOK:
		slog.Warn("raindrop export returned non-200", "status", resp.StatusCode)
		writeError(w, http.StatusBadGateway, "Raindrop.io returned an error — try again in a moment")
		return
	}

	// Same cap as an uploaded file; one byte over means truncation, and a
	// silently truncated CSV row could import a mangled URL.
	data, err := io.ReadAll(io.LimitReader(resp.Body, importMaxBytes+1))
	if err != nil {
		slog.Warn("reading raindrop export", "err", err)
		writeError(w, http.StatusBadGateway, "could not read the Raindrop.io export — try again in a moment")
		return
	}
	if len(data) > importMaxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "Raindrop export exceeds the size limit — export a CSV per collection from Raindrop and upload the files instead")
		return
	}

	links, err := importer.Parse("raindrop-export.csv", data)
	if err != nil {
		if errors.Is(err, importer.ErrEmpty) {
			// An empty Raindrop account is a valid (if quiet) import, not an error.
			writeJSON(w, http.StatusOK, ImportResult{})
			return
		}
		writeError(w, http.StatusBadGateway, "could not parse the Raindrop.io export")
		return
	}

	result, err := s.importLinks(ctx, userID(ctx), links)
	if err != nil {
		slog.Error("importing raindrop links", "err", err)
		writeError(w, http.StatusInternalServerError, "could not import")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
