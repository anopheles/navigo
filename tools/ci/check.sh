#!/bin/sh
# The CI check steps (.golangci.yml for the linters and formatters, ADR
# 0005 for the licence list). Run from the repo root, with Go, and mpv for
# the player tests. GitHub Actions runs this in the check job;
# tools/ci/local.sh runs it in the cached CI image.
set -eu

tools/ci/install-golangci-lint.sh
golangci-lint fmt --diff
golangci-lint run ./...
golangci-lint run --build-tags e2e ./e2e/...
go vet ./...
go vet -tags e2e ./e2e
go test ./...
command -v go-licenses >/dev/null || go install github.com/google/go-licenses/v2@v2.0.1
go-licenses check ./... --allowed_licenses=MIT,BSD-2-Clause,BSD-3-Clause,Apache-2.0,ISC,MPL-2.0
