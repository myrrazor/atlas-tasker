#!/usr/bin/env sh
set -eu

REPO="${REPO:-myrrazor/atlas-tasker}"
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

payload_looks_like_api_wrapper() {
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

  # Older releases may publish provenance only through the attestations API.
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

need_cmd curl
need_cmd awk
need_cmd basename
need_cmd mktemp
need_cmd grep
need_cmd sed
if [ "$VERIFY_ATTESTATIONS" != "0" ]; then
  need_cmd gh
fi

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

curl_public "$CHECKSUMS_URL" -o "$TMP_DIR/checksums.txt"

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
  fetch_attestation_bundle "$ARCHIVE_PATH"
  BUNDLE="$TMP_DIR/attestation-bundle.jsonl"
  if [ ! -s "$BUNDLE" ]; then
    echo "attestation bundle is empty" >&2
    echo "provenance could not be verified; refusing to install" >&2
    exit 1
  fi
  SOURCE_REF_VALUE="${SOURCE_REF:-refs/tags/${TAG}}"
  validate_signer_workflow "$SIGNER_WORKFLOW"
  validate_source_ref "$SOURCE_REF_VALUE"
  if ! gh attestation verify "$ARCHIVE_PATH" \
      --repo "$REPO" \
      --bundle "$BUNDLE" \
      --signer-workflow "$SIGNER_WORKFLOW" \
      --source-ref "$SOURCE_REF_VALUE"
  then
    echo "attestation verification failed for ${ARCHIVE_NAME}" >&2
    echo "provenance could not be verified; refusing to install" >&2
    exit 1
  fi
fi

echo "verified ${ARCHIVE_NAME} for ${TAG}"
