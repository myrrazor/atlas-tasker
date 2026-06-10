#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-v1.9.0-rc1}"
RELEASE_PROOF_DIR="${RELEASE_PROOF_DIR:-$ROOT_DIR/docs/release-proof}"
RUN_GOVULNCHECK="${RUN_GOVULNCHECK:-0}"
RUN_SBOM="${RUN_SBOM:-0}"
GOVULNCHECK_VERSION="${GOVULNCHECK_VERSION:-v1.3.0}"
CYCLONEDX_GOMOD_VERSION="${CYCLONEDX_GOMOD_VERSION:-v1.10.0}"
BUILDINFO_PKG="github.com/myrrazor/atlas-tasker/internal/buildinfo"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "preflight_status=blocked"
    echo "reason_code=missing_command"
    echo "message=missing required command: $1"
    exit 1
  fi
}

fail() {
  echo "preflight_status=blocked"
  echo "reason_code=$1"
  echo "message=$2"
  exit 1
}

validate_release_version() {
  case "$VERSION" in
    ""|*[!abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._+-]*)
      fail "unsafe_version" "VERSION may only contain letters, numbers, dot, underscore, plus, and hyphen"
      ;;
  esac
}

detect_os() {
  case "$(uname -s)" in
    Darwin) printf 'darwin' ;;
    Linux) printf 'linux' ;;
    *) fail "unsupported_os" "unsupported OS for release preflight: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf 'amd64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *) fail "unsupported_arch" "unsupported architecture for release preflight: $(uname -m)" ;;
  esac
}

if [ "${1:-}" = "--hosted" ] || [ "${HOSTED_RELEASE_PROOF:-0}" = "1" ]; then
  exec sh "$ROOT_DIR/scripts/preflight-release-proof.sh"
fi

validate_release_version

need_cmd go
need_cmd git
need_cmd grep
need_cmd mktemp
need_cmd python3
need_cmd sed
need_cmd sh

for script in \
  scripts/install.sh \
  scripts/verify-release.sh \
  scripts/release-rehearsal.sh \
  scripts/preflight-release-proof.sh \
  scripts/preflight-release.sh \
  scripts/validate-rc.sh
do
  sh -n "$ROOT_DIR/$script" || fail "script_syntax_failed" "shell syntax check failed for $script"
done

OS_NAME="$(detect_os)"
ARCH_NAME="$(detect_arch)"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse --short=12 HEAD 2>/dev/null || echo unknown)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
LDFLAGS="-s -w -X ${BUILDINFO_PKG}.Version=${VERSION} -X ${BUILDINFO_PKG}.Commit=${COMMIT} -X ${BUILDINFO_PKG}.BuildDate=${BUILD_DATE}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

GOOS="$OS_NAME" GOARCH="$ARCH_NAME" CGO_ENABLED=0 go build -trimpath -ldflags="$LDFLAGS" -o "$TMP_DIR/tracker" "$ROOT_DIR/cmd/tracker"
VERSION_JSON="$("$TMP_DIR/tracker" version --json)"
VERSION_JSON_FILE="$TMP_DIR/version.json"
printf '%s' "$VERSION_JSON" > "$VERSION_JSON_FILE"
python3 - "$VERSION_JSON_FILE" "$VERSION" "$COMMIT" "$BUILD_DATE" "$OS_NAME/$ARCH_NAME" <<'PY' || fail "version_json_mismatch" "tracker version --json did not match the release metadata contract"
import json
import sys

path, want_version, want_commit, want_build_date, want_platform = sys.argv[1:6]
with open(path, encoding="utf-8") as fh:
    payload = json.load(fh)
expected_keys = {
    "format_version",
    "kind",
    "version",
    "commit",
    "build_date",
    "go_version",
    "platform",
}
if set(payload) != expected_keys:
    raise SystemExit(f"unexpected keys: {sorted(payload)}")
expected = {
    "format_version": "v1",
    "kind": "tracker_version",
    "version": want_version,
    "commit": want_commit,
    "build_date": want_build_date,
    "platform": want_platform,
}
for key, value in expected.items():
    if payload.get(key) != value:
        raise SystemExit(f"{key}={payload.get(key)!r}, want {value!r}")
if not isinstance(payload.get("go_version"), str) or not payload["go_version"].startswith("go"):
    raise SystemExit("go_version must be a Go runtime string")
PY

if [ "$RUN_GOVULNCHECK" = "1" ]; then
  mkdir -p "$RELEASE_PROOF_DIR"
  GOVULN_OUT="$RELEASE_PROOF_DIR/govulncheck-${VERSION}.txt"
  if ! {
    echo "command=go run golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION} -show verbose ./..."
    echo "module=golang.org/x/vuln@${GOVULNCHECK_VERSION}"
    echo "go=$(go version)"
    echo "generated_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo
    cd "$ROOT_DIR"
    go run "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}" -show verbose ./...
  } >"$GOVULN_OUT" 2>&1; then
    echo "govulncheck_output=$GOVULN_OUT"
    fail "govulncheck_failed" "govulncheck failed; see $GOVULN_OUT"
  fi
fi

if [ "$RUN_SBOM" = "1" ]; then
  mkdir -p "$RELEASE_PROOF_DIR"
  SBOM_OUT="$RELEASE_PROOF_DIR/sbom-${VERSION}.cdx.json"
  if ! (
    cd "$ROOT_DIR"
    GOOS="$OS_NAME" GOARCH="$ARCH_NAME" CGO_ENABLED=0 go run "github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@${CYCLONEDX_GOMOD_VERSION}" app -json -output "$SBOM_OUT" -main cmd/tracker "$ROOT_DIR"
  ); then
    echo "sbom_output=$SBOM_OUT"
    fail "sbom_generation_failed" "CycloneDX SBOM generation failed; see $SBOM_OUT"
  fi
  if [ ! -s "$SBOM_OUT" ]; then
    echo "sbom_output=$SBOM_OUT"
    fail "sbom_generation_empty" "CycloneDX SBOM output was empty"
  fi
fi

echo "preflight_status=ready"
echo "mode=local"
echo "version=$VERSION"
echo "commit=$COMMIT"
echo "build_date=$BUILD_DATE"
echo "platform=$OS_NAME/$ARCH_NAME"
echo "version_json=ok"
if [ "$RUN_GOVULNCHECK" = "1" ]; then
  echo "govulncheck_status=passed"
  echo "govulncheck_output=$GOVULN_OUT"
else
  echo "govulncheck_status=skipped"
fi
if [ "$RUN_SBOM" = "1" ]; then
  echo "sbom_status=generated"
  echo "sbom_output=$SBOM_OUT"
else
  echo "sbom_status=skipped"
fi
echo "hosted_preflight=skipped"
echo "message=run 'sh scripts/preflight-release.sh --hosted' when GitHub prerelease proof is available"
