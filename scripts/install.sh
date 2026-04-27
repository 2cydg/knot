#!/usr/bin/env sh
set -eu

DEFAULT_BASE_URL="https://knot.clay.li/i"

INSTALL_DIR="${KNOT_INSTALL_DIR:-$HOME/.local/bin}"
BASE_URL="${KNOT_BASE_URL:-$DEFAULT_BASE_URL}"
MANIFEST_URL="${KNOT_MANIFEST_URL:-}"
VERSION=""

usage() {
  cat <<'EOF'
usage: install.sh [--version VERSION] [--install-dir DIR] [--base-url URL]

Environment:
  KNOT_INSTALL_DIR   Override install directory.
  KNOT_BASE_URL      Override manifest base URL.
  KNOT_MANIFEST_URL  Override manifest URL directly.
EOF
}

fail() {
  echo "knot install: $*" >&2
  exit 1
}

use_color() {
  [ -t 1 ] && [ -z "${NO_COLOR:-}" ]
}

green_start() {
  if use_color; then
    printf '\033[32m'
  fi
}

color_reset() {
  if use_color; then
    printf '\033[0m'
  fi
}

print_green() {
  green_start
  printf '%s\n' "$*"
  color_reset
}

print_green_block() {
  green_start
  cat
  color_reset
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || fail "--version requires a value"
      VERSION="$2"
      shift 2
      ;;
    --install-dir)
      [ "$#" -ge 2 ] || fail "--install-dir requires a value"
      INSTALL_DIR="$2"
      shift 2
      ;;
    --base-url)
      [ "$#" -ge 2 ] || fail "--base-url requires a value"
      BASE_URL="$2"
      shift 2
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

detect_asset_key() {
  os=$(uname -s 2>/dev/null || true)
  arch=$(uname -m 2>/dev/null || true)

  case "$os" in
    Linux) os_key=linux ;;
    Darwin) os_key=darwin ;;
    *) fail "unsupported operating system: $os" ;;
  esac

  case "$arch" in
    x86_64 | amd64) arch_key=amd64 ;;
    arm64 | aarch64) arch_key=arm64 ;;
    *) fail "unsupported CPU architecture: $arch" ;;
  esac

  printf '%s_%s\n' "$os_key" "$arch_key"
}

download() {
  url="$1"
  out="$2"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
    return
  fi

  if command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url"
    return
  fi

  fail "curl or wget is required"
}

sha256_file() {
  file="$1"

  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{ print $1 }'
    return
  fi

  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{ print $1 }'
    return
  fi

  fail "sha256sum or shasum is required"
}

json_asset_field() {
  manifest="$1"
  asset="$2"
  field="$3"

  sed -n "/\"$asset\"[[:space:]]*:/,/^[[:space:]]*}/p" "$manifest" |
    sed -n "s/.*\"$field\"[[:space:]]*:[[:space:]]*\"\\([^\"]*\\)\".*/\\1/p" |
    head -n 1
}

stop_running_knot() {
  knot_cmd=$(command -v knot 2>/dev/null || true)
  if [ -z "$knot_cmd" ] && [ -x "$INSTALL_DIR/knot" ]; then
    knot_cmd="$INSTALL_DIR/knot"
  fi

  if [ -z "$knot_cmd" ]; then
    return
  fi

  if "$knot_cmd" status >/dev/null 2>&1; then
    echo "Stopping running knot daemon"
    "$knot_cmd" stop >/dev/null 2>&1 || fail "failed to stop running knot daemon"
  fi
}

detect_shell_name() {
  current_shell=""
  if command -v ps >/dev/null 2>&1; then
    current_shell=$(ps -p $$ -o comm= 2>/dev/null || true)
    current_shell=${current_shell##*/}
  fi

  case "$current_shell" in
    bash | zsh | fish) printf '%s\n' "$current_shell"; return ;;
  esac

  login_shell=${SHELL:-}
  login_shell=${login_shell##*/}
  case "$login_shell" in
    bash | zsh | fish) printf '%s\n' "$login_shell"; return ;;
  esac
}

print_completion_hint() {
  shell_name=$(detect_shell_name)

  case "$shell_name" in
    bash)
      cat <<'EOF' | print_green_block

Shell completion:
  source <(knot completion bash)
EOF
      ;;
    zsh)
      cat <<'EOF' | print_green_block

Shell completion:
  autoload -U compinit && compinit && source <(knot completion zsh)
EOF
      ;;
    fish)
      cat <<'EOF' | print_green_block

Shell completion:
  knot completion fish | source
EOF
      ;;
  esac
}

print_next_steps() {
  cat <<'EOF' | print_green_block

Common commands:
  knot add      Add a server configuration
  knot ls       List saved servers
  knot [alias]  Connect to a saved server
EOF

  print_completion_hint

  cat <<'EOF'

Run "knot --help" for the full command reference.
Enjoy.
EOF
}

BASE_URL=${BASE_URL%/}
if [ -z "$MANIFEST_URL" ]; then
  if [ -n "$VERSION" ]; then
    MANIFEST_URL="$BASE_URL/releases/$VERSION/manifest.json"
  else
    MANIFEST_URL="$BASE_URL/latest.json"
  fi
fi

ASSET_KEY=$(detect_asset_key)
TMP_DIR=$(mktemp -d 2>/dev/null || mktemp -d -t knot-install)
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

MANIFEST_FILE="$TMP_DIR/manifest.json"
ARCHIVE_FILE="$TMP_DIR/knot-package"
EXTRACT_DIR="$TMP_DIR/extract"

echo "Downloading manifest: $MANIFEST_URL"
download "$MANIFEST_URL" "$MANIFEST_FILE" || fail "failed to download manifest"

ASSET_URL=$(json_asset_field "$MANIFEST_FILE" "$ASSET_KEY" url)
EXPECTED_SHA=$(json_asset_field "$MANIFEST_FILE" "$ASSET_KEY" sha256)

[ -n "$ASSET_URL" ] || fail "manifest does not contain asset: $ASSET_KEY"
[ -n "$EXPECTED_SHA" ] || fail "manifest does not contain sha256 for: $ASSET_KEY"

echo "Downloading package for $ASSET_KEY"
download "$ASSET_URL" "$ARCHIVE_FILE" || fail "failed to download package"

ACTUAL_SHA=$(sha256_file "$ARCHIVE_FILE" | tr 'A-F' 'a-f')
EXPECTED_SHA=$(printf '%s' "$EXPECTED_SHA" | tr 'A-F' 'a-f')
if [ "$ACTUAL_SHA" != "$EXPECTED_SHA" ]; then
  fail "checksum mismatch for $ASSET_KEY"
fi

mkdir -p "$EXTRACT_DIR"
tar -xzf "$ARCHIVE_FILE" -C "$EXTRACT_DIR" || fail "failed to extract package"

[ -f "$EXTRACT_DIR/knot" ] || fail "package did not contain knot binary"

mkdir -p "$INSTALL_DIR" || fail "failed to create install directory: $INSTALL_DIR"
[ -w "$INSTALL_DIR" ] || fail "install directory is not writable: $INSTALL_DIR"

stop_running_knot

TMP_BIN="$INSTALL_DIR/.knot.tmp.$$"
cp "$EXTRACT_DIR/knot" "$TMP_BIN" || fail "failed to copy knot binary"
chmod +x "$TMP_BIN" || fail "failed to mark knot executable"
mv "$TMP_BIN" "$INSTALL_DIR/knot" || fail "failed to install knot to $INSTALL_DIR/knot"

echo "knot installed to $INSTALL_DIR/knot"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    print_green "Add $INSTALL_DIR to PATH if knot is not found by your shell."
    ;;
esac

print_next_steps
