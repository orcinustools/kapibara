#!/usr/bin/env bash
# Shared dev/e2e environment for kapibara.
#
# Source this from other scripts, or in your shell:  source scripts/env.sh
# Every value is overridable from the outer environment, e.g.:
#   KAPIBARA_DATA_DIR=/tmp/foo source scripts/env.sh
#
# NOTE: the token/secret below are DEV-ONLY defaults matching the local
# orcinus dev cluster. Override them for anything that isn't local testing.

export KAPIBARA_ORCINUS_URL="${KAPIBARA_ORCINUS_URL:-http://localhost:8899}"
export KAPIBARA_ORCINUS_TOKEN="${KAPIBARA_ORCINUS_TOKEN:-kapibara-dev-token}"
export KAPIBARA_JWT_SECRET="${KAPIBARA_JWT_SECRET:-dev-secret}"
export KAPIBARA_CLUSTER_CONTAINER="${KAPIBARA_CLUSTER_CONTAINER:-orcinus}"
export KAPIBARA_KUBECONFIG="${KAPIBARA_KUBECONFIG:-$HOME/.orcinus/kubeconfig}"
export KAPIBARA_DATA_DIR="${KAPIBARA_DATA_DIR:-/tmp/kapibara-data}"
export KAPIBARA_ACME_EMAIL="${KAPIBARA_ACME_EMAIL:-ibnu@biznetgio.com}"

# Where the browser e2e (web/e2e.mjs) points, and the account it logs in as.
export KAPIBARA_URL="${KAPIBARA_URL:-http://localhost:9000}"
export E2E_EMAIL="${E2E_EMAIL:-ibnu@biznetgio.com}"
export E2E_PASS="${E2E_PASS:-supersecret}"
