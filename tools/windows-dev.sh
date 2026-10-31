#!/bin/sh
# Builds this checkout for the Windows side of WSL, as navigo-dev.exe next to
# navigo.exe. A running navigo-dev.exe can't be overwritten or deleted, but it
# can be renamed, so the old one moves aside and goes on a later run.
set -eu

windows=/mnt/c/Windows/System32
appdata=$(cd /mnt/c && "$windows/cmd.exe" /c 'echo %LOCALAPPDATA%' 2>/dev/null | tr -d '\r')
dir=$(wslpath "$appdata")/Programs/navigo
exe=$dir/navigo-dev.exe

mkdir -p "$dir"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "$exe.new" ./cmd/navigo
rm -f "$exe".old.* 2>/dev/null || true
[ ! -e "$exe" ] || mv "$exe" "$exe.old.$$"
mv "$exe.new" "$exe"
"$exe" --version | tr -d '\r'
echo "built $exe"
