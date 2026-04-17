#!/usr/bin/env bash
# lib/config_manager.sh — settings.json read/merge/write operations for Juggernaut v2.
# All writes atomic. Backups auto-rotated. No schema logic here — that lives in schema.sh.
# Requires: bash 4+, jq, flock (util-linux; macOS has it via brew or we fall back).

set -euo pipefail

CONFIG_BACKUP_RETAIN=5

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
  managed="$(echo "$json" | jq -r '.juggernaut.meta.managedBy // ""')"
  [[ "$managed" == "juggernaut" ]]
}

config_get_juggernaut_block() {
  local json="$1"
  echo "$json" | jq '.juggernaut // null'
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
# Returns existing_json with .juggernaut and all Juggernaut-owned native keys removed.
# User's other keys preserved.
config_remove_juggernaut_block() {
  local existing="$1"
  jq '
    del(.juggernaut)
    | del(.env)
    | del(.model)
    | del(.modelOverrides)
    | del(.availableModels)
  ' <<<"$existing"
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

  # Use find + mtime-sort to avoid parsing ls and to handle names with spaces.
  local -a backups=()
  while IFS= read -r -d '' f; do
    backups+=("$f")
  done < <(find "$dir" -maxdepth 1 -name "${base}.backup.*" -printf '%T@ %p\0' 2>/dev/null \
             | sort -z -rn \
             | cut -z -d' ' -f2-)

  local i=0
  for f in "${backups[@]}"; do
    i=$((i + 1))
    if (( i > CONFIG_BACKUP_RETAIN )); then
      rm -f -- "$f"
    fi
  done
}

# config_write_atomic <path> <json_string>
# Atomic write: create parent dir if missing, write to tmp, fsync, rename over target.
# Preserves file mode if file exists; sets 0600 on new files (contains keys).
# Calls config_backup first.
config_write_atomic() {
  local path="$1"
  local content="$2"

  local dir
  dir="$(dirname "$path")"
  mkdir -p "$dir"

  # Validate JSON before touching anything.
  if ! echo "$content" | jq -e . >/dev/null 2>&1; then
    echo "config_write_atomic: refusing to write invalid JSON to $path" >&2
    return 1
  fi

  if [[ -f "$path" ]]; then
    config_backup "$path" >/dev/null
  fi

  local tmp="${path}.tmp.$$"
  # Pretty-print on write so settings.json is human-readable.
  echo "$content" | jq '.' >"$tmp"

  # Preserve mode if target exists; otherwise tighten to 0600 (settings may hold keys).
  if [[ -f "$path" ]]; then
    chmod --reference="$path" "$tmp" 2>/dev/null || true
  else
    chmod 0600 "$tmp"
  fi

  # Best-effort fsync (not all OSes / all shells support sync -f).
  if command -v sync >/dev/null 2>&1; then
    sync "$tmp" 2>/dev/null || true
  fi

  mv -f -- "$tmp" "$path"
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

  # Fallback: mkdir-based mutex (POSIX, atomic on all filesystems we care about).
  local lockdir="${path}.lockdir"
  local waited=0
  until mkdir -- "$lockdir" 2>/dev/null; do
    if (( waited >= 10 )); then
      echo "config_with_lock: could not acquire mkdir lock on $lockdir within 10s" >&2
      return 1
    fi
    sleep 1
    waited=$((waited + 1))
  done
  trap 'rmdir -- "$lockdir" 2>/dev/null || true' RETURN
  "$@"
}

# config_load_effective
# Returns a JSON object summarizing what's visible in both scopes.
# v2.0: no merging — just side-by-side display.
config_load_effective() {
  local user_path project_path user_json project_json
  user_path="$(config_user_settings_path)"
  project_path="$(config_project_settings_path "$PWD" 2>/dev/null || true)"

  user_json="$(config_read "$user_path")"
  if [[ -n "$project_path" ]]; then
    project_json="$(config_read "$project_path")"
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
