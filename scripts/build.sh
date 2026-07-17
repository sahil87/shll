#!/usr/bin/env bash
set -euo pipefail

VERSION="$(git describe --tags --always 2>/dev/null || echo dev)"

# Refresh the embedded standards copies from their canonical docs/site sources
# before building (Constitution VI: thin justfile, logic in scripts/). The copies
# are committed, so this only re-syncs any drift; the drift-guard test enforces it.
./scripts/sync-standards.sh

mkdir -p bin
cd src
go build -ldflags "-X main.version=${VERSION}" -o ../bin/shll ./cmd/shll
echo "built: bin/shll (version: ${VERSION})"
