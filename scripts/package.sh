#!/usr/bin/env sh
set -eu

usage() {
  echo "usage: scripts/package.sh VERSION [DIST_DIR]" >&2
  exit 2
}

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  usage
fi

VERSION="$1"
DIST_DIR="${2:-dist}"
SCRIPT_DIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd "$SCRIPT_DIR/.." && pwd)
GO_CMD="${GO:-go}"

case "$VERSION" in
  v[0-9]*.[0-9]*.[0-9]* | v[0-9]*.[0-9]*.[0-9]*-*) ;;
  *)
    echo "version must look like vMAJOR.MINOR.PATCH, got: $VERSION" >&2
    exit 2
    ;;
esac

if ! command -v "$GO_CMD" >/dev/null 2>&1; then
  echo "go command not found: $GO_CMD" >&2
  exit 1
fi

if ! command -v shasum >/dev/null 2>&1; then
  echo "shasum is required to generate checksums" >&2
  exit 1
fi

COMMIT="${GITHUB_SHA:-}"
if [ -z "$COMMIT" ]; then
  COMMIT=$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || echo unknown)
fi

BUILD_DATE="${BUILD_DATE:-}"
if [ -z "$BUILD_DATE" ]; then
  BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
fi

rm -rf "$ROOT_DIR/$DIST_DIR"
mkdir -p "$ROOT_DIR/$DIST_DIR"

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT INT TERM

build_archive() {
  goos="$1"
  goarch="$2"
  archive="$3"
  binary="$4"

  package_dir="$WORK_DIR/knot-$goos-$goarch"
  rm -rf "$package_dir"
  mkdir -p "$package_dir"

  echo "building $goos/$goarch"
  (
    cd "$ROOT_DIR"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" "$GO_CMD" build -trimpath \
      -ldflags "-s -w -X knot/cmd/knot/commands.version=$VERSION -X knot/cmd/knot/commands.commit=$COMMIT -X knot/cmd/knot/commands.date=$BUILD_DATE" \
      -o "$package_dir/$binary" ./cmd/knot
  )

  cp "$ROOT_DIR/README.md" "$package_dir/README.md"
  cp "$ROOT_DIR/LICENSE" "$package_dir/LICENSE"

  case "$archive" in
    *.tar.gz)
      tar -C "$package_dir" -czf "$ROOT_DIR/$DIST_DIR/$archive" "$binary" README.md LICENSE
      ;;
    *.zip)
      if command -v zip >/dev/null 2>&1; then
        (
          cd "$package_dir"
          zip -qr "$ROOT_DIR/$DIST_DIR/$archive" "$binary" README.md LICENSE
        )
      elif command -v python3 >/dev/null 2>&1; then
        python3 -c '
import os
import sys
import zipfile

output = sys.argv[1]
base_dir = sys.argv[2]
files = sys.argv[3:]

with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED) as archive:
    for name in files:
        archive.write(os.path.join(base_dir, name), arcname=name)
' "$ROOT_DIR/$DIST_DIR/$archive" "$package_dir" "$binary" README.md LICENSE
      else
        echo "zip or python3 is required to package Windows assets" >&2
        exit 1
      fi
      ;;
    *)
      echo "unsupported archive type: $archive" >&2
      exit 1
      ;;
  esac
}

build_archive linux amd64 knot-linux-amd64.tar.gz knot
build_archive linux arm64 knot-linux-arm64.tar.gz knot
build_archive darwin amd64 knot-darwin-amd64.tar.gz knot
build_archive darwin arm64 knot-darwin-arm64.tar.gz knot
build_archive windows amd64 knot-windows-amd64.zip knot.exe
build_archive windows arm64 knot-windows-arm64.zip knot.exe

(
  cd "$ROOT_DIR/$DIST_DIR"
  for asset in knot-*; do
    shasum -a 256 "$asset"
  done > checksums.txt
)

echo "wrote $DIST_DIR"
