#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-v1.5.0-rc1}"
VERSION_NO_V="${VERSION#v}"
BIN_NAME="tracker"
DIST_DIR="${DIST_DIR:-$(mktemp -d)}"
WORK_DIR="${WORK_DIR:-$(mktemp -d)}"
INSTALL_DIR="${INSTALL_DIR:-$(mktemp -d)}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

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

GOOS="$OS_NAME" GOARCH="$ARCH_NAME" CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$DIST_DIR/$BIN_NAME" "$ROOT_DIR/cmd/tracker"
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
VERSION="$VERSION" RELEASE_BASE_URL="http://127.0.0.1:$PORT" VERIFY_ATTESTATIONS=0 sh "$ROOT_DIR/scripts/verify-release.sh" "$DIST_DIR/$ARCHIVE"
RELEASE_BASE_URL="http://127.0.0.1:$PORT" VERSION="$VERSION" BIN_DIR="$INSTALL_DIR" sh "$ROOT_DIR/scripts/install.sh"

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
"$INSTALL_DIR/$BIN_NAME" change create "$RUN_ID" --actor human:owner --json > "$WORK_DIR/change-create.json"
CHANGE_ID="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["payload"]["change"]["change_id"])' < "$WORK_DIR/change-create.json")"
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

echo "release rehearsal ok: version=$VERSION archive=$ARCHIVE install_dir=$INSTALL_DIR work_dir=$WORK_DIR"
