#!/usr/bin/env bash
set -euo pipefail

# gurtcli — install script
# Usage: curl -fsSL https://github.com/sillygru/gurtcli/releases/latest/download/install.sh | bash

INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"
BINARY_NAME="gurtcli"
REPO="sillygru/gurtcli"

# ---- helpers ----

die() {
  echo "error: $*" >&2
  exit 1
}

cleanup() {
  [ -n "${TMPDIR:-}" ] && [ -d "$TMPDIR" ] && rm -rf "$TMPDIR"
}
trap cleanup EXIT

# sha256sum on Linux, shasum -a 256 on macOS
sha256check() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    die "no sha256 tool found (install coreutils or shasum)"
  fi
}

# ---- platform detection ----

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  darwin|linux) ;;
  mingw*|msys*|cygwin*) OS="windows" ;;
  *) die "unsupported OS: $OS" ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *) die "unsupported architecture: $ARCH" ;;
esac

# ---- fetch latest release tag ----

echo "Fetching latest release..."

LATEST="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | \
  grep '"tag_name":' | \
  sed 's/.*"tag_name": "v//;s/".*//')"

[ -n "$LATEST" ] || die "could not determine latest release version"

echo "Found gurtcli v${LATEST}"

# ---- download ----

ARCHIVE_NAME="gurtcli_${LATEST}_${OS}_${ARCH}.tar.gz"
if [ "$OS" = "windows" ]; then
  ARCHIVE_NAME="gurtcli_${LATEST}_${OS}_${ARCH}.zip"
fi

BASE_URL="https://github.com/${REPO}/releases/download/v${LATEST}"
ARCHIVE_URL="${BASE_URL}/${ARCHIVE_NAME}"
CHECKSUMS_URL="${BASE_URL}/checksums.txt"

TMPDIR="$(mktemp -d)"
ARCHIVE_PATH="${TMPDIR}/${ARCHIVE_NAME}"
CHECKSUMS_PATH="${TMPDIR}/checksums.txt"

echo "Downloading ${ARCHIVE_NAME}..."
curl -fsSL "$ARCHIVE_URL" -o "$ARCHIVE_PATH" || die "download failed"

echo "Verifying checksum..."
curl -fsSL "$CHECKSUMS_URL" -o "$CHECKSUMS_PATH" 2>/dev/null || true

if [ -s "$CHECKSUMS_PATH" ]; then
  EXPECTED="$(grep "$ARCHIVE_NAME" "$CHECKSUMS_PATH" | awk '{print $1}')"
  if [ -n "$EXPECTED" ]; then
    ACTUAL="$(sha256check "$ARCHIVE_PATH")"
    if [ "$EXPECTED" != "$ACTUAL" ]; then
      die "checksum mismatch for ${ARCHIVE_NAME}"
    fi
    echo "  Checksum verified"
  fi
else
  echo "  (no checksum file to verify against)"
fi

# ---- extract ----

echo "Extracting..."
mkdir -p "$INSTALL_DIR" "$TMPDIR/extracted"

if [ "$OS" = "windows" ]; then
  if command -v unzip >/dev/null 2>&1; then
    unzip -o "$ARCHIVE_PATH" -d "$TMPDIR/extracted" >/dev/null
  else
    die "unzip is required to install on Windows"
  fi
else
  tar -xzf "$ARCHIVE_PATH" -C "$TMPDIR/extracted"
fi

# Find the binary (may be in a subdirectory inside the archive)
BIN_PATH="$(find "$TMPDIR/extracted" -type f -name "$BINARY_NAME" 2>/dev/null | head -1)"
[ -n "$BIN_PATH" ] || die "binary not found in archive"

INSTALLED="${INSTALL_DIR}/${BINARY_NAME}"
mv "$BIN_PATH" "$INSTALLED"
chmod 755 "$INSTALLED"

echo "gurtcli v${LATEST} installed to ${INSTALLED}"

# ---- PATH check ----

case ":$PATH:" in
  *:"${INSTALL_DIR}":*) ;;
  *)
    echo ""
    echo "  ${INSTALL_DIR} is not in your PATH."
    echo "  Add it to your shell profile:"
    echo ""
    echo "    echo 'export PATH=\"\${PATH}:${INSTALL_DIR}\"' >> ~/.$(basename "${SHELL:-bash}")rc"
    echo "    source ~/.$(basename "${SHELL:-bash}")rc"
    echo ""
    ;;
esac

echo "Run 'gurtcli' to start."