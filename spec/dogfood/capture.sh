#!/usr/bin/env bash
# Dogfood capture driver.
# Runs against the local e2e compose stack, captures the canonical CLI flow,
# and writes to spec/examples/cases/control-plane-v1.case.json.
#
# Prerequisites:
#   docker compose -f e2e/docker-compose.yaml up --build --wait
#
# Usage:
#   bash spec/dogfood/capture.sh
set -euo pipefail

RUNTIME_URL="${RUNTIME_URL:-http://127.0.0.1:8080}"
PROXY_URL="${PROXY_URL:-http://127.0.0.1:8082}"
OUT="${OUT:-spec/examples/cases/control-plane-v1.case.json}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "Starting capture session..."
SESSION_ID=$(softprobe session start \
  --runtime-url "$RUNTIME_URL" \
  --mode capture \
  --json | jq -r .sessionId)
echo "Session: $SESSION_ID"

echo "Running canonical CLI flow..."
# Drive one request through the proxy so the ingress and egress spans are captured.
curl -sf -H "x-softprobe-session-id: $SESSION_ID" "$PROXY_URL/hello" > /dev/null

echo "Closing session..."
softprobe session close \
  --runtime-url "$RUNTIME_URL" \
  --session "$SESSION_ID"

echo "Fetching case..."
softprobe cases get "$SESSION_ID" \
  --runtime-url "$RUNTIME_URL" \
  --out "$REPO_ROOT/$OUT"

echo "Canonicalizing trace IDs..."
# Replace actual session/trace IDs with stable placeholders so the case file
# is deterministic across runs. sed is safe here because trace IDs are hex.
PLACEHOLDER_SESSION="dogfood-session-00000000"
sed -i.bak \
  "s/$SESSION_ID/$PLACEHOLDER_SESSION/g" \
  "$REPO_ROOT/$OUT"
rm -f "$REPO_ROOT/$OUT.bak"

echo "Done: $OUT"
