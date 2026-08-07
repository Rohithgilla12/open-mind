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
	once     sync.Once
	initErr  error
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
}

// New returns a Converter. It does no work: the wasm module is compiled on the
// first Convert call.
func New() *Converter { return &Converter{} }

// init compiles the module exactly once. The runtime it builds is closed by
// Close.
func (c *Converter) init(ctx context.Context) error {
	c.once.Do(func() {
		// WithCloseOnContextDone lets a context deadline interrupt an
		// in-flight conversion; without it a wedged module would run to
		// completion regardless of the ceiling below.
		cfg := wazero.NewRuntimeConfig().
			WithCloseOnContextDone(true).
			WithMemoryLimitPages(memoryLimitPages)
		rt := wazero.NewRuntimeWithConfig(ctx, cfg)
		if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
			c.initErr = fmt.Errorf("instantiating wasi: %w", err)
			_ = rt.Close(ctx)
			return
		}
		compiled, err := rt.CompileModule(ctx, anydocWasm)
		if err != nil {
			c.initErr = fmt.Errorf("compiling anydoc wasm: %w", err)
			_ = rt.Close(ctx)
			return
		}
		c.runtime, c.compiled = rt, compiled
	})
	return c.initErr
}

// Close releases the compiled module and runtime. Safe to call on a Converter
// that was never used.
func (c *Converter) Close(ctx context.Context) error {
	if c.runtime == nil {
		return nil
	}
	return c.runtime.Close(ctx)
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

	ctx, cancel := context.WithTimeout(ctx, convertTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := c.init(ctx); err != nil {
		return Result{}, err
	}

	var stdout, stderr bytes.Buffer
	// WithName("") keeps instances anonymous so concurrent conversions cannot
	// collide on the module namespace.
	cfg := wazero.NewModuleConfig().
		WithName("").
		WithArgs("anydoc", string(format)).
		WithStdin(bytes.NewReader(data)).
		WithStdout(&stdout).
		WithStderr(&stderr)

	mod, err := c.runtime.InstantiateModule(ctx, c.compiled, cfg)
	if mod != nil {
		_ = mod.Close(ctx)
	}
	if err != nil {
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
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, ctxErr
		}
		return Result{}, fmt.Errorf("docmd: running anydoc: %w", err)
	}

	markdown := stdout.String()
	if len(markdown) > maxTextBytes {
		markdown = markdown[:maxTextBytes]
	}
	text := Flatten(markdown)
	if len(text) > maxTextBytes {
		text = text[:maxTextBytes]
	}
	return Result{Title: FirstHeading(markdown), Markdown: markdown, Text: text}, nil
}
