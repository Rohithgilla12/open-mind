#!/usr/bin/env bash
#
# Rebuilds apps/api/internal/docmd/anydoc.wasm from tools/anydoc-wasi.
#
# The resulting artefact is committed to the repo and go:embed-ed, so building
# Openmind needs no Rust toolchain. Run this only when bumping the pinned
# anydoc version in tools/anydoc-wasi/Cargo.toml, then commit the new .wasm
# alongside the manifest change.
#
# Requires: rustup, and the wasm32-wasip1 target (installed automatically).

set -euo pipefail

readonly TARGET=wasm32-wasip1

# Declared and assigned separately on purpose: `readonly X="$(cmd)"` takes its
# exit status from readonly, not the command substitution, so `set -e` would not
# abort on a failed cd and ROOT would silently hold the wrong path.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly ROOT
readonly CRATE_DIR="$ROOT/tools/anydoc-wasi"
readonly OUT="$ROOT/apps/api/internal/docmd/anydoc.wasm"

if ! command -v rustup >/dev/null 2>&1; then
  echo "error: rustup not found; install from https://rustup.rs" >&2
  exit 1
fi

if ! rustup target list --installed | grep -qx "$TARGET"; then
  echo "installing Rust target $TARGET..."
  rustup target add "$TARGET"
fi

echo "building anydoc-wasi for $TARGET..."
cargo build --manifest-path "$CRATE_DIR/Cargo.toml" --release --target "$TARGET"

cp "$CRATE_DIR/target/$TARGET/release/anydoc-wasi.wasm" "$OUT"

echo "wrote $OUT ($(du -h "$OUT" | cut -f1))"
echo "commit the artefact together with any Cargo.toml change."
