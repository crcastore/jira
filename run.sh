#!/usr/bin/env bash
set -euo pipefail

# Build and run the jira-agent binary from the cmd entrypoint.
# Usage:
#   ./run.sh           # build + run web mode
#   ./run.sh serve     # build + run web mode
#   ./run.sh cli       # build + run CLI mode
#   ./run.sh build     # build only
#   ./run.sh test      # run tests

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_PATH="$ROOT_DIR/jira-agent"

cmd="${1:-serve}"

build() {
  echo "Building jira-agent..."
  go build -o "$BIN_PATH" ./cmd/jira-agent
}

run_cli() {
  echo "Running CLI mode..."
  exec "$BIN_PATH"
}

run_web() {
  echo "Running web mode on http://localhost:8080 ..."
  exec "$BIN_PATH" serve
}

run_tests() {
  echo "Running test suite..."
  go test ./...
}

cd "$ROOT_DIR"

case "$cmd" in
  build)
    build
    ;;
  test)
    run_tests
    ;;
  serve|web)
    build
    run_web
    ;;
  cli)
    build
    run_cli
    ;;
  *)
    # Forward any unknown args to the binary in CLI mode.
    build
    echo "Running with args: $*"
    exec "$BIN_PATH" "$@"
    ;;
esac
