#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-v1.9.0-rc1}"
VERSION_NO_V="${VERSION#v}"
BIN_NAME="tracker"
DIST_DIR="${DIST_DIR:-$(mktemp -d)}"
WORK_DIR="${WORK_DIR:-$(mktemp -d)}"
INSTALL_DIR="${INSTALL_DIR:-$(mktemp -d)}"
BUILDINFO_PKG="github.com/myrrazor/atlas-tasker/internal/buildinfo"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse --short=12 HEAD 2>/dev/null || echo unknown)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

validate_release_version() {
  case "$VERSION" in
    ""|*[!abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._+-]*)
      echo "unsafe release version: $VERSION" >&2
      exit 1
      ;;
  esac
}

validate_release_version

LDFLAGS="-s -w -X ${BUILDINFO_PKG}.Version=${VERSION} -X ${BUILDINFO_PKG}.Commit=${COMMIT} -X ${BUILDINFO_PKG}.BuildDate=${BUILD_DATE}"

need_cmd go
need_cmd git
need_cmd tar
need_cmd mktemp
need_cmd python3

OS_NAME="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH_NAME="$(uname -m)"

case "$OS_NAME" in
  darwin|linux) ;;
  *) echo "unsupported OS for rehearsal: $OS_NAME" >&2; exit 1 ;;
esac

case "$ARCH_NAME" in
  x86_64|amd64) ARCH_NAME="amd64" ;;
  arm64|aarch64) ARCH_NAME="arm64" ;;
  *) echo "unsupported arch for rehearsal: $ARCH_NAME" >&2; exit 1 ;;
esac

ARCHIVE="${BIN_NAME}_${VERSION_NO_V}_${OS_NAME}_${ARCH_NAME}.tar.gz"

mkdir -p "$DIST_DIR" "$WORK_DIR" "$INSTALL_DIR"

GOOS="$OS_NAME" GOARCH="$ARCH_NAME" CGO_ENABLED=0 go build -trimpath -ldflags="$LDFLAGS" -o "$DIST_DIR/$BIN_NAME" "$ROOT_DIR/cmd/tracker"
tar -czf "$DIST_DIR/$ARCHIVE" -C "$DIST_DIR" "$BIN_NAME"
if command -v shasum >/dev/null 2>&1; then
  shasum -a 256 "$DIST_DIR/$ARCHIVE" > "$DIST_DIR/checksums.txt"
else
  sha256sum "$DIST_DIR/$ARCHIVE" > "$DIST_DIR/checksums.txt"
fi

PORT_FILE="$DIST_DIR/http.port"
python3 - <<'PY' "$DIST_DIR" "$PORT_FILE" &
import http.server
import os
import socketserver
import sys

dist = sys.argv[1]
port_file = sys.argv[2]
os.chdir(dist)
with socketserver.TCPServer(("127.0.0.1", 0), http.server.SimpleHTTPRequestHandler) as httpd:
    with open(port_file, "w", encoding="utf-8") as fh:
        fh.write(str(httpd.server_address[1]))
    httpd.serve_forever()
PY
SERVER_PID=$!
trap 'kill $SERVER_PID 2>/dev/null || true' EXIT INT TERM

for _ in $(seq 1 50); do
  if [ -f "$PORT_FILE" ]; then
    break
  fi
  sleep 0.1
done

if [ ! -f "$PORT_FILE" ]; then
  echo "failed to start local rehearsal server" >&2
  exit 1
fi

PORT="$(cat "$PORT_FILE")"
VERSION="$VERSION" RELEASE_BASE_URL="http://127.0.0.1:$PORT" VERIFY_ATTESTATIONS=0 ALLOW_INSECURE_RELEASE_BASE_URL=1 sh "$ROOT_DIR/scripts/verify-release.sh" "$DIST_DIR/$ARCHIVE"
RELEASE_BASE_URL="http://127.0.0.1:$PORT" VERSION="$VERSION" BIN_DIR="$INSTALL_DIR" VERIFY_ATTESTATIONS=0 ALLOW_INSECURE_RELEASE_BASE_URL=1 sh "$ROOT_DIR/scripts/install.sh"
VERSION_JSON="$("$INSTALL_DIR/$BIN_NAME" version --json)"
case "$VERSION_JSON" in
  *"\"version\": \"$VERSION\""*) ;;
  *) echo "installed binary has unexpected version metadata: $VERSION_JSON" >&2; exit 1 ;;
esac
case "$VERSION_JSON" in
  *"\"commit\": \"$COMMIT\""*) ;;
  *) echo "installed binary has unexpected commit metadata: $VERSION_JSON" >&2; exit 1 ;;
esac
case "$VERSION_JSON" in
  *"\"platform\": \"$OS_NAME/$ARCH_NAME\""*) ;;
  *) echo "installed binary has unexpected platform metadata: $VERSION_JSON" >&2; exit 1 ;;
esac

cd "$WORK_DIR"
git init -b main >/dev/null
git config user.email atlas@example.com
git config user.name Atlas
printf '# atlas\n' > README.md
git add README.md
git commit -m init >/dev/null
"$INSTALL_DIR/$BIN_NAME" init
"$INSTALL_DIR/$BIN_NAME" project create APP "App Project"
"$INSTALL_DIR/$BIN_NAME" ticket create --project APP --title "Smoke" --type task --reviewer agent:reviewer-1 --actor human:owner
"$INSTALL_DIR/$BIN_NAME" ticket move APP-1 ready --actor human:owner
"$INSTALL_DIR/$BIN_NAME" agent create builder-1 --name "Builder One" --provider codex --capability go --actor human:owner
DISPATCH_JSON="$("$INSTALL_DIR/$BIN_NAME" run dispatch APP-1 --agent builder-1 --actor human:owner --json)"
RUN_ID="$(printf '%s' "$DISPATCH_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["payload"]["run_id"])')"
"$INSTALL_DIR/$BIN_NAME" run launch "$RUN_ID" --actor human:owner
"$INSTALL_DIR/$BIN_NAME" run start "$RUN_ID" --actor human:owner
"$INSTALL_DIR/$BIN_NAME" ticket move APP-1 in_progress --actor human:owner
"$INSTALL_DIR/$BIN_NAME" run checkpoint "$RUN_ID" --title "Smoke checkpoint" --body "runtime + worktree ready" --actor human:owner
"$INSTALL_DIR/$BIN_NAME" run evidence add "$RUN_ID" --type note --title "Smoke evidence" --body "packaged rehearsal" --actor human:owner
CHANGE_CREATE_JSON="$("$INSTALL_DIR/$BIN_NAME" change create "$RUN_ID" --actor human:owner --json)"
CHANGE_ID="$(printf '%s' "$CHANGE_CREATE_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["payload"]["change"]["change_id"])')"
"$INSTALL_DIR/$BIN_NAME" change status "$CHANGE_ID" >/dev/null
"$INSTALL_DIR/$BIN_NAME" run handoff "$RUN_ID" --next-actor agent:reviewer-1 --next-gate review --actor human:owner
APPROVALS_JSON="$("$INSTALL_DIR/$BIN_NAME" approvals --json)"
GATE_ID="$(printf '%s' "$APPROVALS_JSON" | python3 -c 'import json,sys; items=json.load(sys.stdin)["items"]; print(items[0]["gate"]["gate_id"] if items else "")')"
if [ -z "$GATE_ID" ]; then
  echo "failed to find review gate during rehearsal" >&2
  exit 1
fi
"$INSTALL_DIR/$BIN_NAME" gate approve "$GATE_ID" --actor agent:reviewer-1 --reason "packaged rehearsal"
"$INSTALL_DIR/$BIN_NAME" run complete "$RUN_ID" --actor human:owner --summary "smoke complete"
"$INSTALL_DIR/$BIN_NAME" ticket request-review APP-1 --actor agent:builder-1
"$INSTALL_DIR/$BIN_NAME" ticket approve APP-1 --actor agent:reviewer-1
"$INSTALL_DIR/$BIN_NAME" ticket complete APP-1 --actor human:owner
"$INSTALL_DIR/$BIN_NAME" export create --scope workspace --actor human:owner >/dev/null
find "$WORK_DIR/.tracker/runtime/$RUN_ID" -exec touch -t 202001010101 {} +
"$INSTALL_DIR/$BIN_NAME" archive plan --target runtime --project APP >/dev/null
"$INSTALL_DIR/$BIN_NAME" archive apply --target runtime --project APP --yes --actor human:owner >/dev/null
ARCHIVE_JSON="$("$INSTALL_DIR/$BIN_NAME" archive list --target runtime --json)"
ARCHIVE_ID="$(printf '%s' "$ARCHIVE_JSON" | python3 -c 'import json,sys; items=json.load(sys.stdin)["items"]; print(items[0]["archive_id"] if items else "")')"
if [ -z "$ARCHIVE_ID" ]; then
  echo "failed to find runtime archive during rehearsal" >&2
  exit 1
fi
"$INSTALL_DIR/$BIN_NAME" archive restore "$ARCHIVE_ID" --actor human:owner >/dev/null
"$INSTALL_DIR/$BIN_NAME" run cleanup "$RUN_ID" --actor human:owner
"$INSTALL_DIR/$BIN_NAME" inspect APP-1 --actor human:owner --json >/dev/null

TRACKER="$INSTALL_DIR/$BIN_NAME"
run_in() {
  dir="$1"
  shift
  (
    cd "$dir"
    "$TRACKER" "$@"
  )
}

GIT_REMOTE_ROOT="$(mktemp -d)"
GIT_REMOTE="$GIT_REMOTE_ROOT/sync-remote.git"
git init --bare "$GIT_REMOTE" >/dev/null
WORK_B="$(mktemp -d)"
WORK_C="$(mktemp -d)"
WORK_D="$(mktemp -d)"

run_in "$WORK_B" init
run_in "$WORK_C" init
run_in "$WORK_D" init

run_in "$WORK_DIR" collaborator add rev-1 --name "Rev One" --actor-map agent:reviewer-1 --actor human:owner >/dev/null
run_in "$WORK_DIR" collaborator trust rev-1 --actor human:owner >/dev/null
run_in "$WORK_DIR" membership bind rev-1 --scope-kind project --scope-id APP --role reviewer --actor human:owner >/dev/null
run_in "$WORK_DIR" ticket create --project APP --title "Shared sync" --type task --actor human:owner >/dev/null
run_in "$WORK_DIR" ticket comment APP-2 --body "loop in @rev-1" --actor human:owner >/dev/null
run_in "$WORK_DIR" remote add origin --kind git --location "$GIT_REMOTE" --default-action push --actor human:owner >/dev/null

SOURCE_PUSH_JSON="$(run_in "$WORK_DIR" sync push --remote origin --actor human:owner --json)"
SOURCE_WORKSPACE_ID="$(printf '%s' "$SOURCE_PUSH_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["payload"]["publication"]["workspace_id"])')"
if [ -z "$SOURCE_WORKSPACE_ID" ]; then
  echo "failed to read source workspace id from packaged sync push" >&2
  exit 1
fi

run_in "$WORK_B" remote add origin --kind git --location "$GIT_REMOTE" --default-action pull --actor human:owner >/dev/null
run_in "$WORK_B" sync pull --remote origin --workspace "$SOURCE_WORKSPACE_ID" --actor human:owner >/dev/null
MENTIONS_B="$(run_in "$WORK_B" mentions list --collaborator rev-1 --json)"
case "$MENTIONS_B" in
  *'"collaborator_id": "rev-1"'*) ;;
  *) echo "workspace B missing synced canonical mention: $MENTIONS_B" >&2; exit 1 ;;
esac

run_in "$WORK_B" ticket edit APP-2 --title "Shared sync from B" --actor human:owner >/dev/null
REMOTE_PUSH_JSON="$(run_in "$WORK_B" sync push --remote origin --actor human:owner --json)"
REMOTE_WORKSPACE_ID="$(printf '%s' "$REMOTE_PUSH_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["payload"]["publication"]["workspace_id"])')"
if [ -z "$REMOTE_WORKSPACE_ID" ]; then
  echo "failed to read workspace B id from packaged sync push" >&2
  exit 1
fi

run_in "$WORK_DIR" ticket edit APP-2 --title "Shared sync from A" --actor human:owner >/dev/null
PULL_CONFLICT_LOG="$(mktemp)"
if run_in "$WORK_DIR" sync pull --remote origin --workspace "$REMOTE_WORKSPACE_ID" --actor human:owner --json >"$PULL_CONFLICT_LOG" 2>&1; then
  echo "expected packaged sync pull conflict, got success" >&2
  cat "$PULL_CONFLICT_LOG" >&2
  exit 1
fi
CONFLICT_LIST_JSON="$(run_in "$WORK_DIR" conflict list --json)"
CONFLICT_ID="$(printf '%s' "$CONFLICT_LIST_JSON" | python3 -c 'import json,sys; items=json.load(sys.stdin)["items"]; print(next((item["conflict_id"] for item in items if item["entity_kind"] == "ticket" and item["status"] == "open"), ""))')"
if [ -z "$CONFLICT_ID" ]; then
  echo "expected open packaged ticket conflict, got: $CONFLICT_LIST_JSON" >&2
  exit 1
fi
run_in "$WORK_DIR" conflict resolve "$CONFLICT_ID" --resolution use_remote --actor human:owner >/dev/null
TICKET_AFTER_RESOLVE="$(run_in "$WORK_DIR" ticket view APP-2 --json)"
case "$TICKET_AFTER_RESOLVE" in
  *'Shared sync from B'*) ;;
  *) echo "expected remote conflict resolution to win for APP-2: $TICKET_AFTER_RESOLVE" >&2; exit 1 ;;
esac

run_in "$WORK_B" ticket create --project APP --title "Shared from B" --type task --actor human:owner >/dev/null
REMOTE_PUSH_JSON="$(run_in "$WORK_B" sync push --remote origin --actor human:owner --json)"
REMOTE_WORKSPACE_ID="$(printf '%s' "$REMOTE_PUSH_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["payload"]["publication"]["workspace_id"])')"

run_in "$WORK_C" remote add origin --kind git --location "$GIT_REMOTE" --default-action pull --actor human:owner >/dev/null
run_in "$WORK_C" sync pull --remote origin --workspace "$REMOTE_WORKSPACE_ID" --actor human:owner >/dev/null
TICKETS_C="$(run_in "$WORK_C" ticket list --project APP --json)"
case "$TICKETS_C" in
  *'"id": "APP-3"'*) ;;
  *) echo "expected workspace C to receive APP-3: $TICKETS_C" >&2; exit 1 ;;
esac
run_in "$WORK_C" ticket create --project APP --title "Shared from C" --type task --actor human:owner >/dev/null
C_PUSH_JSON="$(run_in "$WORK_C" sync push --remote origin --actor human:owner --json)"
C_WORKSPACE_ID="$(printf '%s' "$C_PUSH_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["payload"]["publication"]["workspace_id"])')"
run_in "$WORK_DIR" sync pull --remote origin --workspace "$C_WORKSPACE_ID" --actor human:owner >/dev/null
TICKETS_A="$(run_in "$WORK_DIR" ticket list --project APP --json)"
for ticket_id in APP-3 APP-4; do
  case "$TICKETS_A" in
    *"\"id\": \"$ticket_id\""*) ;;
    *) echo "expected workspace A to converge with $ticket_id: $TICKETS_A" >&2; exit 1 ;;
  esac
done

BUNDLE_CREATE_JSON="$(run_in "$WORK_DIR" bundle create --actor human:owner --json)"
BUNDLE_REF="$(printf '%s' "$BUNDLE_CREATE_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["payload"]["job"]["bundle_ref"])')"
if [ -z "$BUNDLE_REF" ]; then
  echo "failed to read bundle ref from packaged bundle create" >&2
  exit 1
fi
BUNDLE_VERIFY_JSON="$(run_in "$WORK_DIR" bundle verify "$BUNDLE_REF" --actor human:owner --json)"
case "$BUNDLE_VERIFY_JSON" in
  *'"verified": true'*) ;;
  *) echo "expected bundle verify to pass: $BUNDLE_VERIFY_JSON" >&2; exit 1 ;;
esac
run_in "$WORK_D" bundle import "$BUNDLE_REF" --actor human:owner >/dev/null
TICKETS_D="$(run_in "$WORK_D" ticket list --project APP --json)"
case "$TICKETS_D" in
  *'"id": "APP-4"'*) ;;
  *) echo "expected bundle import workspace to receive converged tickets: $TICKETS_D" >&2; exit 1 ;;
esac
MENTIONS_D="$(run_in "$WORK_D" mentions list --collaborator rev-1 --json)"
case "$MENTIONS_D" in
  *'"collaborator_id": "rev-1"'*) ;;
  *) echo "expected bundle import workspace to receive mentions: $MENTIONS_D" >&2; exit 1 ;;
esac

run_in "$WORK_DIR" ticket move APP-3 ready --actor human:owner >/dev/null
SYNCED_DISPATCH_JSON="$(run_in "$WORK_DIR" run dispatch APP-3 --agent builder-1 --actor human:owner --json)"
SYNCED_RUN_ID="$(printf '%s' "$SYNCED_DISPATCH_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["payload"]["run_id"])')"
run_in "$WORK_DIR" run launch "$SYNCED_RUN_ID" --actor human:owner >/dev/null
run_in "$WORK_DIR" run start "$SYNCED_RUN_ID" --actor human:owner >/dev/null
run_in "$WORK_DIR" ticket move APP-3 in_progress --actor human:owner >/dev/null
run_in "$WORK_DIR" run checkpoint "$SYNCED_RUN_ID" --title "Synced checkpoint" --body "runtime after sync" --actor human:owner >/dev/null
run_in "$WORK_DIR" run handoff "$SYNCED_RUN_ID" --next-actor agent:reviewer-1 --next-gate review --actor human:owner >/dev/null
SYNCED_APPROVALS_JSON="$(run_in "$WORK_DIR" approvals --json)"
SYNCED_GATE_ID="$(printf '%s' "$SYNCED_APPROVALS_JSON" | python3 -c 'import json,sys; items=json.load(sys.stdin)["items"]; print(items[0]["gate"]["gate_id"] if items else "")')"
if [ -z "$SYNCED_GATE_ID" ]; then
  echo "failed to find synced review gate during packaged rehearsal" >&2
  exit 1
fi
run_in "$WORK_DIR" gate approve "$SYNCED_GATE_ID" --actor agent:reviewer-1 --reason "synced archive rehearsal" >/dev/null
run_in "$WORK_DIR" run complete "$SYNCED_RUN_ID" --actor human:owner --summary "synced flow complete" >/dev/null
find "$WORK_DIR/.tracker/runtime/$SYNCED_RUN_ID" -exec touch -t 202001010101 {} +
run_in "$WORK_DIR" archive apply --target runtime --project APP --yes --actor human:owner >/dev/null
SYNC_ARCHIVE_JSON="$(run_in "$WORK_DIR" archive list --target runtime --json)"
SYNC_ARCHIVE_ID="$(python3 - "$SYNCED_RUN_ID" "$SYNC_ARCHIVE_JSON" <<'PY'
import json
import sys

run_id = sys.argv[1]
items = json.loads(sys.argv[2])["items"]
target = f".tracker/runtime/{run_id}"
for item in items:
    if target in item.get("source_paths", []):
        print(item["archive_id"])
        break
else:
    print("")
PY
)"
if [ -z "$SYNC_ARCHIVE_ID" ]; then
  echo "expected synced runtime archive: $SYNC_ARCHIVE_JSON" >&2
  exit 1
fi
run_in "$WORK_DIR" compact --yes --actor human:owner >/dev/null
run_in "$WORK_DIR" archive restore "$SYNC_ARCHIVE_ID" --actor human:owner >/dev/null
run_in "$WORK_DIR" reindex >/dev/null
DOCTOR_JSON="$(run_in "$WORK_DIR" doctor --repair --json)"
case "$DOCTOR_JSON" in
  *'"repair_pending": 0'*) ;;
  *) echo "expected doctor repair to settle after synced archive flow: $DOCTOR_JSON" >&2; exit 1 ;;
esac

echo "release rehearsal ok: version=$VERSION commit=$COMMIT archive=$ARCHIVE install_dir=$INSTALL_DIR work_dir=$WORK_DIR git_remote=$GIT_REMOTE"
