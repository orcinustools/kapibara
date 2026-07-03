# scripts/ — dev & e2e helpers

Small wrappers around the local dev/e2e recipe so setup is one command instead
of a wall of env vars.

| Script | What it does |
|--------|--------------|
| `env.sh`   | Shared `KAPIBARA_*` env (dev defaults, all overridable). Source it or let the others source it. |
| `serve.sh` | `make build` (skip with `SKIP_BUILD=1`) then `kapibara serve` on `:9000`. |
| `e2e.sh`   | Full browser e2e: preflight orcinus → ensure server up → register e2e user → run `web/e2e.mjs`. |

## Prerequisites

The orcinus dev cluster and its API must be running first:

- **k3s cluster**: docker container named `orcinus` (ports 80/443/6443), kubeconfig
  at `~/.orcinus/kubeconfig`. Docker socket must be accessible
  (`sudo chmod 666 /var/run/docker.sock`).
- **orcinus API** on `:8899`, started from the orcinus repo:
  ```bash
  orcinus/bin/orcinus api --addr :8899 --token kapibara-dev-token &
  ```
- Node + Playwright/Chromium are installed under `web/` (`cd web && npm install`).

## Usage

```bash
# Run the whole browser e2e (builds if bin/kapibara is missing):
scripts/e2e.sh

# Reuse the already-built binary:
SKIP_BUILD=1 scripts/e2e.sh

# Just run the server (e.g. to poke the UI by hand):
scripts/serve.sh
```

`e2e.sh` reuses a kapibara already listening on `:9000`; otherwise it starts one
in the background (log at `/tmp/kapibara-serve.log`) and stops it on exit.

## Overriding defaults

Any `KAPIBARA_*` / `E2E_*` var can be set in the environment before calling a
script, e.g. a clean data dir and a different account:

```bash
KAPIBARA_DATA_DIR=/tmp/kapi-fresh E2E_EMAIL=test@example.com E2E_PASS=hunter2 scripts/e2e.sh
```

> The token/secret in `env.sh` are **dev-only** defaults for the local cluster.
> Override them for anything that isn't local testing.
