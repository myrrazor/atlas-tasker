#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VERSION="${VERSION:-v1.8.0-rc1}"
WORK_PARENT="${VALIDATE_RC_WORK_DIR:-}"
KEEP_WORK="${VALIDATE_RC_KEEP_WORK:-0}"
PYTHON="${PYTHON:-python3}"

fail() {
  echo "rc_validation=failed reason=$1" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing_$1"
}

need_cmd go
need_cmd git
need_cmd "$PYTHON"

if [ -z "$WORK_PARENT" ]; then
  WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/atlas-rc-validation.XXXXXX")
else
  mkdir -p "$WORK_PARENT"
  WORK_DIR=$(mktemp -d "$WORK_PARENT/atlas-rc-validation.XXXXXX")
fi

cleanup() {
  if [ "$KEEP_WORK" != "1" ]; then
    rm -rf "$WORK_DIR"
  else
    echo "rc_validation_work_dir=$WORK_DIR"
  fi
}
trap cleanup EXIT INT HUP TERM

BIN="$WORK_DIR/tracker"
COMMIT=$(git -C "$ROOT_DIR" rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

go build -trimpath \
  -ldflags "-X github.com/myrrazor/atlas-tasker/internal/buildinfo.Version=$VERSION -X github.com/myrrazor/atlas-tasker/internal/buildinfo.Commit=$COMMIT -X github.com/myrrazor/atlas-tasker/internal/buildinfo.BuildDate=$BUILD_DATE" \
  -o "$BIN" "$ROOT_DIR/cmd/tracker"

"$PYTHON" "$ROOT_DIR/scripts/validate_rc.py" \
  --repo "$ROOT_DIR" \
  --tracker "$BIN" \
  --work-dir "$WORK_DIR" \
  --version "$VERSION"

echo "rc_validation=ok version=$VERSION"
