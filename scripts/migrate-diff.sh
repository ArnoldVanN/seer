#!/usr/bin/env bash
set -euo pipefail

# Generates a new versioned migration by diffing the ent schema
# (apps/server/ent/schema) against a throwaway Postgres, writing it to
# apps/server/migrations/postgres/<timestamp>_<name>.sql.
#
# Run `pnpm gen` (or `go generate ./ent`) after editing the schema first.
#
# Usage:
#   scripts/migrate-diff.sh <name>     # e.g. scripts/migrate-diff.sh add_incidents
#
# Then apply it to the local dev DB:
#   scripts/dev-db-up.sh

if [[ $# -ne 1 ]]; then
  echo "usage: scripts/migrate-diff.sh <migration_name>" >&2
  exit 1
fi
NAME="$1"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVER_DIR="$REPO_ROOT/apps/server"

CONTAINER="seer-atlas-dev"
PORT=15432
DEV_URL="postgres://user:pass@localhost:${PORT}/dev?sslmode=disable"

cleanup() { docker rm -f "$CONTAINER" >/dev/null 2>&1 || true; }
trap cleanup EXIT

cleanup # remove any leftover from a previous interrupted run
echo "Starting throwaway postgres:18 on :${PORT}…"
docker run --rm -d --name "$CONTAINER" -p "${PORT}:5432" \
  -e POSTGRES_USER=user -e POSTGRES_PASSWORD=pass -e POSTGRES_DB=dev \
  postgres:18 >/dev/null

echo "Waiting for it to accept connections…"
for _ in $(seq 1 30); do
  if docker exec "$CONTAINER" pg_isready -U user -d dev >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "Generating migration '${NAME}'…"
( cd "$SERVER_DIR" && ATLAS_DEV_URL="$DEV_URL" go run ./cmd/migrate-diff "$NAME" )

echo "Done. Review the new file in apps/server/migrations/postgres/, then:"
echo "  scripts/dev-db-up.sh"
