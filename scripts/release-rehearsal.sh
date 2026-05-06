#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-v1.3.0-rc1}"
VERSION_NO_V="${VERSION#v}"
BIN_NAME="tracker"
DIST_DIR="${DIST_DIR:-$(mktemp -d)}"
WORK_DIR="${WORK_DIR:-$(mktemp -d)}"
INSTALL_DIR="${INSTALL_DIR:-$(mktemp -d)}"
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
RELEASE_BASE_URL="http://127.0.0.1:$PORT" VERSION="$VERSION" BIN_DIR="$INSTALL_DIR" sh "$ROOT_DIR/scripts/install.sh"

cd "$WORK_DIR"
"$INSTALL_DIR/$BIN_NAME" init
"$INSTALL_DIR/$BIN_NAME" project create APP "App Project"
"$INSTALL_DIR/$BIN_NAME" ticket create --project APP --title "Smoke" --type task --actor human:owner
"$INSTALL_DIR/$BIN_NAME" ticket move APP-1 ready --actor human:owner
"$INSTALL_DIR/$BIN_NAME" queue --actor human:owner

echo "release rehearsal ok: version=$VERSION archive=$ARCHIVE install_dir=$INSTALL_DIR work_dir=$WORK_DIR"
