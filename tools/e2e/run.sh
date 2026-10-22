#!/bin/sh
# Runs the layer B e2e suite against a throwaway Navidrome serving the
# synthetic fixture library, as the CI e2e job does. navigo and mpv run in
# the cached CI image; Navidrome runs beside it on the host network.
#
# Needs Docker and Go on the host. To run
# against the live server instead: go test -tags e2e ./e2e (reads .env.e2e).
set -eu

cd "$(dirname "$0")/../.."
port=${NAVIGO_FIXTURE_PORT:-14533}
trap 'tools/fixtures.sh down >/dev/null 2>&1 || true' EXIT

tools/fixtures.sh up
go run ./tools/recordfixtures -seed-only -url "http://127.0.0.1:$port"
docker build -q -t navigo-ci tools/ci >/dev/null
docker run --rm --network host \
	-v "$PWD":/src:ro \
	-v navigo-ci-gomod:/go/pkg/mod \
	-v navigo-ci-gocache:/root/.cache/go-build \
	-e NAVIGO_E2E_URL="http://127.0.0.1:$port" \
	-e NAVIGO_E2E_USER=admin \
	-e NAVIGO_E2E_PASSWORD=navigo-fixtures \
	navigo-ci sh -euc 'cp -r /src /w && cd /w && rm -f .env.e2e && go test -tags e2e -count=1 -v ./e2e'
