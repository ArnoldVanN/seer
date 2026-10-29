#!/usr/bin/env bash
set -euo pipefail

# Brings up the local dev Postgres (docker-compose.dev.yml), applies
# migrations and seeds a year of demo history (cmd/seed). Idempotent: safe to
# re-run; the seed skips once the history exists.
#
# Usage:
#   scripts/dev-db-up.sh            # up + migrate + seed
#   scripts/dev-db-up.sh --no-seed  # up + migrate only
#   (cd apps/server && go run ./cmd/seed -force)   # regenerate the demo history
#
# Then run the stack:
#   cd apps/server && go run ./cmd/seer all -config ../../seer.example.yaml
#   cd apps/web && pnpm dev

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SEED=1
[[ "${1:-}" == "--no-seed" ]] && SEED=0
export SEER_DB_DSN="${SEER_DB_DSN:-postgres://seer:seer@localhost:5433/seer?sslmode=disable}"

echo "Starting dev Postgres (waiting for healthy)…"
docker compose -f "$REPO_ROOT/docker-compose.dev.yml" up -d --wait

echo "Applying migrations…"
( cd "$REPO_ROOT/apps/server" && go run ./cmd/seer migrate -config "$REPO_ROOT/seer.example.yaml" )

if [[ "$SEED" == "1" ]]; then
  echo "Seeding demo history…"
  ( cd "$REPO_ROOT/apps/server" && go run ./cmd/seed -config "$REPO_ROOT/seer.example.yaml" )
fi

echo "Dev DB ready: $SEER_DB_DSN"
