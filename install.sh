#!/usr/bin/env bash
# install.sh — Juggernaut installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --version 2.1.2
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --latest
#
# Or after downloading:
#   bash install.sh --version 2.1.2
#   bash install.sh --latest

set -e

REPO_URL="${JUGGERNAUT_REPO_URL:-https://github.com/jpvelasco/juggernaut.git}"
INSTALL_DIR="${JUGGERNAUT_DIR:-$HOME/.juggernaut}"
VERSION=""
CONFIGURE=0
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
    --configure)
      CONFIGURE=1
      shift
      ;;
    *)
      SETUP_ARGS+=("$1")
      shift
      ;;
  esac
done

# Normalize version: accept "2.1.2" or "v2.1.2" — tags are always v-prefixed.
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

clone_install() {
  if [[ -n "$VERSION" ]]; then
    git clone --branch "$VERSION" --depth 1 --quiet "$REPO_URL" "$INSTALL_DIR"
  else
    git clone --quiet "$REPO_URL" "$INSTALL_DIR"
  fi
}

backup_existing_install() {
  local ts backup n
  ts="$(date +%Y%m%d_%H%M%S)"
  backup="${INSTALL_DIR}.backup.${ts}"
  n=1
  while [[ -e "$backup" ]]; do
    backup="${INSTALL_DIR}.backup.${ts}.${n}"
    n=$((n + 1))
  done
  echo "Backup created: $backup"
  mv "$INSTALL_DIR" "$backup"
}

install_tree_dirty() {
  if ! git -C "$INSTALL_DIR" rev-parse --git-dir >/dev/null 2>&1; then
    return 0
  fi
  if ! git -C "$INSTALL_DIR" diff --quiet --ignore-submodules --; then
    return 0
  fi
  if ! git -C "$INSTALL_DIR" diff --cached --quiet --ignore-submodules --; then
    return 0
  fi
  [[ -n "$(git -C "$INSTALL_DIR" ls-files --others --exclude-standard)" ]]
}

if [[ -d "$INSTALL_DIR" ]]; then
  if install_tree_dirty; then
    echo "Existing installation has local changes or is not a clean Git checkout."
    backup_existing_install
    clone_install
  else
    echo "Updating existing installation in $INSTALL_DIR"
    git -C "$INSTALL_DIR" fetch --tags --quiet
    if [[ -n "$VERSION" ]]; then
      git -C "$INSTALL_DIR" checkout --quiet "$VERSION"
    else
      git -C "$INSTALL_DIR" checkout --quiet main
      git -C "$INSTALL_DIR" pull --ff-only --quiet
    fi
  fi
else
  clone_install
fi

echo "Installed to $INSTALL_DIR"
chmod +x "$INSTALL_DIR/juggernaut" "$INSTALL_DIR/setup" "$INSTALL_DIR"/commands/*.sh "$INSTALL_DIR"/lib/*.sh 2>/dev/null || {
  echo "Warning: could not update executable permissions for all Juggernaut scripts" >&2
}

BIN_DIR="$HOME/.local/bin"
mkdir -p "$BIN_DIR"
if ln -sfn "$INSTALL_DIR/juggernaut" "$BIN_DIR/juggernaut"; then
  echo "Launcher linked at $BIN_DIR/juggernaut"
else
  echo "Warning: could not create $BIN_DIR/juggernaut symlink" >&2
fi

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "Note: add $BIN_DIR to PATH to run 'juggernaut' from any directory." ;;
esac

echo "Verify after install with: juggernaut doctor --v2"
echo "Configure with one of:"
echo "  juggernaut apply --v2 --auth=bedrock-api-key"
echo "  juggernaut apply --v2 --auth=iam"

if [[ "$CONFIGURE" == "1" ]]; then
  cd "$INSTALL_DIR"
  exec bash ./juggernaut apply --v2 "${SETUP_ARGS[@]+"${SETUP_ARGS[@]}"}"
elif [[ ${#SETUP_ARGS[@]} -gt 0 ]]; then
  echo "Note: install arguments after --version were ignored. Use --configure to run apply during install." >&2
fi
