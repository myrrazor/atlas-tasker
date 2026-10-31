#!/bin/sh
# Write <dist>/checksums.txt for the archives and SBOM that the release
# workflow uploads. Each line is "<sha256>  <bare filename>" so
# `sha256sum -c checksums.txt` succeeds in the directory that holds the
# downloaded assets. A leading "dist/" is not part of the asset name.
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: scripts/write-release-checksums.sh <dist-dir>" >&2
  exit 2
fi

DIST_DIR=$1
if [ ! -d "$DIST_DIR" ]; then
  echo "dist directory not found: $DIST_DIR" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  HASH_CMD=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  HASH_CMD=shasum
else
  echo "missing required command: sha256sum or shasum" >&2
  exit 1
fi

file_sha256() {
  if [ "$HASH_CMD" = "sha256sum" ]; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT INT TERM

found=0
for file in "$DIST_DIR"/*.tar.gz "$DIST_DIR"/sbom-*.cdx.json; do
  if [ ! -f "$file" ]; then
    continue
  fi
  found=1
  name=$(basename "$file")
  sum=$(file_sha256 "$file")
  printf '%s  %s\n' "$sum" "$name" >> "$tmp"
done

if [ "$found" -eq 0 ]; then
  echo "no release assets to checksum in $DIST_DIR" >&2
  exit 1
fi

sort -k 2,2 "$tmp" > "$DIST_DIR/checksums.txt"

(
  cd "$DIST_DIR"
  if [ "$HASH_CMD" = "sha256sum" ]; then
    sha256sum -c checksums.txt
  else
    shasum -a 256 -c checksums.txt
  fi
)
