#!/usr/bin/env sh
set -eu

usage() {
  echo "usage: scripts/sync_cloudflare_kv.sh KEY FILE" >&2
  exit 2
}

if [ "$#" -ne 2 ]; then
  usage
fi

KEY="$1"
FILE="$2"

if [ ! -f "$FILE" ]; then
  echo "file not found: $FILE" >&2
  exit 1
fi

if [ -z "${CLOUDFLARE_API_TOKEN:-}" ] ||
  [ -z "${CLOUDFLARE_ACCOUNT_ID:-}" ] ||
  [ -z "${CLOUDFLARE_KV_NAMESPACE_ID:-}" ]; then
  echo "Cloudflare KV credentials are not configured; skipping sync for $KEY"
  exit 0
fi

case "$KEY" in
  *.json) content_type="application/json; charset=utf-8" ;;
  *.sh) content_type="text/x-shellscript; charset=utf-8" ;;
  *.ps1) content_type="text/plain; charset=utf-8" ;;
  *) content_type="application/octet-stream" ;;
esac

curl -fsS -X PUT \
  "https://api.cloudflare.com/client/v4/accounts/$CLOUDFLARE_ACCOUNT_ID/storage/kv/namespaces/$CLOUDFLARE_KV_NAMESPACE_ID/values/$KEY" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -H "Content-Type: $content_type" \
  --data-binary "@$FILE" >/dev/null

echo "synced $FILE to Cloudflare KV key $KEY"
