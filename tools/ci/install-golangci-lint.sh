#!/bin/sh
# Installs the pinned golangci-lint into $(go env GOPATH)/bin unless that
# version is already on the PATH. The CI image bakes it in; check.sh calls
# this so a bare golang container gets it too.
set -eu

version=v2.15.0
golangci-lint version 2>/dev/null | grep -q "version ${version#v} " && exit 0
curl -sSfL "https://raw.githubusercontent.com/golangci/golangci-lint/$version/install.sh" |
	sh -s -- -b "$(go env GOPATH)/bin" "$version"
