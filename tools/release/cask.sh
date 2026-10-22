#!/bin/sh
# Prints the Homebrew cask for a published release, e.g.
#   tools/release/cask.sh v0.1.0 > Casks/navigo.rb
# Needs gh, authenticated (GH_TOKEN in CI).
set -eu

tag=${1:?usage: cask.sh vX.Y.Z}
version=${tag#v}
cd "$(dirname "$0")/../.."

sums=$(gh release download "$tag" --repo anopheles/navigo --pattern checksums.txt --output -)
sha() {
	s=$(printf '%s\n' "$sums" | awk -v f="navigo_${version}_darwin_$1.tar.gz" '$2 == f { print $1 }')
	[ -n "$s" ] || { echo "cask.sh: no checksum for darwin_$1 in $tag" >&2; exit 1; }
	echo "$s"
}
arm64=$(sha arm64)
amd64=$(sha amd64)

sed -e "s/@VERSION@/$version/" \
	-e "s/@SHA256_ARM64@/$arm64/" \
	-e "s/@SHA256_AMD64@/$amd64/" \
	packaging/navigo.rb.tmpl
