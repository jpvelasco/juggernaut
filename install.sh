#!/usr/bin/env bash
# install.sh — Juggernaut installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --version 2.1.2
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --ref fix-branch
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --latest
#
# Or after downloading:
#   bash install.sh --version 2.1.2
#   bash install.sh --ref fix-branch
#   bash install.sh --latest

set -e

REPO_URL="${JUGGERNAUT_REPO_URL:-https://github.com/jpvelasco/juggernaut.git}"
INSTALL_DIR="${JUGGERNAUT_DIR:-$HOME/.juggernaut}"
VERSION=""
REF="${JUGGERNAUT_REF:-}"
CONFIGURE=0
YES=0
LEGACY_V1=0
KEEP_ALL_BACKUPS=0
SETUP_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      if [[ -z "${2:-}" ]]; then
        echo "Error: --version requires a value" >&2
        exit 1
      fi
      VERSION="$2"
      REF=""
      shift 2
      ;;
    --version=*)
      VERSION="${1#--version=}"
      REF=""
      shift
      ;;
    --ref)
      if [[ -z "${2:-}" ]]; then
        echo "Error: --ref requires a branch, tag, or commit" >&2
        exit 1
      fi
      REF="$2"
      VERSION=""
      shift 2
      ;;
    --ref=*)
      REF="${1#--ref=}"
      VERSION=""
      shift
      ;;
    --latest)
      VERSION=""
      REF=""
      shift
      ;;
    --configure)
      CONFIGURE=1
      shift
      ;;
    --yes|-y)
      YES=1
      shift
      ;;
    --legacy-v1)
      LEGACY_V1=1
      shift
      ;;
    --keep-all-backups)
      KEEP_ALL_BACKUPS=1
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

if [[ -n "$REF" ]]; then
  echo "Installing Juggernaut $REF..."
elif [[ -n "$VERSION" ]]; then
  echo "Installing Juggernaut $VERSION..."
else
  echo "Installing Juggernaut (latest)..."
fi

clone_install() {
  local target="${1:-$INSTALL_DIR}"
  if [[ -n "$REF" ]]; then
    git clone --branch "$REF" --depth 1 --quiet "$REPO_URL" "$target"
  elif [[ -n "$VERSION" ]]; then
    git clone --branch "$VERSION" --depth 1 --quiet "$REPO_URL" "$target"
  else
    git clone --quiet "$REPO_URL" "$target"
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

  # Rotate: keep only the 5 most recent backups unless --keep-all-backups was passed.
  if [[ "$KEEP_ALL_BACKUPS" != "1" ]] && [[ -n "$INSTALL_DIR" ]]; then
    local -a old_backups
    mapfile -t old_backups < <(
      find "$(dirname "$INSTALL_DIR")" -maxdepth 1 \
        -name "$(basename "$INSTALL_DIR").backup.*" -type d -print0 \
        | xargs -0 ls -1dt 2>/dev/null \
        | tail -n +6
    )
    for old in "${old_backups[@]+"${old_backups[@]}"}"; do
      rm -rf -- "$old"
    done
  fi
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
    # Clone to a sibling directory first so a failed clone cannot destroy the
    # existing install. Only if the clone succeeds do we swap directories.
    NEW_DIR="${INSTALL_DIR}.new"
    rm -rf "$NEW_DIR"
    trap 'rm -rf "$NEW_DIR"' EXIT
    clone_install "$NEW_DIR"
    backup_existing_install
    mv "$NEW_DIR" "$INSTALL_DIR"
    trap - EXIT
  else
    echo "Updating existing installation in $INSTALL_DIR"
    git -C "$INSTALL_DIR" fetch --tags --quiet
    if [[ -n "$REF" ]]; then
      git -C "$INSTALL_DIR" fetch --quiet origin "$REF"
      git -C "$INSTALL_DIR" checkout --quiet FETCH_HEAD
    elif [[ -n "$VERSION" ]]; then
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

# ---------------------------------------------------------------------------
# Upgrade banner — show version diff and handle v1→v2 migration prompt.
# ---------------------------------------------------------------------------
RELEASE_VERSION="$(tr -d '\r\n ' < "$INSTALL_DIR/VERSION" 2>/dev/null || true)"

if [[ -f "$INSTALL_DIR/lib/upgrade_banner.sh" && -f "$INSTALL_DIR/lib/profile_paths.sh" && -f "$INSTALL_DIR/lib/migrator.sh" && -f "$INSTALL_DIR/lib/config_manager.sh" ]]; then
  . "$INSTALL_DIR/lib/profile_paths.sh"
  . "$INSTALL_DIR/lib/config_manager.sh"
  . "$INSTALL_DIR/lib/migrator.sh"
  . "$INSTALL_DIR/lib/upgrade_banner.sh"

  BANNER_STATE="$(upgrade_banner_detect_state "$HOME/.claude/settings.json" 2>/dev/null || true)"
  if [[ -n "$BANNER_STATE" ]]; then
    upgrade_banner_print "$BANNER_STATE"
    _confirm_result=0
    upgrade_banner_confirm "$BANNER_STATE" "$([[ $YES == 1 ]] && echo true || echo false)" "$([[ $LEGACY_V1 == 1 ]] && echo true || echo false)" || _confirm_result=$?
    if [[ "$_confirm_result" == "1" ]]; then
      echo "Install complete. Re-run with --yes to migrate to v2, or --legacy-v1 to keep v1." >&2
      exit 3
    elif [[ "$_confirm_result" == "2" ]]; then
      echo "Keeping v1 configuration. Run 'juggernaut apply' whenever you are ready to upgrade."
      if [[ "${JUGGERNAUT_SUPPRESS_DEPRECATION:-0}" != "1" ]]; then
        echo "Note: Juggernaut v1 is deprecated and will be removed in v3.0." >&2
      fi
    else
      # Auto-migrate v1 → v2 if a v1 block is present.
      HAS_V1="$(printf '%s' "$BANNER_STATE" | jq -r '.has_v1')"
      if [[ "$HAS_V1" == "true" ]]; then
        echo "Migrating v1 configuration to v2..."
        V1_PROFILES=()
        while IFS= read -r p; do
          [[ -n "$p" ]] && V1_PROFILES+=("$p")
        done < <(printf '%s' "$BANNER_STATE" | jq -r '.v1_profiles[]')
        for profile in "${V1_PROFILES[@]+"${V1_PROFILES[@]}"}"; do
          if migrator_has_v1_block "$profile" 2>/dev/null; then
            migrator_run "$profile" "$HOME/.claude/settings.json" "$INSTALL_DIR/bedrock-config.json" 2>/dev/null || true
          fi
        done
      fi
    fi
  fi
fi

echo "Verify with: juggernaut doctor"
echo "Configure with one of:"
echo "  juggernaut apply --auth=bedrock-api-key"
echo "  juggernaut apply --auth=iam"

if [[ "$CONFIGURE" == "1" ]]; then
  cd "$INSTALL_DIR"
  exec bash ./juggernaut apply "${SETUP_ARGS[@]+"${SETUP_ARGS[@]}"}"
elif [[ ${#SETUP_ARGS[@]} -gt 0 ]]; then
  echo "Note: install arguments after --version were ignored. Use --configure to run apply during install." >&2
fi
