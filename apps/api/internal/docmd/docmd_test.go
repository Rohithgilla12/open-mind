package docmd

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// newConverter returns a Converter closed at test end. Compilation is lazy, so
// tests that never convert never pay for it.
func newConverter(t *testing.T) *Converter {
	t.Helper()
	c := New()
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

func TestConvertFormats(t *testing.T) {
	tests := []struct {
		name      string
		format    Format
		data      func(*testing.T) []byte
		wantText  []string
		wantTitle string
	}{
		{
			name:      "docx",
			format:    FormatDocx,
			data:      docxFixture,
			wantText:  []string{fixtureDocxHeading, fixtureDocxProse},
			wantTitle: "",
		},
		{
			name:      "odt",
			format:    FormatODT,
			data:      odtFixture,
			wantText:  []string{fixtureODTHeading, fixtureODTProse},
			wantTitle: fixtureODTHeading,
		},
		{
			name:      "epub",
			format:    FormatEPUB,
			data:      epubFixture,
			wantText:  []string{fixtureEPUBTitle, fixtureEPUBProse},
			wantTitle: fixtureEPUBTitle,
		},
		{
			name:      "rtf",
			format:    FormatRTF,
			data:      func(*testing.T) []byte { return []byte(fixtureRTF) },
			wantText:  []string{fixtureRTFProse},
			wantTitle: "",
		},
	}

	c := newConverter(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := c.Convert(context.Background(), tt.data(t), tt.format)
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}
			for _, want := range tt.wantText {
				if !strings.Contains(res.Text, want) {
					t.Errorf("Text = %q, want it to contain %q", res.Text, want)
				}
			}
			if tt.wantTitle != "" && res.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", res.Title, tt.wantTitle)
			}
			// Flattened text must never carry Markdown block syntax: body
			// reaches Kindle EPUBs HTML-escaped and highlight offsets index it.
			for line := range strings.SplitSeq(res.Text, "\n") {
				if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") {
					t.Errorf("Text has Markdown syntax in line %q", line)
				}
			}
		})
	}
}

// Conversion must be a pure function of the bytes: enrichment re-runs and the
// idempotency guarantee depend on it.
func TestConvertIsDeterministic(t *testing.T) {
	c := newConverter(t)
	data := docxFixture(t)
	first, err := c.Convert(context.Background(), data, FormatDocx)
	if err != nil {
		t.Fatalf("first Convert: %v", err)
	}
	second, err := c.Convert(context.Background(), data, FormatDocx)
	if err != nil {
		t.Fatalf("second Convert: %v", err)
	}
	if first != second {
		t.Errorf("Convert not deterministic:\nfirst  = %#v\nsecond = %#v", first, second)
	}
}

func TestConvertRejectsBadInput(t *testing.T) {
	c := newConverter(t)
	tests := []struct {
		name   string
		data   []byte
		format Format
	}{
		{name: "corrupt docx", data: []byte("PK\x03\x04 not really a docx"), format: FormatDocx},
		{name: "corrupt rtf", data: []byte("this is not rtf at all"), format: FormatRTF},
		{name: "unsupported format", data: []byte(fixtureRTF), format: Format("xlsx")},
		{name: "empty input", data: nil, format: FormatDocx},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := c.Convert(context.Background(), tt.data, tt.format); err == nil {
				t.Fatal("want error, got nil")
			}
		})
	}
}

func TestConvertHonoursContext(t *testing.T) {
	c := newConverter(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Convert(ctx, docxFixture(t), FormatDocx); err == nil {
		t.Fatal("want error for cancelled context, got nil")
	}
}

// A converter must survive an error and keep working: one bad document must
// not poison the shared compiled module.
func TestConvertRecoversAfterFailure(t *testing.T) {
	c := newConverter(t)
	if _, err := c.Convert(context.Background(), []byte("PK\x03\x04 junk"), FormatDocx); err == nil {
		t.Fatal("want error for corrupt input, got nil")
	}
	res, err := c.Convert(context.Background(), docxFixture(t), FormatDocx)
	if err != nil {
		t.Fatalf("Convert after failure: %v", err)
	}
	if !strings.Contains(res.Text, fixtureDocxProse) {
		t.Errorf("Text = %q, want it to contain %q", res.Text, fixtureDocxProse)
	}
}

// Instances are throwaway and anonymous, so conversions must be safe to run
// concurrently against one Converter — the enrichment workers do exactly that.
func TestConvertConcurrent(t *testing.T) {
	c := newConverter(t)
	data := docxFixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			if _, err := c.Convert(context.Background(), data, FormatDocx); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Convert: %v", err)
	}
}

// Close must be safe on a Converter that never compiled anything.
func TestCloseWithoutUse(t *testing.T) {
	if err := New().Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestFormatValid(t *testing.T) {
	for _, f := range []Format{FormatDocx, FormatODT, FormatRTF, FormatEPUB} {
		if !f.Valid() {
			t.Errorf("%q.Valid() = false, want true", f)
		}
	}
	for _, f := range []Format{"", "xlsx", "pdf", "csv", "pptx"} {
		if f.Valid() {
			t.Errorf("%q.Valid() = true, want false", f)
		}
	}
}

// Compilation is lazy: constructing a Converter must be effectively free, so
// worker processes that never see a document do not pay the ~3s module compile.
func TestNewIsLazy(t *testing.T) {
	start := time.Now()
	c := New()
	elapsed := time.Since(start)
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	if elapsed > 100*time.Millisecond {
		t.Errorf("New took %v, want it to do no work", elapsed)
	}
	if c.compiled != nil {
		t.Error("New compiled the module eagerly")
	}
}
