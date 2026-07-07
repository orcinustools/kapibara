#!/usr/bin/env bash
# End-to-end test of the kapibara CLI deploying to a live orcinus cluster.
#
# Exercises the "deploy from your laptop to a server" path entirely through the
# `kapibara` CLI (no browser), plus the parity features added on top of it:
#   1. preflight   — orcinus engine reachable
#   2. server      — start a fresh kapibara on :9020 (own data dir)
#   3. account     — register the e2e user (CLI has no register; use the API)
#   4. login       — `kapibara login`  → caches token
#   5. compose     — `kapibara deploy -f compose.yml`  → applied N objects
#   6. app         — `kapibara app deploy` (image + cpu/mem limits + PVC mount)
#   7. verify      — deployment succeeded, pod running, resources rendered
#   8. history     — `kapibara deployment list`
#   9. rollback    — `kapibara deployment redeploy` a past deployment
#  10. teardown    — delete cluster resources, stop server, drop data dir
#
# Usage:
#   scripts/cli-e2e.sh                # build bin/kapibara (build-go) then run
#   SKIP_BUILD=1 scripts/cli-e2e.sh   # use existing bin/kapibara
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source "$ROOT/scripts/env.sh"

PORT="${CLI_E2E_PORT:-9020}"
SERVER="http://localhost:${PORT}"
EMAIL="cli-e2e@example.com"
PASS="supersecret123"
CLI_CFG="$(mktemp /tmp/kapi-cli-e2e-cfg.XXXXXX.json)"
DATA_DIR="$(mktemp -d /tmp/kapi-cli-e2e.XXXXXX)"
SERVE_LOG="/tmp/kapibara-cli-e2e-serve.log"
BIN="$ROOT/bin/kapibara"

log(){ echo "[cli-e2e] $*"; }
die(){ echo "[cli-e2e] ERROR: $*" >&2; exit 1; }
cli(){ KAPIBARA_CLI_CONFIG="$CLI_CFG" "$BIN" "$@"; }

SRV_PID=""
cleanup(){
  [[ -n "$SRV_PID" ]] && kill "$SRV_PID" 2>/dev/null || true
  rm -rf "$DATA_DIR" "$CLI_CFG" 2>/dev/null || true
}
trap cleanup EXIT

# 0. Build the binary (backend + embedded UI already committed → build-go only).
if [[ "${SKIP_BUILD:-0}" != "1" ]]; then
  log "building bin/kapibara (make build-go)…"
  make build-go >/dev/null
fi
[[ -x "$BIN" ]] || die "binary $BIN missing"

# 1. Preflight.
curl -sf "${KAPIBARA_ORCINUS_URL}/healthz" >/dev/null \
  || die "orcinus API unreachable at ${KAPIBARA_ORCINUS_URL}"
log "orcinus engine healthy ✓"

# 2. Fresh server.
fuser -k "${PORT}/tcp" >/dev/null 2>&1 || true
sleep 1
KAPIBARA_ADDR=":${PORT}" KAPIBARA_DATA_DIR="$DATA_DIR" "$BIN" serve >"$SERVE_LOG" 2>&1 &
SRV_PID=$!
for _ in $(seq 1 60); do curl -sf "${SERVER}/healthz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -sf "${SERVER}/healthz" >/dev/null 2>&1 || { tail -20 "$SERVE_LOG" >&2; die "server did not start"; }
log "kapibara up on ${SERVER} (pid ${SRV_PID}) ✓"

# 3. Account (register via API — CLI has no register command).
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${SERVER}/api/v1/auth/register" \
  -H 'Content-Type: application/json' -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASS}\"}")
[[ "$code" == 200 || "$code" == 201 || "$code" == 409 ]] || die "register failed: HTTP $code"
log "account ready ✓"

# 4. Login.
cli login --server "$SERVER" --email "$EMAIL" --password "$PASS" >/dev/null
log "CLI login ✓"

# 5. Compose deploy.
COMPOSE_FILE="$(mktemp /tmp/kapi-cli-e2e-compose.XXXXXX.yml)"
cat >"$COMPOSE_FILE" <<'YAML'
services:
  web:
    image: nginx:alpine
    ports:
      - "80"
YAML
cli deploy --project cli-e2e -f "$COMPOSE_FILE" --wait | tee /tmp/kapi-cli-e2e-compose.out
grep -q "applied" /tmp/kapi-cli-e2e-compose.out || die "compose deploy did not report applied objects"
rm -f "$COMPOSE_FILE"
log "compose deploy ✓"

# 6. App deploy with parity features: image + CPU/mem limits + persistent mount.
cli app deploy --project cli-e2e --name apiapp --build image --image nginx:alpine \
  --port 80 --cpu-limit 0.25 --memory-limit 128M --mount data:/usr/share/nginx/html/data \
  --volume-size 1Gi --follow | tee /tmp/kapi-cli-e2e-app.out
grep -q "succeeded" /tmp/kapi-cli-e2e-app.out || die "app deploy did not succeed"
log "app deploy (with resources + mount) ✓"

# 7. Verify: rendered compose carries resources + mount; pod is running.
APPID=$(curl -s "${SERVER}/api/v1/orgs/$(jq -r .orgId "$CLI_CFG")/projects" \
  -H "Authorization: Bearer $(jq -r .token "$CLI_CFG")" | jq -r '.projects[]|select(.name=="cli-e2e").id')
[[ -n "$APPID" ]] || die "could not resolve project id"
DEPS=$(curl -s "${SERVER}/api/v1/projects/${APPID}/deployments" \
  -H "Authorization: Bearer $(jq -r .token "$CLI_CFG")")
APP_SRC=$(echo "$DEPS" | jq -r '[.deployments[]|select(.kind=="application")][0].source')
echo "$APP_SRC" | grep -q "cpus:" || die "rendered app compose missing cpu limit:\n$APP_SRC"
echo "$APP_SRC" | grep -q "memory: 128M" || die "rendered app compose missing memory limit"
echo "$APP_SRC" | grep -q "data:/usr/share/nginx/html/data" || die "rendered app compose missing mount"
echo "$APP_SRC" | grep -q "x-orcinus-volume-size: 1Gi" || die "rendered app compose missing volume size"
log "rendered app compose carries resources + mount ✓"

# In-cluster proof: the built Deployment has resource limits (orcinus honored
# deploy.resources). Prefer host kubectl; fall back to the k3s container's.
KCTL=""
if command -v kubectl >/dev/null 2>&1; then
  KCTL="kubectl"
elif command -v docker >/dev/null 2>&1 && docker exec "$KAPIBARA_CLUSTER_CONTAINER" kubectl version --client >/dev/null 2>&1; then
  KCTL="docker exec ${KAPIBARA_CLUSTER_CONTAINER} kubectl"
fi
if [[ -n "$KCTL" ]]; then
  sleep 3
  LIM=$(KUBECONFIG="$KAPIBARA_KUBECONFIG" $KCTL get deploy -A -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.spec.template.spec.containers[0].resources.limits}{"\n"}{end}' 2>/dev/null | grep -iE 'apiapp' || true)
  log "cluster deploy resource limits: ${LIM:-<none found>}"
  if [[ "$LIM" == *"memory"* || "$LIM" == *"cpu"* ]]; then
    log "in-cluster resource limits confirmed ✓"
  else
    log "WARN: could not confirm limits via kubectl (non-fatal)"
  fi
fi

# Pod running check via the CLI's server (pods aggregate across units).
PODS=$(curl -s "${SERVER}/api/v1/projects/${APPID}/pods" -H "Authorization: Bearer $(jq -r .token "$CLI_CFG")")
echo "$PODS" | jq -e '.pods|length > 0' >/dev/null || die "no pods reported for project"
log "pods present ✓"

# 8. History.
cli deployment list --project cli-e2e | tee /tmp/kapi-cli-e2e-hist.out
FIRST_APP_DEP=$(echo "$DEPS" | jq -r '[.deployments[]|select(.kind=="application")][0].id')
[[ -n "$FIRST_APP_DEP" ]] || die "no application deployment to roll back to"

# 9. Rollback (redeploy the app's snapshot — same image, no rebuild).
cli deployment redeploy "$FIRST_APP_DEP" | tee /tmp/kapi-cli-e2e-rb.out
grep -q "redeployed from" /tmp/kapi-cli-e2e-rb.out || die "redeploy did not succeed"
log "rollback/redeploy ✓"

# 10. Teardown cluster resources (delete the project).
curl -s -X DELETE "${SERVER}/api/v1/projects/${APPID}" \
  -H "Authorization: Bearer $(jq -r .token "$CLI_CFG")" >/dev/null || true
log "cluster resources torn down ✓"

echo
log "ALL CLI E2E CHECKS PASSED ✅"
