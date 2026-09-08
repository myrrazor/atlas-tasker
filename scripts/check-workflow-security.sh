#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
EXPECTED_GO="$(awk '$1 == "go" { print $2; exit }' "$ROOT_DIR/go.mod")"

for workflow in "$ROOT_DIR"/.github/workflows/*.yml "$ROOT_DIR"/.github/workflows/*.yaml; do
  [ -f "$workflow" ] || continue
  if grep -E '^[[:space:]]*(-[[:space:]]+)?uses:' "$workflow" | grep -Ev '@[0-9a-f]{40}([[:space:]]*#.*)?$' >/dev/null; then
    echo "workflow action is not pinned to a commit: $workflow" >&2
    exit 1
  fi

  if ! grep -q '^permissions:$' "$workflow" || ! grep -q '^  contents: read$' "$workflow"; then
    echo "workflow is missing a read-only top-level permission: $workflow" >&2
    exit 1
  fi

  if grep 'go-version-file:' "$workflow" | grep -Ev 'go-version-file: *go.mod([[:space:]]*#.*)?$' >/dev/null; then
    echo "workflow Go version file must be go.mod: $workflow" >&2
    exit 1
  fi

  if grep 'go-version:' "$workflow" | grep -Fv "'$EXPECTED_GO'" >/dev/null; then
    echo "workflow Go version does not match go.mod ($EXPECTED_GO): $workflow" >&2
    exit 1
  fi
done

echo "workflow security policy passed"
