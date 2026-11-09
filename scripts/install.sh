#!/usr/bin/env bash
set -euo pipefail

repo="Blue-Alien-Tec/clabok"
version="latest"
install_dir="${HOME}/.local/bin"

usage() {
  echo "usage: scripts/install.sh [--version <version>] [--dir <install-dir>]" >&2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version|-v) version="${2:-}"; shift 2 ;;
    --dir|-d) install_dir="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 1 ;;
  esac
done

if [[ "$version" == "latest" ]]; then
  if ! command -v curl >/dev/null 2>&1; then
    echo "curl is required to resolve the latest release" >&2
    exit 1
  fi
  tag="$(curl -fsSL "https://api.github.com/repos/${repo}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)"
  if [[ -z "$tag" ]]; then
    echo "cannot resolve the latest release" >&2
    exit 1
  fi
  version="$tag"
fi
version="${version#v}"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

install_dir="${install_dir/#\~/$HOME}"
base="https://github.com/${repo}/releases/download/v${version}"
asset="pecia_${version}_${os}_${arch}.tar.gz"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$base/$asset" -o "$tmp/$asset"
  curl -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"
elif command -v wget >/dev/null 2>&1; then
  wget -q "$base/$asset" -O "$tmp/$asset"
  wget -q "$base/SHA256SUMS" -O "$tmp/SHA256SUMS"
else
  echo "curl or wget is required" >&2
  exit 1
fi

expected="$(grep -F "$asset" "$tmp/SHA256SUMS" | awk '{print $1}' | head -n 1)"
if [[ -z "$expected" ]]; then
  echo "no checksum for $asset in SHA256SUMS" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp/$asset" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')"
fi
if [[ "$expected" != "$actual" ]]; then
  echo "checksum mismatch for $asset (expected $expected, got $actual)" >&2
  exit 1
fi

tar -xzf "$tmp/$asset" -C "$tmp" pecia
mkdir -p "$install_dir"
install -m 0755 "$tmp/pecia" "$install_dir/pecia"

echo "installed pecia $version to $install_dir"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) echo "add $install_dir to PATH to run pecia from any directory" ;;
esac
"$install_dir/pecia" version
