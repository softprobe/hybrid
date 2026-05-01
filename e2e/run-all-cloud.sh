#!/usr/bin/env bash
set -euo pipefail

# Runs all e2e harnesses against a hosted runtime.
# Defaults to runtime.softprobe.dev and reads SOFTPROBE_API_KEY from ../.env.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

SOFTPROBE_RUNTIME_URL="${SOFTPROBE_RUNTIME_URL:-https://runtime.softprobe.dev}"
APP_URL="${APP_URL:-http://127.0.0.1:8081}"
PROXY_URL="${PROXY_URL:-http://127.0.0.1:8082}"
UPSTREAM_URL="${UPSTREAM_URL:-http://127.0.0.1:8083}"

if [[ -f "${REPO_ROOT}/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${REPO_ROOT}/.env"
  set +a
fi

if [[ -z "${SOFTPROBE_API_KEY:-}" ]]; then
  echo "SOFTPROBE_API_KEY is required (set env var or ${REPO_ROOT}/.env)."
  exit 1
fi

export SOFTPROBE_RUNTIME_URL
export SOFTPROBE_API_KEY
export SOFTPROBE_API_TOKEN="${SOFTPROBE_API_TOKEN:-$SOFTPROBE_API_KEY}"
export RUNTIME_URL="$SOFTPROBE_RUNTIME_URL"
export APP_URL
export PROXY_URL
export UPSTREAM_URL

echo "Runtime: ${SOFTPROBE_RUNTIME_URL}"
echo "App URL: ${APP_URL}"
echo "Proxy:   ${PROXY_URL}"

cd "${SCRIPT_DIR}"
python3 ./scripts/render-envoy-cloud.py
docker compose -f docker-compose.yaml -f docker-compose.cloud.yaml up --build --wait softprobe-proxy upstream app

trap 'docker compose -f docker-compose.yaml -f docker-compose.cloud.yaml down || true' EXIT

go test ./hosted/ -v -count=1
go test ./go/... -count=1

(cd jest-replay && npm install && npm test)
(cd jest-hooks && npm install && npm test)

python3 -m pytest pytest-replay/ -v

(cd "${REPO_ROOT}/softprobe-java" && mvn -q install -DskipTests)
(cd junit-replay && mvn test)
