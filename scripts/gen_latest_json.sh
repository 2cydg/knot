#!/usr/bin/env sh
set -eu

usage() {
  echo "usage: scripts/gen_latest_json.sh VERSION DIST_DIR GITHUB_REPOSITORY [ASSET_BASE_URL]" >&2
  exit 2
}

if [ "$#" -lt 3 ] || [ "$#" -gt 4 ]; then
  usage
fi

VERSION="$1"
DIST_DIR="$2"
REPOSITORY="$3"
ASSET_BASE_URL="${4:-https://github.com/$REPOSITORY/releases/download/$VERSION}"
PUBLISHED_AT="${PUBLISHED_AT:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
OUT="$DIST_DIR/latest.json"
CHECKSUMS="$DIST_DIR/checksums.txt"

if [ ! -f "$CHECKSUMS" ]; then
  echo "checksums file not found: $CHECKSUMS" >&2
  exit 1
fi

checksum_for() {
  file="$1"
  awk -v file="$file" '$2 == file { print $1; found = 1; exit } END { if (!found) exit 1 }' "$CHECKSUMS"
}

write_asset() {
  key="$1"
  file="$2"

  if [ ! -f "$DIST_DIR/$file" ]; then
    return
  fi

  sha=$(checksum_for "$file")
  if [ "$first_asset" -eq 0 ]; then
    printf ',\n' >> "$OUT"
  fi
  first_asset=0

  {
    printf '    "%s": {\n' "$key"
    printf '      "url": "%s/%s",\n' "$ASSET_BASE_URL" "$file"
    printf '      "sha256": "%s"\n' "$sha"
    printf '    }'
  } >> "$OUT"
}

mkdir -p "$DIST_DIR"

cat > "$OUT" <<EOF
{
  "version": "$VERSION",
  "published_at": "$PUBLISHED_AT",
  "channel": "stable",
  "notes_url": "https://github.com/$REPOSITORY/releases/tag/$VERSION",
  "assets": {
EOF

first_asset=1
write_asset linux_amd64 knot-linux-amd64.tar.gz
write_asset linux_arm64 knot-linux-arm64.tar.gz
write_asset darwin_amd64 knot-darwin-amd64.tar.gz
write_asset darwin_arm64 knot-darwin-arm64.tar.gz
write_asset windows_amd64 knot-windows-amd64.zip
write_asset windows_arm64 knot-windows-arm64.zip

cat >> "$OUT" <<EOF

  }
}
EOF

echo "wrote $OUT"
