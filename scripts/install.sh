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

# Mirrors setup.DefaultStateDir: XDG_STATE_HOME/atlas-tasker, else
# macOS ~/Library/Application Support/Atlas Tasker, else
# Linux ~/.local/state/atlas-tasker.
resolve_state_dir() {
  if [ -n "${XDG_STATE_HOME:-}" ]; then
    case "$XDG_STATE_HOME" in
      /*) printf '%s/atlas-tasker' "$XDG_STATE_HOME"; return ;;
      *) echo "XDG_STATE_HOME must be an absolute path; skipping install receipt" >&2; return 1 ;;
    esac
  fi
  if [ -z "${HOME:-}" ] || [ "${HOME#/}" = "$HOME" ]; then
    echo "HOME must be an absolute path to write an install receipt" >&2
    return 1
  fi
  case "$(uname -s)" in
    Darwin) printf '%s/Library/Application Support/Atlas Tasker' "$HOME" ;;
    *) printf '%s/.local/state/atlas-tasker' "$HOME" ;;
  esac
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

write_install_receipt() {
  binary_path="$BIN_DIR/$BIN_NAME"
  if [ ! -f "$binary_path" ]; then
    echo "warning: installed binary missing; tracker uninstall will refuse without a receipt" >&2
    return 0
  fi
  state_dir="$(resolve_state_dir)" || return 0
  mkdir -p "$state_dir" || {
    echo "warning: could not write install receipt; tracker uninstall will preview only" >&2
    return 0
  }
  chmod 700 "$state_dir" 2>/dev/null || true
  sum="$(checksum_file "$binary_path")"
  created="$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo 1970-01-01T00:00:00Z)"
  digest="$( {
    printf '%s\n' "$binary_path" "$sum" "script" "$TAG"
  } | {
    if command -v shasum >/dev/null 2>&1; then
      shasum -a 256
    else
      sha256sum
    fi
  } | awk '{print $1}')"
  receipt="$state_dir/install-receipt.json"
  cat > "$receipt" <<EOF
{
  "format": "atlas_install_receipt_v1",
  "created_at": "$created",
  "version": "$(json_escape "$TAG")",
  "install_method": "script",
  "binary_path": "$(json_escape "$binary_path")",
  "binary_sha256": "$sum",
  "owned_paths": [
    {
      "path": "$(json_escape "$binary_path")",
      "kind": "executable",
      "sha256": "$sum"
    }
  ],
  "digest": "$digest"
}
EOF
  chmod 600 "$receipt" 2>/dev/null || true
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

offer_integrations() {
  if [ "${SKIP_INTEGRATIONS:-0}" = "1" ]; then
    return
  fi
  # curl | sh leaves stdin attached to the download. Use the controlling
  # terminal for optional setup; unattended installs must never read stdin
  # and must never initialize the current directory.
  if [ ! -t 1 ] || ! ( : </dev/tty ) 2>/dev/null; then
    echo "Next: run tracker setup in an Atlas project to install coding-agent guidance."
    return
  fi
  # Prefer tracker setup. Older pinned binaries without that command fall
  # back to a message instead of initializing the current directory.
  if ! "$BIN_DIR/$BIN_NAME" setup --help >/dev/null 2>&1; then
    echo "Next: run tracker init in your project, then tracker integrations install."
    return
  fi
  printf '\nSet up coding-agent guidance in %s? Requires an existing Atlas workspace. [y/N] ' "$(pwd)" >/dev/tty
  answer=""
  if ! IFS= read -r answer </dev/tty; then
    echo "Skipped setup. Run tracker setup in your project when ready."
    return
  fi
  case "$answer" in
    y|Y|yes|YES|Yes)
      if ! "$BIN_DIR/$BIN_NAME" setup </dev/tty >/dev/tty; then
        echo "Tracker is installed, but agent setup did not complete. Run tracker setup in your project to try again." >&2
      fi
      ;;
    *) echo "Skipped setup. Run tracker setup in your project when ready." ;;
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
write_install_receipt
offer_integrations
