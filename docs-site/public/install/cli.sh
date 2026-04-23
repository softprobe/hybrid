#!/usr/bin/env sh
# Install the softprobe CLI binary.
# Usage: curl -fsSL https://docs.softprobe.dev/install/cli.sh | sh
set -e

REPO="softprobe/hybrid"
BIN="softprobe"
GCS_BUCKET="softprobe-published-files"
GCS_PREFIX="cli/softprobe"
INSTALL_DIR="${SOFTPROBE_INSTALL_DIR:-/usr/local/bin}"

# Resolve OS
case "$(uname -s)" in
  Linux)  OS="linux" ;;
  Darwin) OS="darwin" ;;
  *)
    echo "Unsupported OS: $(uname -s)" >&2
    exit 1
    ;;
esac

# Resolve arch
case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

# Resolve latest version from GitHub API
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' | sed 's/.*"tag_name": *"\(.*\)".*/\1/')

if [ -z "$LATEST" ]; then
  echo "Could not determine latest release version." >&2
  exit 1
fi

ASSET="${BIN}-${OS}-${ARCH}"
URL="https://storage.googleapis.com/${GCS_BUCKET}/${GCS_PREFIX}/${LATEST}/${ASSET}"

echo "Installing softprobe ${LATEST} (${OS}/${ARCH}) → ${INSTALL_DIR}/${BIN}"

TMPFILE=$(mktemp)
trap 'rm -f "$TMPFILE"' EXIT

curl -fsSL "$URL" -o "$TMPFILE"
chmod +x "$TMPFILE"

# Install — try without sudo first, fall back with sudo
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMPFILE" "${INSTALL_DIR}/${BIN}"
else
  echo "Requesting sudo to write to ${INSTALL_DIR}"
  sudo mv "$TMPFILE" "${INSTALL_DIR}/${BIN}"
fi

echo "Done. Run: softprobe doctor"
