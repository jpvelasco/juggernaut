#!/usr/bin/env bash
# install.sh - Juggernaut v3 installer (wipe-and-reinstall)
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --version v3.0.8
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --ref fix-branch
#   curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --latest
#
# Or after downloading:
#   bash install.sh --version v3.0.8
#   bash install.sh --ref fix-branch
#   bash install.sh --latest --dry-run
#
# Destructive behavior (v3):
#   - Strips Juggernaut and legacy "Claude Code Bedrock Configuration" BEGIN/END
#     blocks from every known shell profile on this machine.
#   - Removes the "juggernaut" key from ~/.claude/settings.json (backup rotation
#     via config_manager preserves the 5 most recent copies).
#   - Removes the "juggernaut-bedrock" OS-keychain entry.
#   - Does NOT auto-apply. Run 'juggernaut apply --auth=iam' or
#     'juggernaut apply --auth=bedrock-api-key' explicitly after install.

set -e

REPO_URL="${JUGGERNAUT_REPO_URL:-https://github.com/jpvelasco/juggernaut.git}"
INSTALL_DIR="${JUGGERNAUT_DIR:-$HOME/.juggernaut}"
VERSION=""
REF="${JUGGERNAUT_REF:-}"
DRY_RUN=0

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
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    --help|-h)
      cat <<'EOF'
Juggernaut v3 installer

Usage:
  install.sh [--version <tag>] [--ref <branch|sha>] [--latest] [--dry-run]

Installs Juggernaut to ~/.juggernaut (override with JUGGERNAUT_DIR).
Before installing, strips legacy Juggernaut/Claude-Code-Bedrock blocks from
shell profiles, removes the 'juggernaut' key from ~/.claude/settings.json,
and deletes the 'juggernaut-bedrock' keychain entry.

--dry-run prints what would be wiped and exits without writing anything.
EOF
      exit 0
      ;;
    *)
      echo "Error: unknown argument '$1'" >&2
      echo "Run 'install.sh --help' for usage." >&2
      exit 1
      ;;
  esac
done

if [[ -n "$VERSION" && "$VERSION" != v* ]]; then
  VERSION="v${VERSION}"
fi

if ! command -v git >/dev/null 2>&1; then
  echo "Error: git is required but not installed" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Pre-wipe summary
# ---------------------------------------------------------------------------
SETTINGS_PATH="$HOME/.claude/settings.json"
KEYCHAIN_SERVICE_NAME="juggernaut-bedrock"
PROFILE_TOKEN_PATH="${XDG_CONFIG_HOME:-$HOME/.config}/juggernaut/bearer-token"

# Shell profile candidates (must stay in sync with lib/profile_paths.sh)
PROFILE_CANDIDATES=(
  "$HOME/.bashrc"
  "$HOME/.bash_profile"
  "$HOME/.zshrc"
  "$HOME/.config/fish/config.fish"
  "$HOME/.profile"
)

profile_has_juggernaut_block() {
  local path="$1"
  [[ -f "$path" ]] || return 1
  grep -qE '^# BEGIN: Juggernaut|^# BEGIN: Claude Code Bedrock Configuration' "$path" 2>/dev/null
}

settings_has_juggernaut() {
  [[ -f "$SETTINGS_PATH" ]] || return 1
  if command -v jq >/dev/null 2>&1; then
    jq -e '.juggernaut // empty' "$SETTINGS_PATH" >/dev/null 2>&1
  else
    grep -qE '"juggernaut"[[:space:]]*:' "$SETTINGS_PATH" 2>/dev/null
  fi
}

keychain_has_entry() {
  case "$OSTYPE" in
    darwin*)
      command -v security >/dev/null 2>&1 \
        && security find-generic-password -s "$KEYCHAIN_SERVICE_NAME" -a api-key -w >/dev/null 2>&1
      ;;
    linux*)
      command -v secret-tool >/dev/null 2>&1 \
        && [[ -n "$(secret-tool lookup service "$KEYCHAIN_SERVICE_NAME" account api-key 2>/dev/null)" ]]
      ;;
    msys*|mingw*|cygwin*)
      command -v cmdkey.exe >/dev/null 2>&1 \
        && cmdkey.exe /list:"$KEYCHAIN_SERVICE_NAME" 2>/dev/null | grep -q "$KEYCHAIN_SERVICE_NAME"
      ;;
    *)
      return 1
      ;;
  esac
}

TO_STRIP_PROFILES=()
for p in "${PROFILE_CANDIDATES[@]}"; do
  if profile_has_juggernaut_block "$p"; then
    TO_STRIP_PROFILES+=("$p")
  fi
done

STRIP_SETTINGS=0
settings_has_juggernaut && STRIP_SETTINGS=1

STRIP_KEYCHAIN=0
keychain_has_entry && STRIP_KEYCHAIN=1

STRIP_PROFILE_TOKEN=0
[[ -f "$PROFILE_TOKEN_PATH" ]] && STRIP_PROFILE_TOKEN=1

echo "Juggernaut installer - wipe-and-reinstall"
echo ""
echo "Pre-wipe summary:"
if [[ ${#TO_STRIP_PROFILES[@]} -gt 0 ]]; then
  for p in "${TO_STRIP_PROFILES[@]}"; do
    echo "  - strip Juggernaut/v1 block from $p"
  done
else
  echo "  - shell profiles: no Juggernaut/v1 blocks found"
fi
if [[ "$STRIP_SETTINGS" -eq 1 ]]; then
  echo "  - remove 'juggernaut' key from $SETTINGS_PATH"
else
  echo "  - settings.json: no 'juggernaut' key found"
fi
if [[ "$STRIP_KEYCHAIN" -eq 1 ]]; then
  echo "  - remove OS-keychain entry '$KEYCHAIN_SERVICE_NAME'"
else
  echo "  - keychain: no '$KEYCHAIN_SERVICE_NAME' entry found"
fi
if [[ "$STRIP_PROFILE_TOKEN" -eq 1 ]]; then
  echo "  - remove profile token file $PROFILE_TOKEN_PATH"
else
  echo "  - profile token: no token file found"
fi
echo ""

if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "--dry-run: no changes written. Exiting."
  exit 0
fi

# ---------------------------------------------------------------------------
# Wipe
# ---------------------------------------------------------------------------
strip_profile_block() {
  local path="$1"
  [[ -f "$path" ]] || return 0
  local tmp
  tmp="$(mktemp "${path}.wipeXXXXXX")"
  awk '
    BEGIN { skip = 0 }
    /^# BEGIN: Juggernaut/ || /^# BEGIN: Claude Code Bedrock Configuration/ { skip = 1; next }
    /^# END: Juggernaut/   || /^# END: Claude Code Bedrock Configuration/   { skip = 0; next }
    skip == 0 { print }
  ' "$path" > "$tmp"
  mv "$tmp" "$path"
}

for p in "${TO_STRIP_PROFILES[@]}"; do
  strip_profile_block "$p"
  echo "Stripped Juggernaut block from $p"
done

if [[ "$STRIP_SETTINGS" -eq 1 ]]; then
  if command -v jq >/dev/null 2>&1; then
    tmp="$(mktemp "${SETTINGS_PATH}.wipeXXXXXX")"
    jq 'del(.juggernaut)' "$SETTINGS_PATH" > "$tmp" && mv "$tmp" "$SETTINGS_PATH"
    echo "Removed 'juggernaut' key from $SETTINGS_PATH"
  else
    echo "Warning: jq not found; leaving 'juggernaut' key in $SETTINGS_PATH" >&2
    echo "  Install jq and re-run 'install.sh' to complete the wipe." >&2
  fi
fi

if [[ "$STRIP_KEYCHAIN" -eq 1 ]]; then
  case "$OSTYPE" in
    darwin*)
      security delete-generic-password -s "$KEYCHAIN_SERVICE_NAME" -a api-key >/dev/null 2>&1 || true ;;
    linux*)
      secret-tool clear service "$KEYCHAIN_SERVICE_NAME" account api-key >/dev/null 2>&1 || true ;;
    msys*|mingw*|cygwin*)
      cmdkey.exe /delete:"$KEYCHAIN_SERVICE_NAME" >/dev/null 2>&1 || true ;;
  esac
  echo "Removed keychain entry: $KEYCHAIN_SERVICE_NAME"
fi

if [[ "$STRIP_PROFILE_TOKEN" -eq 1 ]]; then
  rm -f -- "$PROFILE_TOKEN_PATH"
  echo "Removed profile token file: $PROFILE_TOKEN_PATH"
fi

# ---------------------------------------------------------------------------
# Install
# ---------------------------------------------------------------------------
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
    git -c advice.detachedHead=false clone --branch "$VERSION" --depth 1 --quiet "$REPO_URL" "$target"
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

  # Always rotate: keep only the 5 most recent backups.
  if [[ -n "$INSTALL_DIR" ]]; then
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
      git -C "$INSTALL_DIR" -c advice.detachedHead=false checkout --quiet "$VERSION"
    else
      git -C "$INSTALL_DIR" checkout --quiet main
      git -C "$INSTALL_DIR" pull --ff-only --quiet
    fi
  fi
else
  clone_install
fi

echo "Installed to $INSTALL_DIR"

# Executable bits only matter on non-Windows; chmod on Git Bash silently
# misbehaves but does not fail. Suppress errors so Windows runs stay clean.
chmod +x "$INSTALL_DIR/juggernaut" "$INSTALL_DIR"/commands/*.sh "$INSTALL_DIR"/lib/*.sh 2>/dev/null || true

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
# Claude launcher: bracketed shell function in the user's shell profiles.
# ---------------------------------------------------------------------------
# Mirrors the Windows PS profile function approach. The function reads
# AWS_BEARER_TOKEN_BEDROCK from the OS keychain (via lib/keychain.sh) and
# exports it before exec'ing the real claude binary via the `command`
# builtin (which bypasses the function). A shell function takes precedence
# over binaries on PATH, so this survives `claude update` self-rewrites
# that would have clobbered a symlink at ~/.local/bin/claude.
install_launcher_profile_block() {
  local begin='# BEGIN: Juggernaut Launcher'
  local end='# END: Juggernaut Launcher'
  local install_dir="$INSTALL_DIR"

  # The function body is written verbatim into the user's profile. Any
  # substitution that must happen at install time (INSTALL_DIR, service
  # name default) uses ${INSTALL_DIR_LITERAL} / heredoc interpolation.
  local block
  block=$(cat <<LAUNCHER
# BEGIN: Juggernaut Launcher
# Juggernaut claude launcher - injects AWS_BEARER_TOKEN_BEDROCK from DPAPI or
# the OS keychain before exec'ing the real claude binary. Silent on success.
claude() {
  if [ -z "\${AWS_BEARER_TOKEN_BEDROCK:-}" ]; then
    if [ -r "$install_dir/lib/keychain.sh" ]; then
      # shellcheck disable=SC1091
      _juggernaut_token=\$(. "$install_dir/lib/keychain.sh"; bearer_token_get 2>/dev/null) || _juggernaut_token=''
      if [ -n "\$_juggernaut_token" ]; then
        export AWS_BEARER_TOKEN_BEDROCK="\$_juggernaut_token"
      fi
      unset _juggernaut_token
    fi
  fi
  command claude "\$@"
}
# END: Juggernaut Launcher
LAUNCHER
)

  local profile_candidates=(
    "$HOME/.bashrc"
    "$HOME/.zshrc"
    "$HOME/.profile"
  )

  # Only write to profiles that already exist. This avoids creating a
  # .zshrc for a bash user (and vice versa).
  local targets=()
  for candidate in "${profile_candidates[@]}"; do
    if [[ -f "$candidate" ]]; then
      targets+=("$candidate")
    fi
  done

  # If the user's login shell has no rc file yet, seed the matching one.
  if [[ ${#targets[@]} -eq 0 ]]; then
    case "${SHELL:-}" in
      */zsh)  targets+=("$HOME/.zshrc") ;;
      */bash) targets+=("$HOME/.bashrc") ;;
      *)      targets+=("$HOME/.profile") ;;
    esac
  fi

  for path in "${targets[@]}"; do
    # Strip any existing launcher block first (idempotent re-install).
    if [[ -f "$path" ]]; then
      local tmp
      tmp="$(mktemp "${path}.launcherXXXXXX")"
      awk -v b="$begin" -v e="$end" '
        BEGIN { skip = 0 }
        $0 == b { skip = 1; next }
        $0 == e { skip = 0; next }
        skip == 0 { print }
      ' "$path" > "$tmp" && mv "$tmp" "$path"
    else
      # Create parent dir (e.g. ~/.config for fish in the future) and touch.
      mkdir -p "$(dirname "$path")"
      : > "$path"
    fi

    # Append fresh block with a leading blank line for separation.
    local needs_sep=0
    [[ -s "$path" ]] && needs_sep=1
    {
      (( needs_sep )) && printf '\n'
      printf '%s\n' "$block"
    } >> "$path"
    echo "Claude launcher function written to $path"
  done
}
# END install_launcher_profile_block

install_launcher_profile_block

echo ""
echo "Install complete. No configuration has been written."
echo "Configure Juggernaut explicitly with one of:"
echo "  juggernaut apply --auth=iam"
echo "  juggernaut apply --auth=bedrock-api-key"
echo ""
echo "Verify with: juggernaut doctor"
