#!/bin/sh
# Generates the synthetic fixture library, runs a throwaway Navidrome on it
# and records the JSON fixtures the fake Subsonic server replays.
#
#   tools/fixtures.sh up       generate the library and start Navidrome
#   tools/fixtures.sh record   seed playlists and stars, then record fixtures
#   tools/fixtures.sh down     stop Navidrome and drop its data
#
# Needs Docker and Go. The library goes to $NAVIGO_FIXTURE_MUSIC
# (default: the user cache dir), never into the repo.
set -eu

cd "$(dirname "$0")/.."
NAVIGO_FIXTURE_MUSIC=${NAVIGO_FIXTURE_MUSIC:-${XDG_CACHE_HOME:-$HOME/.cache}/navigo-fixtures/music}
NAVIGO_FIXTURE_PORT=${NAVIGO_FIXTURE_PORT:-14533}
export NAVIGO_FIXTURE_MUSIC NAVIGO_FIXTURE_PORT
compose="docker compose -f tools/navidrome/compose.yaml"

case ${1:-} in
up)
	go run ./tools/genlibrary -out "$NAVIGO_FIXTURE_MUSIC"
	$compose up -d
	;;
record)
	go run ./tools/recordfixtures -url "http://127.0.0.1:$NAVIGO_FIXTURE_PORT" \
		-out internal/subsonic/subsonictest/testdata
	;;
down)
	$compose down
	;;
*)
	echo "usage: $0 up|record|down" >&2
	exit 2
	;;
esac
