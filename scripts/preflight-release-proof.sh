#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
WORKFLOW_FILE="$ROOT_DIR/.github/workflows/release.yml"
REPO="${REPO:-}"

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

need_cmd gh
need_cmd git
need_cmd grep
need_cmd sed
need_cmd python3

if [ ! -f "$WORKFLOW_FILE" ]; then
  fail "release_workflow_missing" "release workflow file not found at $WORKFLOW_FILE"
fi

if ! gh auth status >/dev/null 2>&1; then
  fail "gh_auth_missing" "gh auth status failed; authenticate before attempting hosted prerelease proof"
fi

ACTOR="$(gh api user --jq .login 2>/dev/null || true)"
if [ -z "$REPO" ]; then
  REPO="$(gh repo view --json owner,name --jq '.owner.login + "/" + .name' 2>/dev/null || true)"
fi
if [ -z "$REPO" ]; then
  fail "repo_unresolved" "could not resolve the GitHub repository for hosted prerelease proof"
fi

REPO_JSON="$(gh repo view "$REPO" --json visibility,isPrivate,url,defaultBranchRef,name,owner 2>/dev/null || true)"
if [ -z "$REPO_JSON" ]; then
  fail "repo_metadata_unavailable" "gh repo view failed for $REPO"
fi

VISIBILITY="$(printf '%s' "$REPO_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["visibility"])')"
REPO_URL="$(printf '%s' "$REPO_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["url"])')"
DEFAULT_BRANCH="$(printf '%s' "$REPO_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["defaultBranchRef"]["name"])')"

if [ "$VISIBILITY" != "PUBLIC" ]; then
  fail "repo_visibility_unsupported" "hosted attestation proof requires a public repo here; got visibility=$VISIBILITY"
fi

for snippet in "contents: write" "attestations: write" "id-token: write" "softprops/action-gh-release@3bb12739c298aeb8a4eeaf626c5b8d85266b0e65" "actions/attest-build-provenance@v2" "sbom-"; do
  if ! grep -q "$snippet" "$WORKFLOW_FILE"; then
    fail "release_workflow_incomplete" "release workflow is missing required stanza: $snippet"
  fi
done

WORKFLOW_PERMS_RAW="$(gh api "repos/$REPO/actions/permissions/workflow" 2>&1 || true)"
case "$WORKFLOW_PERMS_RAW" in
  *'"default_workflow_permissions"'*)
    WORKFLOW_PERMS="$(printf '%s' "$WORKFLOW_PERMS_RAW" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("default_workflow_permissions","unknown"))')"
    ;;
  *)
    fail "workflow_permissions_unreadable" "GitHub Actions workflow permissions API is not readable with the current token; raw=$WORKFLOW_PERMS_RAW"
    ;;
esac

echo "preflight_status=ready"
echo "repo=$REPO"
echo "repo_url=$REPO_URL"
echo "visibility=$VISIBILITY"
echo "default_branch=$DEFAULT_BRANCH"
echo "actor=$ACTOR"
echo "workflow_default_permissions=$WORKFLOW_PERMS"
