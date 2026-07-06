package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/epub"
	"github.com/rohithgilla12/openmind/api/internal/mailer"
	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// kindleDigestCap is the maximum number of items folded into a single Lens
// digest EPUB, so a broad rule never produces an unreasonably large book.
const kindleDigestCap = 25

// KindleDeps carries what the send_kindle worker needs to actually deliver a
// message: a configured Mailer and the destination address. Configured
// mirrors whether SMTP_HOST, SMTP_FROM and KINDLE_EMAIL were all set at
// startup; handlers gate on it before ever enqueueing a job, so the worker
// only sees Configured go false if the deploy's config changed between
// enqueue and run — in that case Work returns an error so River retries
// rather than silently dropping the send.
type KindleDeps struct {
	Mailer     mailer.Mailer
	To         string
	Configured bool
}

// SendKindleArgs is the River job payload for e-mailing an item or a Lens
// digest to Kindle. Exactly one of ItemID or LensID must be set; the worker
// fetches fresh state inside the job rather than trusting a stale snapshot.
type SendKindleArgs struct {
	UserID uuid.UUID  `json:"user_id"`
	ItemID *uuid.UUID `json:"item_id,omitempty"`
	LensID *uuid.UUID `json:"lens_id,omitempty"`
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
	return w.sendLensDigest(ctx, args.UserID, *args.LensID)
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
		Chapters: []epub.Chapter{{Title: title, Body: item.Body}},
	}
	return w.send(ctx, doc, fmt.Sprintf("openmind-%s.epub", shortID(item.ID)))
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

	var rule kindleLensRule
	if len(lens.Rule) > 0 {
		if err := json.Unmarshal(lens.Rule, &rule); err != nil {
			return fmt.Errorf("send_kindle: decoding lens rule: %w", err)
		}
	}
	var q, color string
	if rule.Q != nil {
		q = *rule.Q
	}
	if rule.Color != nil {
		color = *rule.Color
	}

	results, err := search.RunLensRule(ctx, w.Store, w.Provider, uid, q, color, rule.Types)
	if err != nil && !errors.Is(err, search.ErrBadColor) {
		return fmt.Errorf("send_kindle: running lens rule: %w", err)
	}

	chapters := make([]epub.Chapter, 0, kindleDigestCap)
	for _, res := range results {
		if res.Item.Body == "" {
			continue
		}
		title := res.Item.Title
		if title == "" {
			title = res.Item.Url
		}
		chapters = append(chapters, epub.Chapter{Title: title, Body: res.Item.Body})
		if len(chapters) >= kindleDigestCap {
			break
		}
	}
	if len(chapters) == 0 {
		slog.Warn("send_kindle: lens has no items with bodies; nothing to send", "lens_id", lensID)
		return nil
	}

	title := fmt.Sprintf("Openmind digest — %s — %s", lens.Name, time.Now().Format("2006-01-02"))
	doc := epub.Document{Title: title, Author: "Openmind", Chapters: chapters}
	return w.send(ctx, doc, fmt.Sprintf("openmind-%s.epub", shortID(lens.ID)))
}

// kindleLensRule decodes the subset of a stored LensRule this worker needs
// (q, color; types is decoded separately as it doesn't need pointer
// semantics). It mirrors the api package's LensRule JSON shape without
// importing internal/api (which would create an import cycle).
type kindleLensRule struct {
	Q     *string  `json:"q"`
	Color *string  `json:"color"`
	Types []string `json:"types"`
}

// send builds doc into an EPUB in memory and e-mails it as an attachment.
func (w *SendKindleWorker) send(ctx context.Context, doc epub.Document, filename string) error {
	var buf bytes.Buffer
	if err := epub.Build(&buf, doc); err != nil {
		return fmt.Errorf("send_kindle: building epub: %w", err)
	}
	msg := mailer.Message{
		To:       w.Deps.To,
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
