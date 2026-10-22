#!/bin/sh
# Records docs/media/demo.gif: a throwaway Navidrome serves the synthetic
# fixture library with generated covers, and VHS (in Docker, with mpv) runs
# navigo against it. Needs Docker and Go. Nothing is recorded from a
# real server.
#
#   tools/demo/demo.sh
set -eu

cd "$(dirname "$0")/../.."
work=$(mktemp -d)
music=${NAVIGO_DEMO_MUSIC:-${XDG_CACHE_HOME:-$HOME/.cache}/navigo-demo/music}
port=14534
compose="docker compose -p navigo-demo -f tools/navidrome/compose.yaml"
export NAVIGO_FIXTURE_MUSIC=$music NAVIGO_FIXTURE_PORT=$port

cleanup() {
	$compose down >/dev/null 2>&1 || true
	rm -rf "$work"
}
trap cleanup EXIT INT TERM

# The library is kept between runs; delete $music to generate it again.
[ -d "$music/.navigo-fixtures/covers" ] || go run ./tools/genlibrary -covers -out "$music"
CGO_ENABLED=0 go build -o "$work/navigo" ./cmd/navigo
docker build -q -t navigo-demo tools/demo >/dev/null

$compose up -d
printf 'waiting for Navidrome to scan'
i=0
until curl -fs "http://127.0.0.1:$port/rest/getScanStatus?u=admin&p=navigo-fixtures&v=1.16.1&c=demo&f=json" |
	grep -q '"scanning":false' &&
	curl -fs "http://127.0.0.1:$port/rest/getAlbumList2?u=admin&p=navigo-fixtures&v=1.16.1&c=demo&f=json&type=newest&size=1" |
	grep -q '"album"'; do
	i=$((i + 1))
	[ "$i" -lt 90 ] || { echo " timed out" >&2; exit 1; }
	printf .
	sleep 2
done
echo

mkdir -p docs/media
docker run --rm --network host -w /out \
	-v "$work/navigo:/usr/local/bin/navigo:ro" \
	-v "$PWD/tools/demo/config.yaml:/root/.config/navigo/config.yaml:ro" \
	-v "$PWD/tools/demo:/demo:ro" \
	-v "$PWD/docs/media:/out" \
	navigo-demo /demo/demo.tape
ls -l docs/media/demo.gif
