#!/usr/bin/env bash
set -euo pipefail

dry_run=0
if [[ "${1:-}" == "--dry-run" ]]; then
  dry_run=1
  shift
fi
version="${1:-}"
if [[ -z "$version" ]]; then
  echo "usage: scripts/release.sh [--dry-run] <version>" >&2
  exit 1
fi
version="${version#v}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "invalid version '$version'; expected MAJOR.MINOR.PATCH or MAJOR.MINOR.PATCH-suffix" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if [[ -n "$(git status --porcelain)" ]]; then
  echo "the working tree is not clean" >&2
  git status --short >&2
  exit 1
fi

branch="$(git rev-parse --abbrev-ref HEAD)"
if [[ "$branch" != "main" ]]; then
  echo "release from main, not '$branch'" >&2
  exit 1
fi

git fetch origin main --tags
counts="$(git rev-list --left-right --count HEAD...origin/main)"
if [[ "$counts" != $'0\t0' ]]; then
  echo "HEAD is not equal to origin/main (ahead/behind: $counts); push or pull first" >&2
  exit 1
fi

source_version="$(sed -n 's/.*Version = "\(.*\)".*/\1/p' internal/buildinfo/buildinfo.go | head -n 1)"
if [[ "$source_version" != "$version" && "$source_version" != "$version-dev" ]]; then
  echo "buildinfo.Version is '$source_version', expected '$version' or '$version-dev'" >&2
  exit 1
fi

unformatted="$(gofmt -l .)"
if [[ -n "$unformatted" ]]; then
  echo "gofmt reports unformatted files" >&2
  echo "$unformatted" >&2
  exit 1
fi

go vet ./...
go test ./... -count=1
go build ./...

tag="v$version"
if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  echo "tag $tag already exists locally" >&2
  exit 1
fi
if [[ -n "$(git ls-remote --tags origin "refs/tags/$tag")" ]]; then
  echo "tag $tag already exists on origin" >&2
  exit 1
fi

if [[ "$dry_run" == "1" ]]; then
  echo "dry run: would tag $tag and push it"
  exit 0
fi

git tag -a "$tag" -m "CLABOK $version"
git push origin "$tag"

echo "released $tag"
echo "https://github.com/Blue-Alien-Tec/clabok/releases/tag/$tag"
