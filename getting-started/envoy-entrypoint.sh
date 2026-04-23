#!/bin/sh
set -e

WASM_URL="${SOFTPROBE_WASM_URL:-https://storage.googleapis.com/softprobe-published-files/agent/proxy-wasm/latest/sp_istio_agent.wasm}"
WASM_LOCAL_PATH="${SOFTPROBE_WASM_LOCAL_PATH:-/tmp/sp_istio_agent.wasm}"

ensure_downloader() {
  if command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1; then
    return 0
  fi

  echo "INFO: curl/wget not found; attempting to install curl..."
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update && apt-get install -y --no-install-recommends curl ca-certificates
    rm -rf /var/lib/apt/lists/*
  elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache curl ca-certificates
  elif command -v yum >/dev/null 2>&1; then
    yum install -y curl ca-certificates
  elif command -v microdnf >/dev/null 2>&1; then
    microdnf install -y curl ca-certificates
  else
    echo "ERROR: no supported package manager found to install curl" >&2
    return 1
  fi
}

if [ -z "$SOFTPROBE_API_TOKEN" ]; then
  echo "ERROR: SOFTPROBE_API_TOKEN is not set. Get your token at https://https://dashboard.softprobe.ai" >&2
  exit 1
fi

ensure_downloader

if command -v curl >/dev/null 2>&1; then
  mkdir -p "$(dirname "$WASM_LOCAL_PATH")"
  curl -fsSL "$WASM_URL" -o "$WASM_LOCAL_PATH"
elif command -v wget >/dev/null 2>&1; then
  mkdir -p "$(dirname "$WASM_LOCAL_PATH")"
  wget -qO "$WASM_LOCAL_PATH" "$WASM_URL"
else
  echo "ERROR: curl or wget is required to fetch the latest WASM file" >&2
  exit 1
fi

if [ ! -s "$WASM_LOCAL_PATH" ]; then
  echo "ERROR: downloaded WASM file is missing or empty: $WASM_LOCAL_PATH" >&2
  exit 1
fi

API_TOKEN_ESCAPED="$(printf '%s' "$SOFTPROBE_API_TOKEN" | sed 's/[\\/&]/\\&/g')"

sed \
  -e "s/__SOFTPROBE_API_TOKEN__/$API_TOKEN_ESCAPED/g" \
  /etc/envoy/envoy.yaml > /tmp/envoy-rendered.yaml

exec /usr/local/bin/envoy -c /tmp/envoy-rendered.yaml "$@"
