#!/usr/bin/env bash
# lib/config_manager.sh — settings.json read/merge/write for Juggernaut v2.
# Atomic writes, rotated backups, best-effort locking.
# Requires: bash 4+, jq.  Optional: flock (mkdir fallback when absent).

set -euo pipefail

CONFIG_BACKUP_RETAIN=5

# Copy the mode from $1 to $2 using a portable stat (GNU or BSD) and chmod.
# Falls back to 0600 on failure — settings.json may hold keys, err on tighter.
config_copy_mode() {
  local src="$1" dst="$2" mode
  mode="$(stat -c '%a' "$src" 2>/dev/null || stat -f '%Lp' "$src" 2>/dev/null || echo "")"
  if [[ -n "$mode" ]]; then
    chmod "$mode" "$dst" 2>/dev/null && return 0
  fi
  chmod 0600 "$dst" 2>/dev/null || true
}

config_user_settings_path() {
  echo "${HOME}/.claude/settings.json"
}

config_project_settings_path() {
  # Walks up from CWD looking for .claude/settings.json, stops at $HOME or /.
  # Optional $1 = start dir (default: $PWD). Returns 1 if not found.
  local dir="${1:-$PWD}"
  while [[ "$dir" != "/" && "$dir" != "$HOME" ]]; do
    if [[ -f "$dir/.claude/settings.json" ]]; then
      echo "$dir/.claude/settings.json"
      return 0
    fi
    dir="$(dirname -- "$dir")"
  done
  return 1
}

config_resolve_target() {
  local scope="${1:-user}"
  case "$scope" in
    user)    config_user_settings_path ;;
    project) echo "${PWD}/.claude/settings.json" ;;
    *) echo "config_resolve_target: unknown scope '$scope'" >&2; return 1 ;;
  esac
}

config_exists() {
  local path="$1"
  [[ -s "$path" ]]
}

# config_read <path>
# Prints the file contents as JSON. If file is absent or empty, prints "{}".
# Errors out if file exists but is malformed.
config_read() {
  local path="$1"
  if [[ ! -s "$path" ]]; then
    echo "{}"
    return 0
  fi
  if ! jq -e . "$path" >/dev/null 2>&1; then
    echo "config_read: $path is not valid JSON" >&2
    return 1
  fi
  cat "$path"
}

config_has_juggernaut_block() {
  local json="$1"
  local managed
  managed="$(printf '%s' "$json" | jq -r '.juggernaut.meta.managedBy // ""')"
  [[ "$managed" == "juggernaut" ]]
}

config_get_juggernaut_block() {
  local json="$1"
  printf '%s' "$json" | jq '.juggernaut // null'
}

# config_merge_juggernaut_block <existing_json> <new_block> <native_keys>
# Returns existing_json with:
#   .juggernaut replaced by new_block
#   native keys (env, model, modelOverrides, availableModels) replaced by values from native_keys
# User's other top-level keys (permissions, hooks, theme, etc.) are preserved untouched.
config_merge_juggernaut_block() {
  local existing="$1"
  local new_block="$2"
  local native_keys="$3"

  jq -n \
    --argjson existing "$existing" \
    --argjson block "$new_block" \
    --argjson native "$native_keys" \
    '
    $existing
    | .juggernaut = $block
    | .env = $native.env
    | .model = $native.model
    | .modelOverrides = $native.modelOverrides
    '
}

# config_remove_juggernaut_block <existing_json>
# Strips every key Juggernaut writes: .juggernaut plus the derived natives
# (.env, .model, .modelOverrides, .availableModels). availableModels is also
# treated as Juggernaut-managed — users who want to keep it should store it
# under a different key.
# User's other top-level keys (permissions, hooks, theme, ...) are preserved.
config_remove_juggernaut_block() {
  local existing="$1"
  printf '%s' "$existing" | jq '
    del(.juggernaut)
    | del(.env)
    | del(.model)
    | del(.modelOverrides)
    | del(.availableModels)
  '
}

# config_backup <path>
# Copies path → path.backup.YYYYMMDD_HHMMSS. Rotates: keeps most recent CONFIG_BACKUP_RETAIN, prunes older.
# No-op if path doesn't exist.
config_backup() {
  local path="$1"
  [[ -f "$path" ]] || return 0
  local stamp
  stamp="$(date +%Y%m%d_%H%M%S)"
  local backup="${path}.backup.${stamp}"
  cp -p "$path" "$backup"
  echo "$backup"
  config_rotate_backups "$path"
}

config_rotate_backups() {
  local path="$1"
  local dir base
  dir="$(dirname -- "$path")"
  base="$(basename -- "$path")"

  # Sort backups by mtime descending. Use ls -t (portable) on the glob pattern;
  # names will not contain newlines or spaces because the backup stamp is YYYYMMDD_HHMMSS.
  local -a backups=()
  while IFS= read -r f; do
    [[ -n "$f" ]] && backups+=("$f")
  done < <(ls -1t "$dir"/"${base}".backup.* 2>/dev/null)

  local i=0
  for f in "${backups[@]}"; do
    i=$((i + 1))
    if (( i > CONFIG_BACKUP_RETAIN )); then
      rm -f -- "$f"
    fi
  done
}

# config_write_atomic <path> <json_string>
# Validate → backup → write tmp → rename. Whole body runs under lock so two
# concurrent callers cannot corrupt each other's tmp file.
#
# Atomicity caveats: mv(1) is atomic only within the same filesystem. sync(1)
# is best-effort; not all OSes support it. On a power failure, settings.json
# may be the old value or the new value, never a mix.
config_write_atomic() {
  local path="$1"
  local content="$2"

  # Validate JSON before doing anything — no lock needed for a pure check.
  if ! printf '%s' "$content" | jq -e . >/dev/null 2>&1; then
    echo "config_write_atomic: refusing to write invalid JSON to $path" >&2
    return 1
  fi

  local dir
  dir="$(dirname -- "$path")"
  if ! mkdir -p -- "$dir"; then
    echo "config_write_atomic: cannot create parent directory $dir" >&2
    return 1
  fi

  config_with_lock "$path" _config_write_atomic_locked "$path" "$content"
}

# Internal: the locked critical section of config_write_atomic. Do not call directly.
_config_write_atomic_locked() {
  local path="$1"
  local content="$2"
  local tmp="${path}.tmp.$$"

  # Clean tmp if any earlier run died mid-write.
  rm -f -- "$tmp"

  if [[ -f "$path" ]]; then
    if ! config_backup "$path" >/dev/null; then
      echo "_config_write_atomic_locked: backup failed for $path" >&2
      return 1
    fi
  fi

  # Pretty-print on write so settings.json stays human-readable.
  if ! printf '%s' "$content" | jq '.' >"$tmp"; then
    echo "_config_write_atomic_locked: failed to write tmp file $tmp" >&2
    rm -f -- "$tmp"
    return 1
  fi

  if [[ -f "$path" ]]; then
    config_copy_mode "$path" "$tmp"
  else
    chmod 0600 "$tmp" 2>/dev/null || true
  fi

  # Best-effort fsync. Not every OS supports `sync <file>`; failures are tolerable.
  if command -v sync >/dev/null 2>&1; then
    sync "$tmp" 2>/dev/null || true
  fi

  if ! mv -f -- "$tmp" "$path"; then
    echo "_config_write_atomic_locked: rename $tmp → $path failed" >&2
    rm -f -- "$tmp"
    return 1
  fi
}

# config_with_lock <path> <command...>
# Runs command under an exclusive flock on path.lock with a 10s timeout.
# Falls back to a directory-based mkdir lock when flock is absent (macOS by default,
# Git Bash on Windows). On fallback, caller gets the same blocking semantics.
config_with_lock() {
  local path="$1"; shift
  local lockfile="${path}.lock"
  mkdir -p -- "$(dirname -- "$lockfile")"

  if command -v flock >/dev/null 2>&1; then
    local rc
    {
      if ! flock -x -w 10 9; then
        echo "config_with_lock: could not acquire flock on $lockfile within 10s" >&2
        return 1
      fi
      "$@"
      rc=$?
    } 9>"$lockfile"
    return "$rc"
  fi

  # Fallback: mkdir-based mutex (POSIX-atomic: mkdir succeeds only for one caller).
  # Stale-lock recovery: if the lockdir is older than 30s, assume the prior holder
  # died without cleanup and remove it. The 30s window is generous enough to cover
  # any real write, but short enough not to block a manual re-run after a crash.
  local lockdir="${path}.lockdir"
  local waited=0
  until mkdir -- "$lockdir" 2>/dev/null; do
    # Check for stale lock before deciding to wait or fail.
    if [[ -d "$lockdir" ]]; then
      local now mtime lockage
      now="$(date +%s)"
      mtime="$(stat -c '%Y' "$lockdir" 2>/dev/null || stat -f '%m' "$lockdir" 2>/dev/null || echo "0")"
      lockage=$(( now - mtime ))
      if (( lockage > 30 )); then
        rmdir -- "$lockdir" 2>/dev/null || true
        continue
      fi
    fi
    if (( waited >= 10 )); then
      echo "config_with_lock: could not acquire mkdir lock on $lockdir within 10s" >&2
      return 1
    fi
    sleep 1
    waited=$((waited + 1))
  done
  # Run the action and always release the lock, even on failure.
  local rc=0
  "$@" || rc=$?
  rmdir -- "$lockdir" 2>/dev/null || true
  return "$rc"
}

# config_load_effective
# Returns a JSON object summarizing what's visible in both scopes.
# v2.0: no merging — just side-by-side display.
# Returns 1 if either scope's file exists but is unreadable/malformed so callers
# (e.g., `juggernaut doctor`) can surface a real error instead of silent empties.
config_load_effective() {
  local user_path project_path user_json project_json
  user_path="$(config_user_settings_path)"
  project_path="$(config_project_settings_path "$PWD" 2>/dev/null || true)"

  if ! user_json="$(config_read "$user_path")"; then
    echo "config_load_effective: failed to read user settings at $user_path" >&2
    return 1
  fi
  if [[ -n "$project_path" ]]; then
    if ! project_json="$(config_read "$project_path")"; then
      echo "config_load_effective: failed to read project settings at $project_path" >&2
      return 1
    fi
  else
    project_json="null"
  fi

  jq -n \
    --arg userPath "$user_path" \
    --arg projectPath "${project_path:-}" \
    --argjson user "$user_json" \
    --argjson project "$project_json" \
    '
    {
      user:    { path: $userPath,    settings: $user },
      project: (if $projectPath | length > 0 then { path: $projectPath, settings: $project } else null end)
    }
    '
}
