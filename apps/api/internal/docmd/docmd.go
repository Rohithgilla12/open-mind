// Package docmd converts office-document bytes to Markdown and plain text. It
// is the only package that touches anydoc; callers see plain Go values. anydoc
// runs as WebAssembly under wazero, so the build stays pure Go (no cgo, no
// sidecar, no Rust toolchain — the artefact is committed and embedded).
package docmd

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// anydocWasm is the wasm32-wasip1 build of tools/anydoc-wasi. Rebuild with
// scripts/build-anydoc-wasm.sh when bumping the pinned anydoc version.
//
//go:embed anydoc.wasm
var anydocWasm []byte

// Format names a document format the wrapper understands. The value is passed
// verbatim as the wasm module's argv[1], where anydoc's Format::from_extension
// resolves it.
type Format string

// The formats Openmind accepts. Spreadsheets, presentations and CSV are
// deliberately excluded: they make poor cards and noisy embeddings.
const (
	FormatDocx Format = "docx"
	FormatODT  Format = "odt"
	FormatRTF  Format = "rtf"
	FormatEPUB Format = "epub"
)

// Valid reports whether f is a format this package will convert.
func (f Format) Valid() bool {
	switch f {
	case FormatDocx, FormatODT, FormatRTF, FormatEPUB:
		return true
	}
	return false
}

// convertTimeout is a hard per-document ceiling: a hostile document must never
// pin a worker. Matches pdftext's budget.
const convertTimeout = 30 * time.Second

// maxTextBytes bounds the accumulated output, mirroring pdftext and the enrich
// package's 10 MB response cap.
const maxTextBytes = 10 << 20

// memoryLimitPages caps the wasm instance's linear memory at 512 MiB
// (wasm pages are 64 KiB). A decompression bomb then traps inside the sandbox
// and surfaces as a conversion error, rather than pressuring the host.
const memoryLimitPages = 8192

// ErrEmptyInput is returned when there is nothing to convert.
var ErrEmptyInput = errors.New("docmd: empty input")

// ErrTooLarge is returned when a document's Markdown exceeds maxTextBytes.
var ErrTooLarge = errors.New("docmd: converted output exceeds limit")

// Result is the converted content of one document.
type Result struct {
	Title    string // first H1 of the Markdown, "" when absent
	Markdown string // GitHub-Flavored Markdown as anydoc emitted it
	Text     string // Markdown flattened to plain prose, for item.body
}

// Converter owns the compiled anydoc module. Create one per process and reuse
// it; each conversion runs in a fresh, throwaway instance.
//
// Compilation costs roughly three seconds, so it happens lazily on first use
// rather than at construction: most worker processes never see a document and
// must not pay it at boot.
type Converter struct {
	mu       sync.Mutex
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
}

// New returns a Converter. It does no work: the wasm module is compiled on the
// first Convert call.
func New() *Converter { return &Converter{} }

// ready returns the runtime and compiled module, compiling on first use.
//
// Two deliberate choices, both learned the hard way:
//
// Compilation runs on context.Background(), never the caller's context. It is
// shared process-wide work costing seconds, so a caller that cancels or times
// out must not tear it down for everyone else. Under `go test -race` the
// compile takes ~50s (vs ~3s normally); when it ran on the caller's context it
// blew the per-document deadline and surfaced as an opaque wasm exit status.
//
// A failure is not memoized. Caching it — as a sync.Once does — meant one
// unlucky first call disabled document conversion for the whole process
// lifetime, with every later call reporting the stale error.
func (c *Converter) ready() (wazero.Runtime, wazero.CompiledModule, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.compiled != nil {
		return c.runtime, c.compiled, nil
	}

	ctx := context.Background()
	// WithCloseOnContextDone lets a context deadline interrupt an in-flight
	// conversion; without it a wedged module would run to completion
	// regardless of the per-document ceiling.
	cfg := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(memoryLimitPages)
	rt := wazero.NewRuntimeWithConfig(ctx, cfg)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, nil, fmt.Errorf("instantiating wasi: %w", err)
	}
	compiled, err := rt.CompileModule(ctx, anydocWasm)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, nil, fmt.Errorf("compiling anydoc wasm: %w", err)
	}

	c.runtime, c.compiled = rt, compiled
	return rt, compiled, nil
}

// Close releases the compiled module and runtime. Safe to call on a Converter
// that was never used, and safe to race with Convert. The fields are cleared
// under the lock so a later Convert recompiles rather than reaching for a
// closed runtime.
func (c *Converter) Close(ctx context.Context) error {
	c.mu.Lock()
	rt := c.runtime
	c.runtime, c.compiled = nil, nil
	c.mu.Unlock()

	if rt == nil {
		return nil
	}
	return rt.Close(ctx)
}

// Convert turns document bytes into Markdown and flattened text. It respects
// ctx and additionally enforces its own 30s ceiling. Corrupt or unparseable
// input returns an error; a document with no text returns a Result with empty
// Markdown and Text, and no error.
func (c *Converter) Convert(ctx context.Context, data []byte, format Format) (Result, error) {
	if !format.Valid() {
		return Result{}, fmt.Errorf("docmd: unsupported format %q", format)
	}
	if len(data) == 0 {
		return Result{}, ErrEmptyInput
	}

	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	// Compile before starting the per-document clock: the one-time module
	// compile costs seconds and must not eat a document's budget.
	rt, compiled, err := c.ready()
	if err != nil {
		return Result{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, convertTimeout)
	defer cancel()

	stdout := &limitedWriter{limit: maxTextBytes}
	var stderr bytes.Buffer
	// WithName("") keeps instances anonymous so concurrent conversions cannot
	// collide on the module namespace.
	cfg := wazero.NewModuleConfig().
		WithName("").
		WithArgs("anydoc", string(format)).
		WithStdin(bytes.NewReader(data)).
		WithStdout(stdout).
		WithStderr(&stderr)

	mod, err := rt.InstantiateModule(ctx, compiled, cfg)
	if mod != nil {
		_ = mod.Close(ctx)
	}
	// Checked before err: the guest sees the rejected write and exits non-zero,
	// so the size cause would otherwise be reported as a generic failure.
	if stdout.exceeded {
		return Result{}, fmt.Errorf("%w (%d bytes)", ErrTooLarge, maxTextBytes)
	}
	if err != nil {
		// A deadline or cancellation reaches us as a terminated guest carrying
		// an opaque exit status, so the context is checked first — otherwise a
		// timeout masquerades as a conversion failure.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, ctxErr
		}
		// A non-zero exit is how the wrapper reports a failed conversion; its
		// stderr line carries anydoc's message.
		var exitErr *sys.ExitError
		if errors.As(err, &exitErr) {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = fmt.Sprintf("exit status %d", exitErr.ExitCode())
			}
			return Result{}, fmt.Errorf("docmd: converting %s: %s", format, msg)
		}
		return Result{}, fmt.Errorf("docmd: running anydoc: %w", err)
	}

	// No truncation: limitedWriter already bounded the output, so there is no
	// byte-index cut that could split a multi-byte rune. Flatten only ever
	// shrinks its input, so Text is bounded too.
	markdown := stdout.buf.String()
	return Result{Title: FirstHeading(markdown), Markdown: markdown, Text: Flatten(markdown)}, nil
}

// limitedWriter accumulates up to limit bytes and then refuses further writes.
//
// Bounding the host buffer is what actually caps memory: a heavily compressed
// container can expand far past its upload size, and truncating after the fact
// would already have buffered the whole thing. Failing rather than truncating
// also avoids storing a silently half-converted document, and sidesteps cutting
// a multi-byte rune at the boundary — Postgres rejects invalid UTF-8 in a text
// column, which would fail the job identically on every retry.
type limitedWriter struct {
	buf      bytes.Buffer
	limit    int
	exceeded bool
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.exceeded || w.buf.Len()+len(p) > w.limit {
		w.exceeded = true
		return 0, ErrTooLarge
	}
	return w.buf.Write(p)
}
