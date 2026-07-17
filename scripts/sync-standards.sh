#!/usr/bin/env bash
# Copy the canonical docs/site standards into src/ so they can be embedded via
# //go:embed. The Go module root is src/ and docs/site/ sits above it, so embed
# cannot reach the canonical files directly — this copy step bridges the gap
# (Constitution VI: thin justfile, logic in scripts/). The copies are committed
# so a clean `go build ./...` (which does not run this script) compiles; the
# drift-guard test in standards_test.go keeps them byte-honest.
set -euo pipefail

# Run from the repo root regardless of caller CWD.
cd "$(dirname "$0")/.."

SRC_DIR="docs/site/standards"
DEST_DIR="src/cmd/shll/standards"
STANDARDS=(principles help-dump readme-extraction skill)

mkdir -p "$DEST_DIR"
for name in "${STANDARDS[@]}"; do
    cp -f "${SRC_DIR}/${name}.md" "${DEST_DIR}/${name}.md"
done
echo "synced ${#STANDARDS[@]} standards: ${DEST_DIR}/{$(IFS=,; echo "${STANDARDS[*]}")}.md"
