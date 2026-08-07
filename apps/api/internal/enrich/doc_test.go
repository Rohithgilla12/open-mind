package enrich_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/assets"
	"github.com/rohithgilla12/openmind/api/internal/docmd"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
	"github.com/rohithgilla12/openmind/api/internal/pdftext"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// fakeDoc is a DocConverter test double so enrich tests never compile the
// anydoc wasm module.
type fakeDoc struct {
	res docmd.Result
	err error
	// gotFormat records what the pipeline asked for, so tests can assert the
	// content-type → format mapping without reaching into unexported code.
	gotFormat *docmd.Format
}

func (f fakeDoc) Convert(ctx context.Context, data []byte, format docmd.Format) (docmd.Result, error) {
	if f.gotFormat != nil {
		*f.gotFormat = format
	}
	return f.res, f.err
}

// uploadedDoc creates a user, an item, and a document asset with blob bytes on
// disk, wired the way CreateAsset leaves them: the asset path in the item's
// URL and the filename stem as the title.
func uploadedDoc(t *testing.T, s *store.Store, assetStore *assets.FSStore, contentType, filename, title string) (uuid.UUID, db.Item) {
	t.Helper()
	ctx := context.Background()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	asset, err := s.Queries.CreateAsset(ctx, db.CreateAssetParams{
		UserID: userID, ItemID: pgtype.UUID{Bytes: item.ID, Valid: true},
		ContentType: contentType, OriginalFilename: filename,
	})
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}
	const blob = "fixture-document-bytes"
	if _, err := assetStore.Put(asset.ID, strings.NewReader(blob), int64(len(blob))); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	if err := s.Queries.SetItemURL(ctx, db.SetItemURLParams{
		UserID: userID, ID: item.ID, Url: "/assets/" + asset.ID.String(),
	}); err != nil {
		t.Fatalf("set url: %v", err)
	}
	if err := s.Queries.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: item.ID, Title: title, Body: "", LeadImageUrl: "", CardType: "article",
	}); err != nil {
		t.Fatalf("set metadata: %v", err)
	}
	item.Title = title
	return userID, item
}

const (
	docxType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	odtType  = "application/vnd.oasis.opendocument.text"
	rtfType  = "application/rtf"
	epubType = "application/epub+zip"
)

func TestRunUploadedDoc(t *testing.T) {
	tests := []struct {
		name         string
		contentType  string
		filename     string
		uploadTitle  string
		res          docmd.Result
		wantFormat   docmd.Format
		wantCardType string
		wantTitle    string
	}{
		{
			name:         "docx becomes an article with the heading as title",
			contentType:  docxType,
			filename:     "report.docx",
			uploadTitle:  "report",
			res:          docmd.Result{Title: "Quarterly Report", Markdown: "# Quarterly Report\n\nProse.", Text: "Quarterly Report\n\nProse."},
			wantFormat:   docmd.FormatDocx,
			wantCardType: "article",
			wantTitle:    "Quarterly Report",
		},
		{
			name:         "epub becomes a book",
			contentType:  epubType,
			filename:     "novel.epub",
			uploadTitle:  "novel",
			res:          docmd.Result{Title: "A Novel", Markdown: "# A Novel\n\nChapter one.", Text: "A Novel\n\nChapter one."},
			wantFormat:   docmd.FormatEPUB,
			wantCardType: "book",
			wantTitle:    "A Novel",
		},
		{
			name:         "odt maps to the odt parser",
			contentType:  odtType,
			filename:     "notes.odt",
			uploadTitle:  "notes",
			res:          docmd.Result{Title: "", Markdown: "Plain prose.", Text: "Plain prose."},
			wantFormat:   docmd.FormatODT,
			wantCardType: "article",
			wantTitle:    "notes", // no heading, so the filename stem stands
		},
		{
			name:         "rtf falls back to the upload filename",
			contentType:  rtfType,
			filename:     "memo.rtf",
			uploadTitle:  "memo",
			res:          docmd.Result{Title: "", Markdown: "Some text.", Text: "Some text."},
			wantFormat:   docmd.FormatRTF,
			wantCardType: "article",
			wantTitle:    "memo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			assetStore := newTestAssetStore(t)
			ctx := context.Background()
			userID, item := uploadedDoc(t, s, assetStore, tt.contentType, tt.filename, tt.uploadTitle)

			var gotFormat docmd.Format
			p := &enrich.Pipeline{
				Store: s, AI: ai.NewFake(), Assets: assetStore,
				Doc: fakeDoc{res: tt.res, gotFormat: &gotFormat},
			}
			if err := p.Run(ctx, userID, item.ID); err != nil {
				t.Fatalf("first run: %v", err)
			}
			first, err := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
			if err != nil {
				t.Fatalf("get item: %v", err)
			}

			if gotFormat != tt.wantFormat {
				t.Errorf("format = %q, want %q", gotFormat, tt.wantFormat)
			}
			if first.CardType != tt.wantCardType {
				t.Errorf("card_type = %q, want %q", first.CardType, tt.wantCardType)
			}
			if first.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", first.Title, tt.wantTitle)
			}
			if first.Body != tt.res.Text {
				t.Errorf("body = %q, want %q", first.Body, tt.res.Text)
			}
			if !first.BodyMarkdown.Valid || first.BodyMarkdown.String != tt.res.Markdown {
				t.Errorf("body_markdown = %+v, want %q", first.BodyMarkdown, tt.res.Markdown)
			}
			if first.Status != "enriched" {
				t.Errorf("status = %q, want enriched", first.Status)
			}
			// Documents have no page count; the PDF path's column stays null.
			if first.PageCount.Valid {
				t.Errorf("page_count = %+v, want null", first.PageCount)
			}

			// Re-run: idempotent, identical rows.
			if err := p.Run(ctx, userID, item.ID); err != nil {
				t.Fatalf("second run: %v", err)
			}
			second, err := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
			if err != nil {
				t.Fatalf("get item: %v", err)
			}
			if second.Body != first.Body || second.Title != first.Title ||
				second.CardType != first.CardType || second.Status != first.Status ||
				second.BodyMarkdown != first.BodyMarkdown {
				t.Errorf("second run changed state:\nfirst  %+v\nsecond %+v", first, second)
			}
		})
	}
}

func TestRunUploadedDocFailures(t *testing.T) {
	tests := []struct {
		name string
		doc  enrich.DocConverter
	}{
		{name: "conversion error", doc: fakeDoc{err: errors.New("boom")}},
		{name: "converter not configured", doc: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			assetStore := newTestAssetStore(t)
			ctx := context.Background()
			userID, item := uploadedDoc(t, s, assetStore, docxType, "broken.docx", "broken")

			p := &enrich.Pipeline{Store: s, AI: ai.NewFake(), Assets: assetStore, Doc: tt.doc}
			if err := p.Run(ctx, userID, item.ID); err == nil {
				t.Fatal("want error, got nil")
			}
			got, err := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
			if err != nil {
				t.Fatalf("get item: %v", err)
			}
			// The item and its asset survive a failure; only the status flips.
			if got.Status != "failed" {
				t.Errorf("status = %q, want failed", got.Status)
			}
			if got.Title != "broken" {
				t.Errorf("title = %q, want the upload title to survive", got.Title)
			}
		})
	}
}

// A PDF asset must keep routing to the PDF path now that the /assets/ branch
// switches on content type.
func TestUploadedPDFStillRoutesToPDF(t *testing.T) {
	s := newTestStore(t)
	assetStore := newTestAssetStore(t)
	ctx := context.Background()
	userID, item := uploadedDoc(t, s, assetStore, "application/pdf", "paper.pdf", "paper")

	p := &enrich.Pipeline{
		Store: s, AI: ai.NewFake(), Assets: assetStore,
		PDF: fakePDF{res: pdftext.Result{Title: "PDF Title", Text: "pdf body", Pages: 7}},
		Doc: fakeDoc{err: errors.New("document path must not run for a PDF")},
	}
	if err := p.Run(ctx, userID, item.ID); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if got.Body != "pdf body" {
		t.Errorf("body = %q, want the PDF path's output", got.Body)
	}
	if !got.PageCount.Valid || got.PageCount.Int32 != 7 {
		t.Errorf("page_count = %+v, want valid 7 — the PDF path sets it", got.PageCount)
	}
	if got.BodyMarkdown.Valid {
		t.Errorf("body_markdown = %+v, want null for a PDF", got.BodyMarkdown)
	}
}

func TestDocFormatFor(t *testing.T) {
	tests := []struct {
		contentType string
		want        docmd.Format
		wantOK      bool
	}{
		{contentType: docxType, want: docmd.FormatDocx, wantOK: true},
		{contentType: odtType, want: docmd.FormatODT, wantOK: true},
		{contentType: rtfType, want: docmd.FormatRTF, wantOK: true},
		{contentType: epubType, want: docmd.FormatEPUB, wantOK: true},
		{contentType: "application/pdf", wantOK: false},
		{contentType: "image/png", wantOK: false},
		{contentType: "application/zip", wantOK: false},
		{contentType: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			got, ok := enrich.DocFormatFor(tt.contentType)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("format = %q, want %q", got, tt.want)
			}
		})
	}
}
