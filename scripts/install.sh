#!/usr/bin/env bash
set -euo pipefail

./scripts/build.sh

DEST="${HOME}/.local/bin/shll"
mkdir -p "$(dirname "$DEST")"
cp -f ./bin/shll "$DEST"
echo "installed: $DEST"
