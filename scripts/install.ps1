#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$Version = 'latest',
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'clabok\bin')
)

$ErrorActionPreference = 'Stop'
$repo = 'Blue-Alien-Tec/clabok'

function Fail([string]$Message) {
    Write-Error $Message
    exit 1
}

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'ARM64' { 'arm64' }
    default { 'amd64' }
}

if ($Version -eq 'latest') {
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers @{ 'User-Agent' = 'clabok-installer' }
    } catch {
        Fail "cannot resolve the latest release: $($_.Exception.Message)"
    }
    $Version = $release.tag_name
}
$Version = $Version.TrimStart('v')

$base = "https://github.com/$repo/releases/download/v$Version"
$asset = "pecia_${Version}_windows_${arch}.zip"
$temp = Join-Path ([System.IO.Path]::GetTempPath()) ("clabok-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temp | Out-Null

try {
    $zip = Join-Path $temp $asset
    $sums = Join-Path $temp 'SHA256SUMS'
    Invoke-WebRequest -Uri "$base/$asset" -OutFile $zip -UseBasicParsing
    Invoke-WebRequest -Uri "$base/SHA256SUMS" -OutFile $sums -UseBasicParsing

    $line = Get-Content -LiteralPath $sums | Where-Object { $_ -match [regex]::Escape($asset) } | Select-Object -First 1
    if (-not $line) {
        Fail "no checksum for $asset in SHA256SUMS"
    }
    $expected = ($line -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($expected -ne $actual) {
        Fail "checksum mismatch for $asset (expected $expected, got $actual)"
    }

    Expand-Archive -LiteralPath $zip -DestinationPath $temp -Force
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $temp 'pecia.exe') -Destination (Join-Path $InstallDir 'pecia.exe') -Force

    Write-Output "installed pecia $Version to $InstallDir"
    if (-not (($env:PATH -split ';') -contains $InstallDir)) {
        Write-Output "add $InstallDir to PATH to run pecia from any directory"
    }
    & (Join-Path $InstallDir 'pecia.exe') version
} finally {
    Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue
}
