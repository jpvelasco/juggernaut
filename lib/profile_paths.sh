#!/usr/bin/env bash
# lib/profile_paths.sh - Shell profile paths scanned for legacy Juggernaut
# and v1 "Claude Code Bedrock Configuration" blocks during installer wipe.
# Juggernaut v3 does not write to shell profiles; this list exists only so
# the installer can strip leftover blocks from earlier versions.

set -euo pipefail

# profile_paths_scan_targets
# Prints one absolute path per line for each shell profile that may still
# contain a Juggernaut or v1 BEGIN/END block on this machine.
profile_paths_scan_targets() {
  local home="${HOME:-}"
  [[ -z "$home" ]] && return 0

  printf '%s\n' \
    "$home/.bashrc" \
    "$home/.bash_profile" \
    "$home/.zshrc" \
    "$home/.config/fish/config.fish" \
    "$home/.profile"
}

# Legacy alias kept for any in-tree callers we may have missed.
profile_paths_v1_candidates() { profile_paths_scan_targets "$@"; }
