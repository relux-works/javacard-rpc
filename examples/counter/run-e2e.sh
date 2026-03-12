#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
BRIDGE_LOG="$(mktemp -t counter-bridge.XXXXXX.log)"
BRIDGE_PID=""

cleanup() {
  local status=$?

  if [ -n "$BRIDGE_PID" ] && kill -0 "$BRIDGE_PID" 2>/dev/null; then
    kill "$BRIDGE_PID" 2>/dev/null || true
    wait "$BRIDGE_PID" 2>/dev/null || true
  fi

  if [ $status -ne 0 ] && [ -f "$BRIDGE_LOG" ]; then
    echo "[run-e2e] bridge log:"
    cat "$BRIDGE_LOG"
  fi

  rm -f "$BRIDGE_LOG"
  trap - EXIT INT TERM
  exit $status
}

trap cleanup EXIT INT TERM

echo "[run-e2e] generating and building counter example..."
make -C "$REPO_ROOT" generate build-bridge build-applet build-cli

echo "[run-e2e] starting bridge..."
JCRPC_SKIP_BUILD=1 "$SCRIPT_DIR/run-bridge.sh" >"$BRIDGE_LOG" 2>&1 &
BRIDGE_PID=$!

for _ in $(seq 1 60); do
  if ! kill -0 "$BRIDGE_PID" 2>/dev/null; then
    echo "[run-e2e] bridge exited before becoming ready"
    exit 1
  fi

  if grep -q "\[bridge\] listening on" "$BRIDGE_LOG"; then
    echo "[run-e2e] bridge is ready"
    break
  fi

  sleep 1
done

if ! grep -q "\[bridge\] listening on" "$BRIDGE_LOG"; then
  echo "[run-e2e] bridge did not become ready in time"
  exit 1
fi

echo "[run-e2e] running Swift E2E harness..."
(cd "$SCRIPT_DIR/cli" && swift run)
