#!/usr/bin/env bash
set -e

REPO_URL="https://github.com/jpvelasco/juggernaut.git"
INSTALL_DIR="${JUGGERNAUT_DIR:-$HOME/.juggernaut}"

echo "Installing Juggernaut..."

if [[ -d "$INSTALL_DIR" ]]; then
    echo "Updating existing installation in $INSTALL_DIR"
    git -C "$INSTALL_DIR" pull --ff-only
else
    git clone "$REPO_URL" "$INSTALL_DIR"
fi

cd "$INSTALL_DIR"
exec bash ./setup "$@"
