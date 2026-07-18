#!/usr/bin/env bash
# Copy the canonical docs/site embed sources into src/ so they can be embedded via
# //go:embed. The Go module root is src/ and docs/site/ sits above it, so embed
# cannot reach the canonical files directly — this copy step bridges the gap
# (Constitution VI: thin justfile, logic in scripts/). The copies are committed
# so a clean `go build ./...` (which does not run this script) compiles; the
# drift-guard tests in standards_test.go / skill_test.go keep them byte-honest.
set -euo pipefail

# Run from the repo root regardless of caller CWD.
cd "$(dirname "$0")/.."

# 1. The producer-facing standards documents (docs/site/standards/ → embed dir).
SRC_DIR="docs/site/standards"
DEST_DIR="src/cmd/shll/standards"
STANDARDS=(principles help-dump readme-extraction skill)

mkdir -p "$DEST_DIR"
for name in "${STANDARDS[@]}"; do
    cp -f "${SRC_DIR}/${name}.md" "${DEST_DIR}/${name}.md"
done
echo "synced ${#STANDARDS[@]} standards: ${DEST_DIR}/{$(IFS=,; echo "${STANDARDS[*]}")}.md"

# 2. shll's own agent skill bundle (docs/site/skill.md → embed dir). Served
#    in-process by `shll skill shll`; drift-guarded by TestSkillEmbedMatchesCanonical.
SKILL_SRC="docs/site/skill.md"
SKILL_DEST_DIR="src/cmd/shll/skill"
mkdir -p "$SKILL_DEST_DIR"
cp -f "$SKILL_SRC" "${SKILL_DEST_DIR}/skill.md"
echo "synced shll skill bundle: ${SKILL_DEST_DIR}/skill.md"
