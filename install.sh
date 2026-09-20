#!/usr/bin/env bash
# Install the szaidi.quran audio engine (quranproxyd daemon + quranctl CLI).
#
# Binaries are downloaded from a PINNED, immutable release tag and verified
# against SHA-256 digests committed in this source tree (checksums/<tag>.sha256).
# Installation fails closed if those digests are missing or do not match.
#
# Usage:
#   install.sh                 download + verify + install pinned binaries
#   install.sh --build         build from source instead (needs Go 1.22+)
#   install.sh --prefix DIR    install into DIR (default: ~/.local/bin)
#   install.sh --arch amd64    force an architecture (amd64|arm64)
#
# Idempotent, never needs sudo (installs into the user's own bin dir).

set -euo pipefail

# Immutable install coordinates. Deliberately NOT overridable from the
# environment: the download source must match the digests reviewed in-tree.
readonly REPO="szaidi-code/quran-plugin"
readonly PINNED_TAG="v1.1.2"

# Network & transfer safety bounds for binary release downloads.
# Release archives are ~4.6 MiB (amd64) and ~4.2 MiB (arm64); cap at 16 MiB.
readonly CONNECT_TIMEOUT=15          # seconds to establish TCP connection
readonly MAX_DOWNLOAD_TIME=120       # maximum seconds for complete download
readonly MAX_ARCHIVE_BYTES=16777216  # 16 MiB producer-side budget cap

PREFIX="${PREFIX:-$HOME/.local/bin}"
BUILD=0
ARCH=""

usage() {
  sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
  exit 0
}

die() {
  echo "install.sh: $*" >&2
  exit 1
}

while (($# > 0)); do
  case "$1" in
  --build) BUILD=1 ;;
  --prefix) PREFIX="$2"; shift ;;
  --arch) ARCH="$2"; shift ;;
  -h | --help) usage ;;
  *) die "unknown option: $1" ;;
  esac
  shift
done

case "$(uname -s)" in
Linux) ;;
*) die "unsupported OS: $(uname -s) (Linux only)" ;;
esac

if [[ -z $ARCH ]]; then
  case "$(uname -m)" in
  x86_64) ARCH=amd64 ;;
  aarch64 | arm64) ARCH=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
  esac
fi

case "$ARCH" in
amd64 | arm64) ;;
*) die "unsupported architecture: $ARCH (expected amd64 or arm64)" ;;
esac

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

mkdir -p "$PREFIX"

build_from_source() {
  command -v go >/dev/null 2>&1 || die "building needs a Go toolchain (go 1.22+)"
  CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -trimpath -ldflags="-s -w" \
    -o "$PREFIX/quranproxyd" ./cmd/quranproxyd
  CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -trimpath -ldflags="-s -w" \
    -o "$PREFIX/quranctl" ./cmd/quranctl
  echo "install.sh: built quranproxyd + quranctl (linux/$ARCH) into $PREFIX"
}

if ((BUILD)); then
  build_from_source
else
  command -v curl >/dev/null 2>&1 || die "curl is required for downloading (or use --build)"
  command -v sha256sum >/dev/null 2>&1 || die "sha256sum is required for verification (or use --build)"

  ARCHIVE="linux-$ARCH.tar.gz"
  CHECKSUM_FILE="$SRC_DIR/checksums/$PINNED_TAG.sha256"

  # Fail closed: the trusted digests must be present in the reviewed tree.
  [[ -f "$CHECKSUM_FILE" ]] ||
    die "missing trusted checksum file: $CHECKSUM_FILE
    refusing to install unverified binaries. Run from a full checkout, or use --build."

  EXPECTED="$(awk -v want="$ARCHIVE" '$2 == want { print $1; exit }' "$CHECKSUM_FILE")"
  [[ -n "$EXPECTED" ]] ||
    die "no trusted digest for $ARCHIVE in $CHECKSUM_FILE (refusing to install)"

  TMP="$(mktemp -d)"
  trap 'rm -rf "$TMP"' EXIT

  ARCHIVE_PATH="$TMP/$ARCHIVE"
  URL="https://github.com/$REPO/releases/download/$PINNED_TAG/$ARCHIVE"
  echo "install.sh: downloading $PINNED_TAG for linux/$ARCH..."

  # Download with explicit connect/overall timeouts and strict size cap.
  # Fail closed and clean up partial files immediately if the server hangs, aborts,
  # or attempts to stream beyond the maximum allowed archive size.
  if ! curl -fsSL \
      --connect-timeout "$CONNECT_TIMEOUT" \
      --max-time "$MAX_DOWNLOAD_TIME" \
      --max-filesize "$MAX_ARCHIVE_BYTES" \
      "$URL" -o "$ARCHIVE_PATH"; then
    rm -f "$ARCHIVE_PATH"
    die "download failed, timed out, or exceeded maximum size limit ($MAX_ARCHIVE_BYTES bytes): $URL"
  fi

  # Strictly enforce maximum archive size and reject/delete oversized partials before hashing.
  [[ -f "$ARCHIVE_PATH" ]] || die "downloaded archive is missing: $ARCHIVE_PATH"
  ARCHIVE_SIZE="$(wc -c < "$ARCHIVE_PATH" | tr -d ' ')"
  if (( ARCHIVE_SIZE == 0 )); then
    rm -f "$ARCHIVE_PATH"
    die "downloaded archive is empty"
  fi
  if (( ARCHIVE_SIZE > MAX_ARCHIVE_BYTES )); then
    rm -f "$ARCHIVE_PATH"
    die "downloaded archive exceeds maximum allowed size ($ARCHIVE_SIZE > $MAX_ARCHIVE_BYTES bytes); deleted partial file"
  fi

  # Verify the archive BEFORE extracting anything from it.
  ACTUAL="$(sha256sum "$ARCHIVE_PATH" | awk '{print $1}')"
  if [[ "$ACTUAL" != "$EXPECTED" ]]; then
    rm -f "$ARCHIVE_PATH"
    die "checksum mismatch for $ARCHIVE
    expected: $EXPECTED
    actual:   $ACTUAL
    refusing to install."
  fi
  echo "install.sh: verified $ARCHIVE against committed digest"

  # Defence in depth: confirm GitHub build provenance when gh is available.
  if command -v gh >/dev/null 2>&1; then
    if gh attestation verify "$ARCHIVE_PATH" --repo "$REPO" >/dev/null 2>&1; then
      echo "install.sh: verified build provenance attestation"
    else
      echo "install.sh: note: could not verify provenance attestation (gh offline or unauthenticated);" >&2
      echo "install.sh: committed-digest verification already passed." >&2
    fi
  fi

  tar -xzf "$ARCHIVE_PATH" -C "$TMP"
  for bin in quranproxyd quranctl; do
    [[ -f "$TMP/linux-$ARCH/$bin" ]] || die "archive is missing $bin (refusing to install)"
  done

  install -m 0755 "$TMP/linux-$ARCH/quranproxyd" "$PREFIX/quranproxyd"
  install -m 0755 "$TMP/linux-$ARCH/quranctl" "$PREFIX/quranctl"
  echo "install.sh: installed quranproxyd + quranctl ($PINNED_TAG, linux/$ARCH) into $PREFIX"
fi

echo "install.sh: restart your Omarchy shell (or re-enable the plugin) to load the engine."
