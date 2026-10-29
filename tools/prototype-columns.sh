#!/bin/sh
# PROTOTYPE (#111, throwaway): runs navigo with the column layouts against
# the fixture Navidrome. F2 cycles the layouts; NAVIGO_COLUMNS=A..E picks
# the first. The fixture server stays up; `make fixtures-down` stops it.
set -eu
cd "$(dirname "$0")/.."
port=${NAVIGO_FIXTURE_PORT:-14533}
curl -fs "http://127.0.0.1:$port/rest/ping?u=admin&p=navigo-fixtures&v=1.16.1&c=proto&f=json" >/dev/null 2>&1 ||
	tools/fixtures.sh up
cfg=$(mktemp -d)
trap 'rm -rf "$cfg"' EXIT
cat >"$cfg/config.yaml" <<YAML
server:
  url: http://127.0.0.1:$port
  user: admin
  password: navigo-fixtures
player:
  mpv_args: ["--ao=null"]
YAML
# waits for the scan, then stars and playlists like the fixtures have
go run ./tools/recordfixtures -url "http://127.0.0.1:$port" -seed-only
go run ./cmd/navigo -config "$cfg/config.yaml"
