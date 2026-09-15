#!/usr/bin/env sh
set -eu

# Places a published tracker binary after checksum + local attestation
# verification. Default provenance uses a public Sigstore bundle and
# `gh attestation verify --bundle`, which does not need GitHub login.
# gh remains a required prerequisite for that check.

REPO="myrrazor/atlas-tasker"
BIN_NAME="tracker"
BIN_DIR="${BIN_DIR:-/usr/local/bin}"
VERSION="${VERSION:-latest}"
RELEASE_BASE_URL="${RELEASE_BASE_URL:-}"
VERIFY_ATTESTATIONS="${VERIFY_ATTESTATIONS:-1}"
ALLOW_INSECURE_RELEASE_BASE_URL="${ALLOW_INSECURE_RELEASE_BASE_URL:-0}"
ATTESTATION_BUNDLE_URL="${ATTESTATION_BUNDLE_URL:-}"
ATTESTATION_API_BASE_URL="${ATTESTATION_API_BASE_URL:-https://api.github.com}"
SIGNER_WORKFLOW="${SIGNER_WORKFLOW:-${REPO}/.github/workflows/release.yml}"
SOURCE_REF="${SOURCE_REF:-}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    if [ "$1" = "gh" ]; then
      echo "GitHub CLI is required to verify release attestations locally (no GitHub login)." >&2
      echo "Install it from https://cli.github.com/ then re-run." >&2
      echo "VERIFY_ATTESTATIONS=0 skips provenance and is only for local unattested fixtures." >&2
    fi
    exit 1
  fi
}

need_sha256() {
  if command -v shasum >/dev/null 2>&1; then
    return
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    return
  fi
  echo "missing required command: shasum or sha256sum" >&2
  exit 1
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

validate_https_or_loopback() {
  url="$1"
  what="$2"
  case "$url" in
    https://*) return ;;
    http://127.0.0.1:*|http://localhost:*|http://\[::1\]:*)
      if [ "$ALLOW_INSECURE_RELEASE_BASE_URL" = "1" ]; then
        return
      fi
      ;;
  esac
  echo "$what must use https://; loopback http requires ALLOW_INSECURE_RELEASE_BASE_URL=1" >&2
  exit 1
}

validate_release_base_url() {
  validate_https_or_loopback "$1" "RELEASE_BASE_URL"
}

validate_api_base_url() {
  base="${1%/}"
  case "$base" in
    https://api.github.com) return ;;
    http://127.0.0.1:*|http://localhost:*|http://\[::1\]:*)
      if [ "$ALLOW_INSECURE_RELEASE_BASE_URL" = "1" ]; then
        return
      fi
      ;;
  esac
  echo "ATTESTATION_API_BASE_URL must be https://api.github.com; loopback http requires ALLOW_INSECURE_RELEASE_BASE_URL=1" >&2
  exit 1
}

validate_signer_workflow() {
  case "$1" in
    *..*)
      echo "unsafe SIGNER_WORKFLOW: $1" >&2
      exit 1
      ;;
    */.github/workflows/*.yml|*/.github/workflows/*.yaml) return ;;
  esac
  echo "SIGNER_WORKFLOW must name a .github/workflows/*.yml file" >&2
  exit 1
}

validate_source_ref() {
  case "$1" in
    refs/tags/*) ;;
    *)
      echo "unsafe SOURCE_REF: $1" >&2
      exit 1
      ;;
  esac
  validate_version "${1#refs/tags/}"
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

check_install_destination() {
  case "$BIN_DIR" in
    /*) ;;
    *)
      echo "BIN_DIR must be an absolute path: ${BIN_DIR}" >&2
      echo "retry with BIN_DIR=\"\$HOME/.local/bin\" or another writable directory" >&2
      exit 1
      ;;
  esac
  if [ -e "$BIN_DIR" ] && [ ! -d "$BIN_DIR" ]; then
    echo "BIN_DIR exists and is not a directory: ${BIN_DIR}" >&2
    exit 1
  fi
  if [ -e "$BIN_DIR/$BIN_NAME" ] && [ ! -w "$BIN_DIR/$BIN_NAME" ]; then
    echo "cannot replace ${BIN_DIR}/${BIN_NAME} (not writable)" >&2
    echo "retry with BIN_DIR=\"\$HOME/.local/bin\" or another writable directory" >&2
    exit 1
  fi
  if [ -d "$BIN_DIR" ]; then
    if [ ! -w "$BIN_DIR" ]; then
      echo "install directory is not writable: ${BIN_DIR}" >&2
      echo "retry with BIN_DIR=\"\$HOME/.local/bin\" or another writable directory" >&2
      exit 1
    fi
    return
  fi
  parent="$BIN_DIR"
  while [ ! -d "$parent" ]; do
    next="$(dirname "$parent")"
    if [ "$next" = "$parent" ]; then
      break
    fi
    parent="$next"
  done
  if [ ! -d "$parent" ] || [ ! -w "$parent" ]; then
    echo "cannot create install directory ${BIN_DIR} (not writable: ${parent})" >&2
    echo "retry with BIN_DIR=\"\$HOME/.local/bin\" or another writable directory" >&2
    exit 1
  fi
}

warn_if_bin_dir_not_on_path() {
  case ":${PATH}:" in
    *":${BIN_DIR}:"*) return ;;
  esac
  echo "warning: ${BIN_DIR} is not on PATH. Add it for this session:" >&2
  echo "  export PATH=\"${BIN_DIR}:\$PATH\"" >&2
  echo "This installer does not edit shell profiles." >&2
}

payload_looks_like_api_wrapper() {
  # GitHub's attestations API is {"attestations":[{"bundle":...}]}.
  # A Sigstore bundle jsonl starts with mediaType / verificationMaterial.
  sed -n '1,40p' "$1" | grep -q '"attestations"'
}

normalize_attestation_payload() {
  src="$1"
  dest="$2"
  if [ ! -s "$src" ]; then
    echo "attestation bundle is empty" >&2
    exit 1
  fi
  if ! payload_looks_like_api_wrapper "$src"; then
    cp "$src" "$dest"
    return
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 is required to unwrap GitHub attestation API JSON" >&2
    echo "provide attestation-bundle.jsonl as a release asset, or set ATTESTATION_BUNDLE_URL" >&2
    exit 1
  fi
  python3 - "$src" "$dest" <<'PY'
import json
import sys

src, dest = sys.argv[1], sys.argv[2]
raw = open(src, encoding="utf-8").read()
if not raw.strip():
    raise SystemExit("attestation bundle is empty")

def write_bundles(bundles):
    if not bundles:
        raise SystemExit("attestation API returned no bundles")
    with open(dest, "w", encoding="utf-8") as out:
        for bundle in bundles:
            if not isinstance(bundle, dict) or not bundle:
                raise SystemExit("attestation bundle entry is empty")
            json.dump(bundle, out, separators=(",", ":"))
            out.write("\n")

try:
    data = json.loads(raw)
except json.JSONDecodeError:
    bundles = []
    for line in raw.splitlines():
        line = line.strip()
        if not line:
            continue
        bundles.append(json.loads(line))
    write_bundles(bundles)
    raise SystemExit(0)

if isinstance(data, dict) and isinstance(data.get("attestations"), list):
    bundles = []
    for item in data["attestations"]:
        if not isinstance(item, dict) or "bundle" not in item:
            raise SystemExit("attestation API response is missing bundle")
        bundles.append(item["bundle"])
    write_bundles(bundles)
elif isinstance(data, dict):
    write_bundles([data])
elif isinstance(data, list):
    write_bundles(data)
else:
    raise SystemExit("unrecognized attestation payload")
PY
}

curl_public() {
  curl -fsSL -H "User-Agent: atlas-tasker-installer" "$@"
}

fetch_attestation_bundle() {
  archive_path="$1"
  dest="$TMP_DIR/attestation-bundle.jsonl"
  raw="$TMP_DIR/attestation-raw"

  if [ -n "$ATTESTATION_BUNDLE_URL" ]; then
    validate_https_or_loopback "$ATTESTATION_BUNDLE_URL" "ATTESTATION_BUNDLE_URL"
    if ! curl_public "$ATTESTATION_BUNDLE_URL" -o "$raw" || [ ! -s "$raw" ]; then
      echo "failed to fetch attestation bundle from ATTESTATION_BUNDLE_URL" >&2
      echo "provenance could not be verified; refusing to install" >&2
      exit 1
    fi
    normalize_attestation_payload "$raw" "$dest"
    return
  fi

  if [ -n "$RELEASE_BASE_URL" ]; then
    bundle_url="${RELEASE_BASE_URL%/}/attestation-bundle.jsonl"
  else
    bundle_url="https://github.com/${REPO}/releases/download/${TAG}/attestation-bundle.jsonl"
  fi

  # Older releases have no bundle asset; the anonymous API fallback below
  # handles that expected case without printing a misleading curl error.
  if curl_public "$bundle_url" -o "$raw" 2>/dev/null && [ -s "$raw" ]; then
    normalize_attestation_payload "$raw" "$dest"
    return
  fi
  rm -f "$raw"

  api_base="${ATTESTATION_API_BASE_URL%/}"
  # Local fixtures must ship a bundle unless the test/override points the
  # attestations API at loopback. Never silently hit api.github.com from a
  # RELEASE_BASE_URL rehearsal.
  if [ -n "$RELEASE_BASE_URL" ] && [ "$api_base" = "https://api.github.com" ]; then
    echo "attestation bundle missing at ${bundle_url}" >&2
    echo "local release fixtures must serve attestation-bundle.jsonl when VERIFY_ATTESTATIONS=1" >&2
    echo "provenance could not be verified; refusing to install" >&2
    exit 1
  fi

  validate_api_base_url "$ATTESTATION_API_BASE_URL"
  digest="$(checksum_file "$archive_path")"
  api_url="${ATTESTATION_API_BASE_URL%/}/repos/${REPO}/attestations/sha256:${digest}"
  if ! curl_public -H "Accept: application/vnd.github+json" "$api_url" -o "$raw" || [ ! -s "$raw" ]; then
    echo "failed to fetch attestation bundle from release assets or GitHub API" >&2
    echo "provenance could not be verified; refusing to install" >&2
    exit 1
  fi
  normalize_attestation_payload "$raw" "$dest"
}

verify_attestations() {
  archive_path="$1"
  archive_name="$2"
  fetch_attestation_bundle "$archive_path"
  bundle="$TMP_DIR/attestation-bundle.jsonl"
  if [ ! -s "$bundle" ]; then
    echo "attestation bundle is empty" >&2
    echo "provenance could not be verified; refusing to install" >&2
    exit 1
  fi
  signer="$SIGNER_WORKFLOW"
  source_ref="${SOURCE_REF:-refs/tags/${TAG}}"
  validate_signer_workflow "$signer"
  validate_source_ref "$source_ref"
  if ! gh attestation verify "$archive_path" \
      --repo "$REPO" \
      --bundle "$bundle" \
      --signer-workflow "$signer" \
      --source-ref "$source_ref"
  then
    echo "attestation verification failed for ${archive_name}" >&2
    echo "provenance could not be verified; refusing to install" >&2
    exit 1
  fi
}

verify_archive() {
  archive_path="$1"
  archive_name="$2"
  if [ -n "$RELEASE_BASE_URL" ]; then
    checksums_url="${RELEASE_BASE_URL%/}/checksums.txt"
  else
    checksums_url="https://github.com/${REPO}/releases/download/${TAG}/checksums.txt"
  fi
  curl_public "$checksums_url" -o "$TMP_DIR/checksums.txt"
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
    verify_attestations "$archive_path" "$archive_name"
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

print_next_init() {
  echo "Next: run tracker init in your project, then restart detected coding agents."
}

offer_init() {
  if [ "${SKIP_INTEGRATIONS:-0}" = "1" ]; then
    print_next_init
    return
  fi
  # curl | sh leaves stdin attached to the download. Use the controlling
  # terminal for optional init; unattended installs must never read stdin
  # and must never initialize the current directory.
  if [ ! -t 1 ] || ! ( : </dev/tty ) 2>/dev/null; then
    print_next_init
    return
  fi
  if [ -f "$(pwd)/.tracker/workspace.json" ]; then
    echo "This directory is already an Atlas workspace."
    echo "Next: run tracker, then restart detected coding agents."
    return
  fi
  if ! "$BIN_DIR/$BIN_NAME" init --help >/dev/null 2>&1; then
    print_next_init
    return
  fi
  printf '\nInitialize an Atlas workspace in %s? [y/N] ' "$(pwd)" >/dev/tty
  answer=""
  if ! IFS= read -r answer </dev/tty; then
    echo "Skipped init. Run tracker init in your project when ready."
    return
  fi
  case "$answer" in
    y|Y|yes|YES|Yes)
      # --no-open: installer already has the TTY; don't pop a browser.
      if ! "$BIN_DIR/$BIN_NAME" init --no-open </dev/tty >/dev/tty; then
        echo "Tracker is installed, but workspace init did not complete. Run tracker init in your project to try again." >&2
      fi
      ;;
    *) echo "Skipped init. Run tracker init in your project when ready." ;;
  esac
}

need_cmd curl
need_cmd awk
need_cmd tar
need_cmd mktemp
need_cmd install
need_cmd dirname
need_cmd grep
need_cmd sed
need_sha256
if [ "$VERIFY_ATTESTATIONS" != "0" ]; then
  need_cmd gh
fi

OS_NAME="$(detect_os)"
ARCH_NAME="$(detect_arch)"
check_install_destination

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

curl_public "$URL" -o "$TMP_DIR/$ARCHIVE"
verify_archive "$TMP_DIR/$ARCHIVE" "$ARCHIVE"
tar -xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR"
if [ ! -f "$TMP_DIR/$BIN_NAME" ]; then
  echo "archive is missing ${BIN_NAME}" >&2
  exit 1
fi
install -d "$BIN_DIR"
install "$TMP_DIR/$BIN_NAME" "$BIN_DIR/$BIN_NAME"

echo "installed ${BIN_NAME} ${TAG} to ${BIN_DIR}/${BIN_NAME}"
write_install_receipt
warn_if_bin_dir_not_on_path
offer_init
