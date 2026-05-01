#!/usr/bin/env bash
# lib/profile_paths.sh — Canonical list of shell profile candidates for v1 block detection.
# Sourced by apply, migrate, uninstall, doctor, and upgrade_banner.

set -euo pipefail

# profile_paths_v1_candidates
# Prints one absolute path per line for each profile file that may contain a
# Juggernaut v1 BEGIN/END block. Caller filters with migrator_has_v1_block.
profile_paths_v1_candidates() {
  local home="${HOME:-}"
  [[ -z "$home" ]] && return 0

  printf '%s\n' \
    "$home/.bashrc" \
    "$home/.bash_profile" \
    "$home/.zshrc" \
    "$home/.config/fish/config.fish" \
    "$home/.profile"
}
