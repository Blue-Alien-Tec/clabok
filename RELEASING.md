# Releasing CLABOK

Releases are tag-driven. CI never creates the tag; the release script validates the tree and pushes
an annotated tag with the operator's or agent's credentials. The `release` workflow then builds the
six archives, writes `SHA256SUMS` and publishes the GitHub Release.

## Prerequisites

- Push access to `main` and the ability to push tags.
- Go 1.27.3+ on `PATH`, plus `gofmt` and `git`.
- A clean, up-to-date `main`.

## Release

```text
# Windows
.\scripts\release.ps1 <version>

# Unix
./scripts/release.sh <version>
```

The script refuses a dirty tree, a branch other than `main`, a HEAD that differs from `origin/main`,
a version that does not match `internal/buildinfo.Version` (`<version>` or `<version>-dev`), an
existing tag, unformatted Go files or failing vet/test/build. It then creates `v<version>` and pushes
it. Use `-DryRun` (PowerShell) or `--dry-run` (shell) to run every check without creating or pushing
the tag.

Before the tag, the script rebuilds `pecia.exe`, renders the layout and asserts two things: the
working tree is still clean afterwards, and `README.md` carries the `version-<version>` badge. A stale
render therefore aborts the release with the changed files listed — commit them and run again. The
render step exists because the `version` badge resolves from `buildinfo.Version` of the binary that
renders, not from the source tree, so a binary built before the bump publishes the previous version.

If the local execution policy blocks PowerShell scripts, run
`powershell -ExecutionPolicy Bypass -File .\scripts\release.ps1 <version>` or use
`scripts/release.sh`.

Do not re-tag a published version: the Go module proxy keeps the first source for a version. Publish
the next patch instead.

## What the workflow does

`.github/workflows/release.yml` runs on `v*` tags:

1. verify: gofmt, vet, build, test.
2. build: windows/amd64+arm64, linux/amd64+arm64 and darwin/amd64+arm64 with `CGO_ENABLED=0`,
   `-trimpath` and `-ldflags "-s -w -X ...buildinfo.Version=<tag>"`.
3. release: archives plus `SHA256SUMS`, generated release notes, and a prerelease for a `-suffix`
   tag.

`workflow_dispatch` builds the six archives as workflow artifacts and never publishes. Use it to
test the matrix without a tag.

## Verify

- The release has six archives and `SHA256SUMS`.
- `sha256sum -c SHA256SUMS` passes.
- The downloaded binary reports the tag: `pecia version`.
- `go install github.com/Blue-Alien-Tec/clabok/cmd/pecia@v<version>` works; `@latest` resolves to the
  release.
- If the default proxy still reports the previous version for `@latest`, verify with the pinned
  `@vX.Y.Z`; the proxy's `@latest` cache refreshes after a short delay.

## Package manifests

`bucket/clabok.json` and `Formula/clabok.rb` carry the archive URLs and SHA256 values for the
current release. Scoop's `checkver`/`autoupdate` handles later versions; the Homebrew formula needs
the new URLs and hashes after each release. Update both files from the published `SHA256SUMS`,
commit them as `chore(release): update the package manifests to v<version>`, and regenerate the
repository layout with `pecia sync --write` because the tracked file set changes. A separate
`homebrew-clabok` tap is the cleaner long-term layout; the formula in this repository is the first
step.

## Signing

Windows Authenticode and macOS notarization are out of scope: both require paid certificates or
memberships. The Windows binary may trigger SmartScreen; the macOS binary may trigger Gatekeeper.
Document the bypass; do not add signing without an explicit cost decision.
