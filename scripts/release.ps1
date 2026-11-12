#Requires -Version 5.1
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$Version,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'

function Fail([string]$Message) {
    Write-Error $Message
    exit 1
}

function Invoke-Git {
    param([string[]]$GitArgs)
    & git @GitArgs
    if ($LASTEXITCODE -ne 0) {
        Fail "git $($GitArgs -join ' ') failed with exit code $LASTEXITCODE"
    }
}

$Version = $Version.Trim()
if ($Version.StartsWith('v')) {
    $Version = $Version.Substring(1)
}
if ($Version -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$') {
    Fail "invalid version '$Version'; expected MAJOR.MINOR.PATCH or MAJOR.MINOR.PATCH-suffix"
}

$RepoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $RepoRoot

$dirty = git status --porcelain
if ($dirty) {
    Fail "the working tree is not clean:`n$dirty"
}

$branch = git rev-parse --abbrev-ref HEAD
if ($branch -ne 'main') {
    Fail "release from main, not '$branch'"
}

Invoke-Git -GitArgs @('fetch', 'origin', 'main', '--tags')
$counts = (git rev-list --left-right --count HEAD...origin/main).Trim()
if ($counts -ne "0`t0") {
    Fail "HEAD is not equal to origin/main (ahead/behind: $counts); push or pull first"
}

$match = Select-String -LiteralPath 'internal/buildinfo/buildinfo.go' -Pattern 'Version\s*=\s*"([^"]+)"'
if (-not $match) {
    Fail 'cannot read internal/buildinfo/buildinfo.go Version'
}
$sourceVersion = $match.Matches[0].Groups[1].Value
if ($sourceVersion -ne $Version -and $sourceVersion -ne "$Version-dev") {
    Fail "buildinfo.Version is '$sourceVersion', expected '$Version' or '$Version-dev'"
}

$unformatted = gofmt -l .
if ($unformatted) {
    Fail "gofmt reports unformatted files:`n$unformatted"
}

& go vet ./...
if ($LASTEXITCODE -ne 0) { Fail 'go vet failed' }
& go test ./... -count=1
if ($LASTEXITCODE -ne 0) { Fail 'go test failed' }
& go build ./...
if ($LASTEXITCODE -ne 0) { Fail 'go build failed' }

# The version badge resolves from the running binary's build version, so rebuild the development
# binary and render the layout before tagging. A stale binary publishes the previous version, and a
# stale render is a dirty tree the release must not tag.
& go build -o pecia.exe ./cmd/pecia
if ($LASTEXITCODE -ne 0) { Fail 'go build -o pecia.exe failed' }
$builtVersion = (& .\pecia.exe version | ConvertFrom-Json).data.version
if ($builtVersion -ne $Version -and $builtVersion -ne "$Version-dev") {
    Fail "the built binary reports '$builtVersion', expected '$Version' or '$Version-dev'"
}
if ($DryRun) {
    & .\pecia.exe sync --check
    if ($LASTEXITCODE -ne 0) {
        Fail 'pecia sync --check reports drift; run pecia sync --write and commit the result'
    }
} else {
    & .\pecia.exe sync --write
    if ($LASTEXITCODE -ne 0) { Fail 'pecia sync --write failed' }
    $rendered = git status --porcelain
    if ($rendered) {
        Fail "the render changed tracked files; commit them and run the release again:`n$rendered"
    }
}
$badge = "badge/version-$Version-"
if (-not (Select-String -LiteralPath 'README.md' -Pattern $badge -SimpleMatch)) {
    Fail "README.md does not carry the version $Version badge; the render used a binary of another version"
}

$tag = "v$Version"
$localTag = git rev-parse -q --verify "refs/tags/$tag" 2>$null
if ($LASTEXITCODE -eq 0 -and $localTag) {
    Fail "tag $tag already exists locally"
}
$remoteTag = git ls-remote --tags origin "refs/tags/$tag"
if ($remoteTag) {
    Fail "tag $tag already exists on origin"
}

if ($DryRun) {
    Write-Output "dry run: would tag $tag and push it"
    exit 0
}

Invoke-Git -GitArgs @('tag', '-a', $tag, '-m', "CLABOK $Version")
Invoke-Git -GitArgs @('push', 'origin', $tag)

Write-Output "released $tag"
Write-Output "https://github.com/Blue-Alien-Tec/clabok/releases/tag/$tag"
