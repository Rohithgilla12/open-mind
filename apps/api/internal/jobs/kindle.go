package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
	"github.com/rohithgilla12/openmind/api/internal/epub"
	"github.com/rohithgilla12/openmind/api/internal/mailer"
	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// maxLeadImageBytes bounds how much of a lead image response body
// fetchLeadImage will read: 5 MiB, plus one extra byte so a response
// exactly at the cap can still be distinguished from one that overflows it.
const maxLeadImageBytes = 5<<20 + 1

// allowedLeadImageTypes is the set of sniffed content types fetchLeadImage
// accepts; anything else (including text/html error pages served with a
// 200) is treated as "no image".
var allowedLeadImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// fetchLeadImage best-effort downloads the image at url and returns its
// bytes and a sniffed content type suitable for epub.Chapter.ImageType. It
// never returns an error: any failure (network error, non-2xx status,
// over-cap body, or a content type outside the allowlist) yields (nil, "")
// and is logged (network failures at debug; a size-cap or content-type
// rejection at warn, since those point at extractor problems rather than
// transient network noise), since a missing hero image must never fail or
// delay a Kindle send. client defaults to enrich.SafeHTTPClient(10s) when
// nil.
func fetchLeadImage(ctx context.Context, client *http.Client, url string) ([]byte, string) {
	if client == nil {
		client = enrich.SafeHTTPClient(10 * time.Second)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Debug("send_kindle: building lead image request", "url", url, "error", err)
		return nil, ""
	}
	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("send_kindle: fetching lead image", "url", url, "error", err)
		return nil, ""
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Debug("send_kindle: lead image non-2xx response", "url", url, "status", resp.StatusCode)
		return nil, ""
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxLeadImageBytes))
	if err != nil {
		slog.Debug("send_kindle: reading lead image body", "url", url, "error", err)
		return nil, ""
	}
	if len(data) > maxLeadImageBytes-1 {
		slog.Warn("send_kindle: lead image exceeds size cap; skipping", "url", url)
		return nil, ""
	}
	if len(data) == 0 {
		return nil, ""
	}

	contentType := http.DetectContentType(data)
	// DetectContentType can append parameters (e.g. "image/png; ..." never
	// happens in practice for sniffed types, but normalize defensively).
	contentType = strings.SplitN(contentType, ";", 2)[0]
	if !allowedLeadImageTypes[contentType] {
		slog.Warn("send_kindle: lead image content type not allowed", "url", url, "content_type", contentType)
		return nil, ""
	}
	return data, contentType
}

// kindleDigestCap is the maximum number of items folded into a single Lens
// digest EPUB, so a broad rule never produces an unreasonably large book.
const kindleDigestCap = 25

// kindleSettingKey is the user_settings key holding a reader's personal
// Kindle e-mail address. It mirrors internal/api's unexported constant of
// the same name (kept separate to avoid an import cycle).
const kindleSettingKey = "kindle_email"

// KindleDeps carries what the send_kindle worker needs to actually deliver a
// message: a configured Mailer and an optional fallback destination address.
// Configured mirrors whether SMTP_HOST and SMTP_FROM (the SMTP transport)
// were both set at startup — KINDLE_EMAIL is no longer required for
// Configured to be true, since it is only ever the server-wide fallback
// recipient (To), not the transport. Handlers gate on Configured before ever
// enqueueing a job, so the worker only sees Configured go false if the
// deploy's config changed between enqueue and run — in that case Work
// returns an error so River retries rather than silently dropping the send.
type KindleDeps struct {
	Mailer     mailer.Mailer
	To         string
	Configured bool
}

// SendKindleArgs is the River job payload for e-mailing an item or a Lens
// digest to Kindle. Exactly one of ItemID or LensID must be set; the worker
// fetches fresh state inside the job rather than trusting a stale snapshot.
// ItemIDs is optional and only meaningful alongside LensID: when set (as the
// scan_digests worker does), the digest is built from exactly those items
// instead of re-running the Lens's rule, so a digest only ever contains what
// was new at scan time even if the rule's result set has since changed.
type SendKindleArgs struct {
	UserID  uuid.UUID   `json:"user_id"`
	ItemID  *uuid.UUID  `json:"item_id,omitempty"`
	LensID  *uuid.UUID  `json:"lens_id,omitempty"`
	ItemIDs []uuid.UUID `json:"item_ids,omitempty"`
}

// Kind identifies the job type in River.
func (SendKindleArgs) Kind() string { return "send_kindle" }

// SendKindleWorker builds an EPUB (a single item, or a multi-chapter Lens
// digest) and e-mails it via Deps.Mailer. A retry after a transient send
// failure may re-deliver the same e-mail — River does not dedupe by content,
// so this can produce a duplicate on the reader's Kindle. That is considered
// acceptable: it is rare (only on error) and harmless (worst case, the same
// article twice).
type SendKindleWorker struct {
	river.WorkerDefaults[SendKindleArgs]
	Store    *store.Store
	Provider ai.Provider
	Deps     KindleDeps
}

// Work sends the item or Lens digest identified by job.Args. It returns an
// error (triggering a River retry, up to MaxAttempts) for anything that
// might succeed on a later attempt: an unconfigured mailer, a transient DB
// error, or an SMTP failure. A body-less item/lens is not retryable — it is
// logged and treated as done, since re-running would find the same emptiness.
func (w *SendKindleWorker) Work(ctx context.Context, job *river.Job[SendKindleArgs]) error {
	args := job.Args
	if (args.ItemID == nil) == (args.LensID == nil) {
		return fmt.Errorf("send_kindle: exactly one of item_id or lens_id must be set")
	}
	if !w.Deps.Configured {
		return fmt.Errorf("send_kindle: kindle is not configured")
	}

	if args.ItemID != nil {
		return w.sendItem(ctx, args.UserID, *args.ItemID)
	}
	if len(args.ItemIDs) > 0 {
		return w.sendItemIDsDigest(ctx, args.UserID, *args.LensID, args.ItemIDs)
	}
	return w.sendLensDigest(ctx, args.UserID, *args.LensID)
}

// recipient resolves the destination Kindle address for uid: the user's own
// kindle_email setting takes priority, falling back to the server-wide
// Deps.To. If neither is set, it returns an error so River retries — this
// mirrors send_kindle's other not-yet-configured cases rather than silently
// dropping the send.
func (w *SendKindleWorker) recipient(ctx context.Context, uid uuid.UUID) (string, error) {
	to, err := resolveKindleRecipient(ctx, w.Store, uid, w.Deps.To)
	if err != nil {
		return "", fmt.Errorf("send_kindle: %w", err)
	}
	return to, nil
}

// resolveKindleRecipient resolves the destination Kindle address for uid: the
// user's own kindle_email setting takes priority, falling back to fallback
// (a server-wide address, typically Deps.To). If neither is set, it returns
// an error. Shared by SendKindleWorker (where an error triggers a River
// retry) and ScanDigestsWorker (where an error means the lens is skipped
// without stamping) so recipient resolution can never drift between the two.
func resolveKindleRecipient(ctx context.Context, s *store.Store, uid uuid.UUID, fallback string) (string, error) {
	if to, err := s.Queries.GetUserSetting(ctx, db.GetUserSettingParams{UserID: uid, Key: kindleSettingKey}); err == nil && to != "" {
		return to, nil
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("fetching kindle_email setting: %w", err)
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("no kindle address configured for user %s", uid)
}

func (w *SendKindleWorker) sendItem(ctx context.Context, uid, itemID uuid.UUID) error {
	item, err := w.Store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: itemID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("send_kindle: item no longer exists; skipping", "item_id", itemID)
			return nil
		}
		return fmt.Errorf("send_kindle: fetching item: %w", err)
	}
	if item.Body == "" {
		slog.Warn("send_kindle: item has no body; nothing to send", "item_id", itemID)
		return nil
	}

	title := item.Title
	if title == "" {
		title = item.Url
	}
	if title == "" {
		title = "Openmind item"
	}
	doc := epub.Document{
		Title:    title,
		Author:   "Openmind",
		Chapters: []epub.Chapter{w.buildChapter(ctx, title, item)},
		Date:     time.Now().UTC().Format("2 January 2006"),
	}
	return w.send(ctx, uid, doc, fmt.Sprintf("openmind-%s.epub", shortID(item.ID)))
}

// buildChapter turns item into an epub.Chapter with title/body, best-effort
// embedding item's lead image as a chapter hero. LeadImageUrl can be a
// relative "/assets/<id>" path for a user-uploaded image — there is no base
// URL available in the worker to resolve it against, so relative URLs are
// skipped entirely rather than guessed at; only absolute (http/https) lead
// image URLs are fetched.
func (w *SendKindleWorker) buildChapter(ctx context.Context, title string, item db.Item) epub.Chapter {
	ch := epub.Chapter{Title: title, Body: item.Body}
	if item.LeadImageUrl == "" || strings.HasPrefix(item.LeadImageUrl, "/") {
		return ch
	}
	if !strings.HasPrefix(item.LeadImageUrl, "http://") && !strings.HasPrefix(item.LeadImageUrl, "https://") {
		return ch
	}
	data, contentType := fetchLeadImage(ctx, nil, item.LeadImageUrl)
	if data != nil {
		ch.Image = data
		ch.ImageType = contentType
	}
	return ch
}

func (w *SendKindleWorker) sendLensDigest(ctx context.Context, uid, lensID uuid.UUID) error {
	lens, err := w.Store.Queries.GetLens(ctx, db.GetLensParams{UserID: uid, ID: lensID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("send_kindle: lens no longer exists; skipping", "lens_id", lensID)
			return nil
		}
		return fmt.Errorf("send_kindle: fetching lens: %w", err)
	}

	items, err := lensItems(ctx, w.Store, w.Provider, uid, lens)
	if err != nil {
		return fmt.Errorf("send_kindle: %w", err)
	}

	chapters := make([]epub.Chapter, 0, kindleDigestCap)
	for _, item := range items {
		if item.Body == "" {
			continue
		}
		title := item.Title
		if title == "" {
			title = item.Url
		}
		chapters = append(chapters, w.buildChapter(ctx, title, item))
		if len(chapters) >= kindleDigestCap {
			break
		}
	}
	if len(chapters) == 0 {
		slog.Warn("send_kindle: lens has no items with bodies; nothing to send", "lens_id", lensID)
		return nil
	}

	title := fmt.Sprintf("Openmind digest — %s — %s", lens.Name, time.Now().Format("2006-01-02"))
	doc := epub.Document{Title: title, Author: "Openmind", Chapters: chapters, Date: time.Now().UTC().Format("2 January 2006")}
	return w.send(ctx, uid, doc, fmt.Sprintf("openmind-%s.epub", shortID(lens.ID)))
}

// sendItemIDsDigest builds a Lens digest from exactly the given item IDs
// (the scan_digests job's "new items since last digest" set) rather than
// re-running the Lens's rule. Missing items (deleted since the scan) are
// skipped; a body-less item is skipped the same way a rule-derived one is.
func (w *SendKindleWorker) sendItemIDsDigest(ctx context.Context, uid, lensID uuid.UUID, itemIDs []uuid.UUID) error {
	lens, err := w.Store.Queries.GetLens(ctx, db.GetLensParams{UserID: uid, ID: lensID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("send_kindle: lens no longer exists; skipping", "lens_id", lensID)
			return nil
		}
		return fmt.Errorf("send_kindle: fetching lens: %w", err)
	}

	chapters := make([]epub.Chapter, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		item, err := w.Store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: itemID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				slog.Warn("send_kindle: digest item no longer exists; skipping", "item_id", itemID)
				continue
			}
			return fmt.Errorf("send_kindle: fetching digest item: %w", err)
		}
		if item.Body == "" {
			continue
		}
		title := item.Title
		if title == "" {
			title = item.Url
		}
		chapters = append(chapters, w.buildChapter(ctx, title, item))
	}
	if len(chapters) == 0 {
		slog.Warn("send_kindle: digest item set has no items with bodies; nothing to send", "lens_id", lensID)
		return nil
	}

	title := fmt.Sprintf("Openmind digest — %s — %s", lens.Name, time.Now().Format("2006-01-02"))
	doc := epub.Document{Title: title, Author: "Openmind", Chapters: chapters, Date: time.Now().UTC().Format("2 January 2006")}
	return w.send(ctx, uid, doc, fmt.Sprintf("openmind-%s.epub", shortID(lens.ID)))
}

// kindleLensRule decodes the subset of a stored LensRule this worker needs.
// It mirrors the api package's LensRule JSON shape without importing
// internal/api (which would create an import cycle).
type kindleLensRule struct {
	Q       *string  `json:"q"`
	Color   *string  `json:"color"`
	Types   []string `json:"types"`
	Domains []string `json:"domains"`
	Scope   string   `json:"scope"`
}

// lensItems runs lens's stored rule and returns the matching items. It is
// shared by the full-rule digest path (sendLensDigest) and the digest-scan
// job (ScanDigestsWorker), so both agree on exactly what a Lens matches.
// Empty scope defaults to library inside RunLensRule.
func lensItems(ctx context.Context, s *store.Store, p ai.Provider, uid uuid.UUID, lens db.Lense) ([]db.Item, error) {
	var rule kindleLensRule
	if len(lens.Rule) > 0 {
		if err := json.Unmarshal(lens.Rule, &rule); err != nil {
			return nil, fmt.Errorf("decoding lens rule: %w", err)
		}
	}
	var text, color string
	if rule.Q != nil {
		text = *rule.Q
	}
	if rule.Color != nil {
		color = *rule.Color
	}

	q := search.Query{
		Text:    text,
		Color:   color,
		Types:   rule.Types,
		Domains: search.NormalizeDomains(rule.Domains),
		Scope:   search.Scope(rule.Scope),
	}
	results, err := search.RunLensRule(ctx, s, p, uid, q)
	if err != nil && !errors.Is(err, search.ErrBadColor) {
		return nil, fmt.Errorf("running lens rule: %w", err)
	}
	items := make([]db.Item, 0, len(results))
	for _, res := range results {
		items = append(items, res.Item)
	}
	return items, nil
}

// send builds doc into an EPUB in memory and e-mails it as an attachment to
// uid's resolved Kindle address.
func (w *SendKindleWorker) send(ctx context.Context, uid uuid.UUID, doc epub.Document, filename string) error {
	to, err := w.recipient(ctx, uid)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := epub.Build(&buf, doc); err != nil {
		return fmt.Errorf("send_kindle: building epub: %w", err)
	}
	msg := mailer.Message{
		To:       to,
		Subject:  doc.Title,
		BodyText: fmt.Sprintf("%s is attached as an EPUB.", doc.Title),
		Attachment: &mailer.Attachment{
			Filename:    filename,
			ContentType: "application/epub+zip",
			Data:        buf.Bytes(),
		},
	}
	if err := w.Deps.Mailer.Send(ctx, msg); err != nil {
		return fmt.Errorf("send_kindle: sending mail: %w", err)
	}
	return nil
}

// shortID returns the first 8 hex characters of id, used for a short,
// filesystem-friendly attachment filename.
func shortID(id uuid.UUID) string {
	s := id.String()
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
