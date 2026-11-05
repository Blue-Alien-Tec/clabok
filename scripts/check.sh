#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
    echo "gofmt needed:"
    echo "$unformatted"
    exit 1
fi

go vet ./...
go build ./...
go test ./...
