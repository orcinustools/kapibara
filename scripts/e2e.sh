#!/usr/bin/env bash
# One-shot browser e2e for the kapibara SPA.
#
# Does the whole dance the manual recipe used to do by hand:
#   1. preflight  — orcinus engine must answer /healthz
#   2. server     — start kapibara on :9000 against a FRESH data dir
#   3. account    — register the e2e user
#   4. run        — web/e2e.mjs (Playwright + Chromium): login → project →
#                   compose deploy → pod visible in Overview
#   5. teardown   — stop the server, drop the temp data dir
#
# A fresh data dir per run keeps things deterministic: web/e2e.mjs hardcodes the
# project name "ui-e2e" and is NOT idempotent, so a reused store accumulates
# duplicate projects and the locator matches two of them.
#
# Usage:
#   scripts/e2e.sh              # build if bin/kapibara is missing, run e2e
#   SKIP_BUILD=1 scripts/e2e.sh # use the existing bin/kapibara as-is
#   REUSE_SERVER=1 scripts/e2e.sh   # reuse a kapibara already on :9000 (state not reset)
#
# Assumes the orcinus dev cluster + API are already up (see scripts/README.md).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
# Fresh, isolated data dir by default so re-runs start clean (overridable).
export KAPIBARA_DATA_DIR="${KAPIBARA_DATA_DIR:-$(mktemp -d /tmp/kapibara-e2e.XXXXXX)}"
# shellcheck source=env.sh
source "$ROOT/scripts/env.sh"

log(){ echo "[e2e] $*"; }
die(){ echo "[e2e] ERROR: $*" >&2; exit 1; }

# 1. Preflight: orcinus engine reachable.
curl -sf "${KAPIBARA_ORCINUS_URL}/healthz" >/dev/null \
  || die "orcinus API not reachable at ${KAPIBARA_ORCINUS_URL} — start it first (see scripts/README.md)"
log "orcinus engine healthy ✓"

# 2. Server.
STARTED_PID=""
TMP_DATA=""
SERVE_LOG="${SERVE_LOG:-/tmp/kapibara-serve.log}"
cleanup(){
  [[ -n "$STARTED_PID" ]] && kill "$STARTED_PID" 2>/dev/null || true
  [[ -n "$TMP_DATA"    ]] && rm -rf "$TMP_DATA"   2>/dev/null || true
}
trap cleanup EXIT

if [[ "${REUSE_SERVER:-0}" == "1" ]] && curl -sf "${KAPIBARA_URL}/healthz" >/dev/null 2>&1; then
  log "reusing kapibara already running at ${KAPIBARA_URL} (state not reset) ✓"
else
  # Free :9000 with fuser (NOT pkill — pkill -f 'kapibara serve' also matches
  # the launching shell and kills this script).
  fuser -k 9000/tcp >/dev/null 2>&1 || true
  sleep 1
  [[ "$KAPIBARA_DATA_DIR" == /tmp/kapibara-e2e.* ]] && TMP_DATA="$KAPIBARA_DATA_DIR"
  log "starting kapibara (data dir ${KAPIBARA_DATA_DIR}, log ${SERVE_LOG})…"
  SKIP_BUILD="${SKIP_BUILD:-1}" "$ROOT/scripts/serve.sh" >"$SERVE_LOG" 2>&1 &
  STARTED_PID=$!
  for _ in $(seq 1 60); do
    curl -sf "${KAPIBARA_URL}/healthz" >/dev/null 2>&1 && break
    sleep 0.5
  done
  curl -sf "${KAPIBARA_URL}/healthz" >/dev/null 2>&1 \
    || { tail -20 "$SERVE_LOG" >&2; die "kapibara did not come up on ${KAPIBARA_URL}"; }
  log "kapibara up (pid ${STARTED_PID}) ✓"
fi

# 3. Register the e2e account (409 = already exists = fine when reusing).
log "ensuring e2e account ${E2E_EMAIL}…"
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${KAPIBARA_URL}/api/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${E2E_EMAIL}\",\"password\":\"${E2E_PASS}\"}")
case "$code" in
  200|201) log "account created ✓" ;;
  409)     log "account already exists ✓" ;;
  *)       die "unexpected register status: HTTP ${code}" ;;
esac

# 4. Run the Playwright flow.
log "running web/e2e.mjs…"
( cd web && node e2e.mjs )
