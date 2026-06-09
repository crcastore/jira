#!/usr/bin/env bash
set -euo pipefail

# Build and run the Rust URL chat server.
# Usage:
#   ./run.sh         # run the web chat
#   ./run.sh serve   # run the web chat
#   ./run.sh build   # compile only
#   ./run.sh check   # type-check only

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cmd="${1:-serve}"

cd "$ROOT_DIR"

case "$cmd" in
  build)
    cargo build
    ;;
  check)
    cargo check
    ;;
  serve|web)
    cargo run
    ;;
  *)
    echo "Unknown command: $cmd" >&2
    echo "Usage: ./run.sh [serve|web|build|check]" >&2
    exit 1
    ;;
esac
