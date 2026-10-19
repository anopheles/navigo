#!/bin/sh
# The CI check steps (ADR 0005 for the licence list). Run from the repo
# root, with Go, and mpv for the player tests. GitHub Actions runs this in
# the check job; tools/ci/local.sh runs it in the cached CI image.
set -eu

test -z "$(gofmt -l .)" || { gofmt -l .; exit 1; }
go vet ./...
go test ./...
command -v go-licenses >/dev/null || go install github.com/google/go-licenses/v2@v2.0.1
go-licenses check ./... --allowed_licenses=MIT,BSD-2-Clause,BSD-3-Clause,Apache-2.0,ISC,MPL-2.0
