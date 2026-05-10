#!/usr/bin/env sh
set -eu

REPO="myrrazor/atlas-tasker"
BIN_NAME="tracker"
BIN_DIR="${BIN_DIR:-/usr/local/bin}"
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

verify_archive() {
  archive_path="$1"
  archive_name="$2"
  if [ -n "$RELEASE_BASE_URL" ]; then
    checksums_url="${RELEASE_BASE_URL%/}/checksums.txt"
  else
    checksums_url="https://github.com/${REPO}/releases/download/${TAG}/checksums.txt"
  fi
  curl -fsSL "$checksums_url" -o "$TMP_DIR/checksums.txt"
  expected_sum="$(awk -v file="$archive_name" '
    {
      name = $2
      sub(/^.*\//, "", name)
      if (name == file) {
        print $1
        exit
      }
    }
  ' "$TMP_DIR/checksums.txt")"
  if [ -z "$expected_sum" ]; then
    echo "checksum entry missing for ${archive_name}" >&2
    exit 1
  fi
  actual_sum="$(checksum_file "$archive_path")"
  if [ "$expected_sum" != "$actual_sum" ]; then
    echo "checksum mismatch for ${archive_name}" >&2
    echo "expected: $expected_sum" >&2
    echo "actual:   $actual_sum" >&2
    exit 1
  fi
  if [ "$VERIFY_ATTESTATIONS" != "0" ]; then
    need_cmd gh
    gh attestation verify "$archive_path" --repo "$REPO" >/dev/null
  fi
}

detect_os() {
  case "$(uname -s)" in
    Darwin) printf 'darwin' ;;
    Linux) printf 'linux' ;;
    *) echo "unsupported operating system: $(uname -s)" >&2; exit 1 ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf 'amd64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
  esac
}

need_cmd curl
need_cmd awk
need_cmd tar
need_cmd mktemp
need_cmd install

OS_NAME="$(detect_os)"
ARCH_NAME="$(detect_arch)"
TAG="$(resolve_version)"
if [ -z "$TAG" ]; then
  echo "failed to resolve Atlas Tasker release version" >&2
  exit 1
fi
validate_version "$TAG"

VERSION_NO_V="${TAG#v}"
ARCHIVE="${BIN_NAME}_${VERSION_NO_V}_${OS_NAME}_${ARCH_NAME}.tar.gz"
if [ -n "$RELEASE_BASE_URL" ]; then
  validate_release_base_url "$RELEASE_BASE_URL"
  BASE_URL="${RELEASE_BASE_URL%/}"
  URL="${BASE_URL}/${ARCHIVE}"
else
  URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}"
fi
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

curl -fsSL "$URL" -o "$TMP_DIR/$ARCHIVE"
verify_archive "$TMP_DIR/$ARCHIVE" "$ARCHIVE"
tar -xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR"
install -d "$BIN_DIR"
install "$TMP_DIR/$BIN_NAME" "$BIN_DIR/$BIN_NAME"

echo "installed ${BIN_NAME} ${TAG} to ${BIN_DIR}/${BIN_NAME}"
