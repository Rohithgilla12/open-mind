//! WASI wrapper around anydoc, built for `wasm32-wasip1` and run under wazero
//! by `apps/api/internal/docmd`.
//!
//! Protocol, deliberately minimal so the Go side needs no ABI knowledge:
//!
//! - stdin  — the document bytes
//! - argv[1] — a bare format extension ("docx", "odt", "rtf", "epub")
//! - stdout — GitHub-Flavored Markdown on success
//! - stderr — a single-line error message on failure
//! - exit   — 0 success, 1 conversion failed, 2 bad invocation
//!
//! The format is named rather than detected so the Go allowlist stays
//! authoritative: a file that sniffs as .docx on the Go side can never be
//! parsed here as a format Openmind chose not to support.

use std::io::{Read, Write};

const EXIT_CONVERT_FAILED: i32 = 1;
const EXIT_BAD_INVOCATION: i32 = 2;

fn main() {
    let Some(ext) = std::env::args().nth(1) else {
        fail(EXIT_BAD_INVOCATION, "missing format argument");
    };
    let Some(format) = anydoc::Format::from_extension(&ext) else {
        fail(EXIT_BAD_INVOCATION, &format!("unsupported format: {ext}"));
    };

    let mut bytes = Vec::new();
    if let Err(e) = std::io::stdin().read_to_end(&mut bytes) {
        fail(EXIT_BAD_INVOCATION, &format!("reading stdin: {e}"));
    }

    match anydoc::to_markdown_bytes(&bytes, format) {
        Ok(markdown) => {
            let mut stdout = std::io::stdout();
            if let Err(e) = stdout.write_all(markdown.as_bytes()) {
                fail(EXIT_CONVERT_FAILED, &format!("writing stdout: {e}"));
            }
            if let Err(e) = stdout.flush() {
                fail(EXIT_CONVERT_FAILED, &format!("flushing stdout: {e}"));
            }
        }
        Err(e) => fail(EXIT_CONVERT_FAILED, &e.to_string()),
    }
}

/// Writes a single-line message to stderr and exits. Never returns.
fn fail(code: i32, msg: &str) -> ! {
    let _ = writeln!(std::io::stderr(), "{msg}");
    std::process::exit(code)
}
