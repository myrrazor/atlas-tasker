#!/usr/bin/env sh
set -eu

REPO="myrrazor/atlas-tasker"
BIN_NAME="tracker"
BIN_DIR="${BIN_DIR:-/usr/local/bin}"
VERSION="${VERSION:-latest}"
RELEASE_BASE_URL="${RELEASE_BASE_URL:-}"

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
need_cmd tar
need_cmd awk
need_cmd mktemp

OS_NAME="$(detect_os)"
ARCH_NAME="$(detect_arch)"
TAG="$(resolve_version)"
if [ -z "$TAG" ]; then
  echo "failed to resolve Atlas Tasker release version" >&2
  exit 1
fi

VERSION_NO_V="${TAG#v}"
ARCHIVE="${BIN_NAME}_${VERSION_NO_V}_${OS_NAME}_${ARCH_NAME}.tar.gz"
if [ -n "$RELEASE_BASE_URL" ]; then
  BASE_URL="${RELEASE_BASE_URL%/}"
  URL="${BASE_URL}/${ARCHIVE}"
  CHECKSUMS_URL="${BASE_URL}/checksums.txt"
else
  URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}"
  CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${TAG}/checksums.txt"
fi
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

curl -fsSL "$URL" -o "$TMP_DIR/$ARCHIVE"
curl -fsSL "$CHECKSUMS_URL" -o "$TMP_DIR/checksums.txt"

EXPECTED_SUM="$(awk -v file="$ARCHIVE" '
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
  echo "checksum entry missing for ${ARCHIVE}" >&2
  exit 1
fi

ACTUAL_SUM="$(checksum_file "$TMP_DIR/$ARCHIVE")"
if [ "$EXPECTED_SUM" != "$ACTUAL_SUM" ]; then
  echo "checksum mismatch for ${ARCHIVE}" >&2
  echo "expected: $EXPECTED_SUM" >&2
  echo "actual:   $ACTUAL_SUM" >&2
  exit 1
fi

tar -xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR"
install -d "$BIN_DIR"
install "$TMP_DIR/$BIN_NAME" "$BIN_DIR/$BIN_NAME"

echo "installed ${BIN_NAME} ${TAG} to ${BIN_DIR}/${BIN_NAME}"
