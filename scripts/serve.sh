#!/usr/bin/env bash
# Build (unless SKIP_BUILD=1) and run `kapibara serve` with the dev/e2e env.
#
#   scripts/serve.sh              # make build, then serve on :9000
#   SKIP_BUILD=1 scripts/serve.sh # serve the existing bin/kapibara as-is
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
# shellcheck source=env.sh
source "$ROOT/scripts/env.sh"

if [[ "${SKIP_BUILD:-0}" != "1" ]]; then
  echo "[serve] make build (set SKIP_BUILD=1 to skip)…"
  make build
fi

echo "[serve] starting kapibara serve → ${KAPIBARA_URL} (engine ${KAPIBARA_ORCINUS_URL})"
exec ./bin/kapibara serve
