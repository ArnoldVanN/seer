#!/usr/bin/env bash
set -euo pipefail

# Runs the Go tests, including the ones that need Postgres, against a
# throwaway postgres:18 container.
#
# Usage:
#   scripts/test-integration.sh [go test args]   # e.g. -run TestHA -v

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONTAINER="seer-test-db"
PORT=15433

cleanup() { docker rm -f "$CONTAINER" >/dev/null 2>&1 || true; }
trap cleanup EXIT
cleanup

docker run --rm -d --name "$CONTAINER" -p "${PORT}:5432" \
  -e POSTGRES_USER=seer -e POSTGRES_PASSWORD=seer -e POSTGRES_DB=seer \
  postgres:18 >/dev/null
for _ in $(seq 1 30); do
  docker exec "$CONTAINER" pg_isready -U seer -d seer >/dev/null 2>&1 && break
  sleep 1
done

cd "$REPO_ROOT/apps/server"
SEER_TEST_DSN="postgres://seer:seer@localhost:${PORT}/seer?sslmode=disable" go test "${@:-./...}"
