#!/bin/sh
# Install git hooks from .githooks/
set -e

HOOKS_DIR="$(git rev-parse --git-dir)/hooks"
mkdir -p "$HOOKS_DIR"

for hook in .githooks/*; do
  cp "$hook" "$HOOKS_DIR/"
  chmod +x "$HOOKS_DIR/$(basename "$hook")"
  echo "Installed $(basename "$hook")"
done

echo "Git hooks installed successfully."
