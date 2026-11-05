$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    $unformatted = gofmt -l .
    if ($unformatted) {
        Write-Error "gofmt needed:`n$unformatted"
        exit 1
    }
    go vet ./...
    go build ./...
    go test ./...
} finally {
    Pop-Location
}
