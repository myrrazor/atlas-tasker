#!/usr/bin/env sh
set -eu

REPO="${REPO:-myrrazor/atlas-tasker}"
VERSION="${VERSION:-latest}"
RELEASE_BASE_URL="${RELEASE_BASE_URL:-}"
VERIFY_ATTESTATIONS="${VERIFY_ATTESTATIONS:-1}"
ALLOW_INSECURE_RELEASE_BASE_URL="${ALLOW_INSECURE_RELEASE_BASE_URL:-0}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

resolve_version() {
  if [ "$VERSION" != "latest" ]; then
    printf '%s' "$VERSION"
    return
  fi
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | sed -n 's/.*"tag_name": "\([^"]*\)".*/\1/p' \
    | head -n1
}

validate_version() {
  case "$1" in
    ""|*[!abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._+-]*)
      echo "unsafe release version: $1" >&2
      exit 1
      ;;
  esac
}

validate_release_base_url() {
  base_url="$1"
  case "$base_url" in
    https://*) return ;;
    http://127.0.0.1:*|http://localhost:*|http://\[::1\]:*)
      if [ "$ALLOW_INSECURE_RELEASE_BASE_URL" = "1" ]; then
        return
      fi
      ;;
  esac
  echo "RELEASE_BASE_URL must use https://; loopback http requires ALLOW_INSECURE_RELEASE_BASE_URL=1" >&2
  exit 1
}

checksum_file() {
  path="$1"
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$path" | awk '{print $1}'
    return
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$path" | awk '{print $1}'
    return
  fi
  echo "missing required command: shasum or sha256sum" >&2
  exit 1
}

need_cmd curl
need_cmd awk
need_cmd basename
need_cmd mktemp

if [ "$#" -ne 1 ]; then
  echo "usage: scripts/verify-release.sh <archive-path>" >&2
  exit 1
fi

ARCHIVE_PATH="$1"
if [ ! -f "$ARCHIVE_PATH" ]; then
  echo "archive not found: $ARCHIVE_PATH" >&2
  exit 1
fi

TAG="$(resolve_version)"
if [ -z "$TAG" ]; then
  echo "failed to resolve Atlas Tasker release version" >&2
  exit 1
fi
validate_version "$TAG"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

if [ -n "$RELEASE_BASE_URL" ]; then
  validate_release_base_url "$RELEASE_BASE_URL"
  CHECKSUMS_URL="${RELEASE_BASE_URL%/}/checksums.txt"
else
  CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${TAG}/checksums.txt"
fi

curl -fsSL "$CHECKSUMS_URL" -o "$TMP_DIR/checksums.txt"

ARCHIVE_NAME="$(basename "$ARCHIVE_PATH")"
EXPECTED_SUM="$(awk -v file="$ARCHIVE_NAME" '
  {
    name = $2
    sub(/^.*\//, "", name)
    if (name == file) {
      print $1
      exit
    }
  }
' "$TMP_DIR/checksums.txt")"

if [ -z "$EXPECTED_SUM" ]; then
  echo "checksum entry missing for ${ARCHIVE_NAME}" >&2
  exit 1
fi

ACTUAL_SUM="$(checksum_file "$ARCHIVE_PATH")"
if [ "$EXPECTED_SUM" != "$ACTUAL_SUM" ]; then
  echo "checksum mismatch for ${ARCHIVE_NAME}" >&2
  echo "expected: $EXPECTED_SUM" >&2
  echo "actual:   $ACTUAL_SUM" >&2
  exit 1
fi

if [ "$VERIFY_ATTESTATIONS" != "0" ]; then
  need_cmd gh
  gh attestation verify "$ARCHIVE_PATH" --repo "$REPO" >/dev/null
fi

echo "verified ${ARCHIVE_NAME} for ${TAG}"
