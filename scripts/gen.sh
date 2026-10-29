#!/usr/bin/env bash
set -euo pipefail

# Regenerates protobuf code (Go + TS) and ent code, in parallel.
#
# Needs buf, protoc-gen-go and protoc-gen-connect-go on PATH:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
#   (buf comes from the @bufbuild/buf dev dependency, or brew/go install)

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROTO_DIR="$REPO_ROOT/packages/protos"
PATH="$PROTO_DIR/node_modules/.bin:$PATH"

gen_rpc() {
  echo "Generating RPC code…"
  cd "$PROTO_DIR"
  rm -rf ./gen
  buf lint
  buf generate --template buf.gen.go.yaml
  buf generate --template buf.gen.ts.yaml
  pnpm run build
}

gen_ent() {
  echo "Generating ent code…"
  cd "$REPO_ROOT/apps/server"
  go generate ./ent
}

LOG_DIR="$(mktemp -d)"
trap 'rm -rf "$LOG_DIR"' EXIT

pids=()
names=()
start() {
  ("$1") >"$LOG_DIR/$1.log" 2>&1 &
  pids+=("$!")
  names+=("$1")
}

start gen_rpc
start gen_ent

status=0
for i in "${!pids[@]}"; do
  wait "${pids[$i]}" || status=1
  cat "$LOG_DIR/${names[$i]}.log"
done
[ "$status" -eq 0 ] || { echo "Generation failed"; exit 1; }
echo "Done"
