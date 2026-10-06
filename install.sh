#!/bin/sh
# Installs the lazyreview binary from a GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/adelplace/lazyreview/master/install.sh | sh
#
# Environment:
#   INSTALL_DIR  destination directory (default: ~/.local/bin)
#   VERSION      release tag to install, e.g. v0.1.0 (default: latest)
#   BASE_URL     override the download location (directory holding the assets)
set -eu

REPO="adelplace/lazyreview"
BIN="lazyreview"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${VERSION:-latest}"

die() {
  echo "install.sh: $*" >&2
  exit 1
}

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported OS: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

asset="${BIN}_${os}_${arch}.tar.gz"

if [ -z "${BASE_URL:-}" ]; then
  if [ "$VERSION" = latest ]; then
    BASE_URL="https://github.com/$REPO/releases/latest/download"
  else
    BASE_URL="https://github.com/$REPO/releases/download/$VERSION"
  fi
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "Downloading $BASE_URL/$asset"
curl -fsSL -o "$tmp/$asset" "$BASE_URL/$asset" || die "download failed: $BASE_URL/$asset"
curl -fsSL -o "$tmp/checksums.txt" "$BASE_URL/checksums.txt" || die "download failed: $BASE_URL/checksums.txt"

expected="$(awk -v f="$asset" '$2 == f { print $1 }' "$tmp/checksums.txt")"
[ -n "$expected" ] || die "no checksum for $asset in checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp/$asset" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "$tmp/$asset" | awk '{ print $1 }')"
else
  die "sha256sum or shasum is required to verify the download"
fi
[ "$expected" = "$actual" ] || die "checksum mismatch for $asset"

tar -xzf "$tmp/$asset" -C "$tmp" "$BIN"
mkdir -p "$INSTALL_DIR"
install -m 755 "$tmp/$BIN" "$INSTALL_DIR/$BIN"

echo "Installed $BIN to $INSTALL_DIR/$BIN"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "Note: $INSTALL_DIR is not in your PATH." ;;
esac
