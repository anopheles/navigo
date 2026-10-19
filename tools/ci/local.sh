#!/bin/sh
# Runs the CI gate locally: the check job, then the CGO_ENABLED=0 build
# matrix, in a clean copy of the repo inside the CI image. The image and
# the Go module and build caches are kept between runs.
set -eu

cd "$(dirname "$0")/../.."
docker build -q -t navigo-ci tools/ci >/dev/null
docker run --rm \
	-v "$PWD":/src:ro \
	-v navigo-ci-gomod:/go/pkg/mod \
	-v navigo-ci-gocache:/root/.cache/go-build \
	navigo-ci sh -euc '
		cp -r /src /w && cd /w && rm -f .env.e2e
		tools/ci/check.sh
		for os in linux darwin windows; do for arch in amd64 arm64; do
			CGO_ENABLED=0 GOOS=$os GOARCH=$arch go vet ./...
			CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -o /dev/null ./cmd/navigo
		done; done
		echo "gate: green"'
