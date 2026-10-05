#!/usr/bin/env bash
set -euo pipefail
TASK_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
TASK_BIN="$(mktemp -d "${TMPDIR:-/tmp}/project-agent-dev.XXXXXX")"
TASK_API_PID=""
TASK_UI_PID=""
cleanup() {
  if [[ -n "$TASK_UI_PID" ]]; then kill "$TASK_UI_PID" 2>/dev/null || true; fi
  if [[ -n "$TASK_API_PID" ]]; then kill "$TASK_API_PID" 2>/dev/null || true; fi
  rm -rf -- "$TASK_BIN"
}
trap cleanup EXIT INT TERM
(cd "$TASK_ROOT/backend" && go build -o "$TASK_BIN/server" ./cmd/server)
export AGENT_EXECUTION_MODE="${AGENT_EXECUTION_MODE:-${AGENT_MODE:-local}}"
export AGENT_DECISION_MODE="${AGENT_DECISION_MODE:-rules}"
export AGENT_DELIVERY_MODE="${AGENT_DELIVERY_MODE:-local}"
export AGENT_DATA_DIR="${AGENT_DATA_DIR:-$TASK_ROOT/.data}"
export HTTP_ADDR="${HTTP_ADDR:-127.0.0.1:8080}"
"$TASK_BIN/server" &
TASK_API_PID=$!
case "$HTTP_ADDR" in
  :*) TASK_PROXY="http://127.0.0.1$HTTP_ADDR" ;;
  0.0.0.0:*) TASK_PROXY="http://127.0.0.1:${HTTP_ADDR##*:}" ;;
  *) TASK_PROXY="http://$HTTP_ADDR" ;;
esac
export AGENT_API_URL="${AGENT_API_URL:-$TASK_PROXY}"
for TASK_ATTEMPT in {1..40}; do
  if ! kill -0 "$TASK_API_PID" 2>/dev/null; then exit 1; fi
  if curl --silent --fail "$AGENT_API_URL/api/health" >/dev/null; then break; fi
  sleep .25
done
curl --silent --fail "$AGENT_API_URL/api/health" >/dev/null
node "$TASK_ROOT/frontend/node_modules/vite/bin/vite.js" "$TASK_ROOT/frontend" --host 127.0.0.1 &
TASK_UI_PID=$!
while kill -0 "$TASK_API_PID" 2>/dev/null && kill -0 "$TASK_UI_PID" 2>/dev/null; do sleep 1; done
