#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$SCRIPT_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is required, but the docker command was not found." >&2
  exit 1
fi

if docker compose version >/dev/null 2>&1; then
  COMPOSE="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE="docker-compose"
else
  echo "Docker Compose is required, but neither 'docker compose' nor 'docker-compose' is available." >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "Docker is not running. Start Docker Desktop and try again." >&2
  exit 1
fi

mkdir -p artifacts

cleanup() {
  $COMPOSE down --remove-orphans >/dev/null 2>&1 || true
}

trap cleanup EXIT INT TERM

echo "Running Qiskit IBM Runtime ZAP capture..."
$COMPOSE up --build --abort-on-container-exit --exit-code-from client client

echo
echo "Done. Artifacts written to:"
echo "  artifacts/http-transcript.md"
echo "  artifacts/zap-messages.json"