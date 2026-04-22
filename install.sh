#!/usr/bin/env bash
# install.sh — Juggernaut installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --version 2.0.0
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --latest
#
# Or after downloading:
#   bash install.sh --version 2.0.0
#   bash install.sh --latest

set -e

REPO_URL="https://github.com/jpvelasco/juggernaut.git"
INSTALL_DIR="${JUGGERNAUT_DIR:-$HOME/.juggernaut}"
VERSION=""
SETUP_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      if [[ -z "${2:-}" ]]; then
        echo "Error: --version requires a value" >&2
        exit 1
      fi
      VERSION="$2"
      shift 2
      ;;
    --version=*)
      VERSION="${1#--version=}"
      shift
      ;;
    --latest)
      VERSION=""
      shift
      ;;
    *)
      SETUP_ARGS+=("$1")
      shift
      ;;
  esac
done

# Normalize version: accept "2.0.0" or "v2.0.0" — tags are always v-prefixed.
if [[ -n "$VERSION" && "$VERSION" != v* ]]; then
  VERSION="v${VERSION}"
fi

if ! command -v git >/dev/null 2>&1; then
  echo "Error: git is required but not installed" >&2
  exit 1
fi

if [[ -n "$VERSION" ]]; then
  echo "Installing Juggernaut $VERSION..."
else
  echo "Installing Juggernaut (latest)..."
fi

if [[ -d "$INSTALL_DIR" ]]; then
  echo "Updating existing installation in $INSTALL_DIR"
  git -C "$INSTALL_DIR" fetch --tags --quiet
  if [[ -n "$VERSION" ]]; then
    git -C "$INSTALL_DIR" checkout --quiet "$VERSION"
  else
    git -C "$INSTALL_DIR" checkout --quiet main
    git -C "$INSTALL_DIR" pull --ff-only --quiet
  fi
else
  if [[ -n "$VERSION" ]]; then
    git clone --branch "$VERSION" --depth 1 --quiet "$REPO_URL" "$INSTALL_DIR"
  else
    git clone --quiet "$REPO_URL" "$INSTALL_DIR"
  fi
fi

echo "Installed to $INSTALL_DIR"
cd "$INSTALL_DIR"
exec bash ./setup "${SETUP_ARGS[@]+"${SETUP_ARGS[@]}"}"
