#!/usr/bin/env bash
set -euo pipefail

REPO="jpvelasco/juggernaut"
BIN_DIR="${JUGGERNAUT_BIN_DIR:-${HOME}/.local/bin}"
VERSION="${JUGGERNAUT_VERSION:-latest}"

detect_platform() {
  local os arch
  case "$(uname -s)" in
    Linux)  os="linux" ;;
    Darwin) os="darwin" ;;
    *)      echo "Unsupported OS: $(uname -s)" >&2; exit 1 ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) echo "Unsupported arch: $(uname -m)" >&2; exit 1 ;;
  esac
  echo "${os}_${arch}"
}

get_latest_version() {
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed 's/.*"v\([^"]*\)".*/\1/'
}

main() {
  if [ "$VERSION" = "latest" ]; then
    VERSION=$(get_latest_version)
  fi

  local platform
  platform=$(detect_platform)
  local archive="juggernaut_${platform}.tar.gz"
  local url="https://github.com/${REPO}/releases/download/v${VERSION}/${archive}"
  local checksum_url="https://github.com/${REPO}/releases/download/v${VERSION}/checksums.txt"

  echo "Installing Juggernaut v${VERSION} (${platform})..."

  local tmp
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  curl -fsSL "$url" -o "${tmp}/${archive}"
  curl -fsSL "$checksum_url" -o "${tmp}/checksums.txt"

  # Verify checksum.
  (cd "$tmp" && grep "${archive}" checksums.txt | sha256sum -c -)

  mkdir -p "$BIN_DIR"
  tar -xzf "${tmp}/${archive}" -C "$tmp"
  mv "${tmp}/juggernaut" "${BIN_DIR}/juggernaut"
  chmod +x "${BIN_DIR}/juggernaut"

  echo "Juggernaut v${VERSION} installed to ${BIN_DIR}/juggernaut"
  echo ""
  echo "Next step: juggernaut apply"
}

main "$@"
