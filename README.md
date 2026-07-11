<p align="center">
  <img src="assets/logo.svg" alt="Kapibara" width="440">
</p>

<p align="center">
  <b>No Docker, no drama, just <code>kapibara up</code>.</b><br>
  Self-hosted PaaS (a Vercel / Heroku / Dokploy alternative) on the
  <a href="https://github.com/orcinustools/orcinus"><b>Orcinus</b></a> cluster engine.
</p>

<p align="center">
  <a href="#install">Install</a> ·
  <a href="#quick-start">Quick start</a> ·
  <a href="#cli">CLI</a> ·
  <a href="#configuration">Configuration</a> ·
  <a href="./docs/SETUP.md">Setup guide</a> ·
  <a href="./docs/DEPLOY-GUIDE.md">Deploy guide</a>
</p>

---

Kapibara is the **control-plane** (REST API + embedded web UI + CLI). It turns a
standard `docker-compose.yml` into a running, TLS-secured app on Kubernetes by
driving **Orcinus**, which converts compose (annotated with `x-orcinus-*` hints)
into Kubernetes objects. Push code from your laptop; the server builds and
deploys it — no Docker or Kubernetes knowledge required.

- **Backend** — Go, a single static binary (the UI is embedded).
- **Frontend** — React + TypeScript + Vite SPA, embedded into the binary (`pkg/webui/dist`).
- **Engine** — Orcinus over its HTTP API + direct cluster access (client-go) for logs/metrics/exec.
- **Store** — SQLite (pure-Go, for dev) or Postgres (for prod), via GORM.

## Features

- **Auth & multi-tenancy** — users, organizations, projects, memberships, RBAC, API tokens (`kap_…`), 2FA (TOTP).
- **Deploy from source, on the server** — `kapibara up` (upload a local directory) or `app deploy --repo <git>` build in-cluster with **Railpack** (auto-detect) or a **Dockerfile** via BuildKit — **no Docker on the client or the server** — then push to the built-in registry and deploy.
- **Deploy Docker Compose** natively to the cluster.
- **Prebuilt images** — reference any public image, or build & push your own with `kapibara image build` (Docker) / `image pack` (no Docker).
- **Domains + TLS** — automatic Let's Encrypt via cert-manager (`x-orcinus-expose/host/tls`); apps default to `<name>.<apps-domain>`.
- **Env & secrets** — sensitive keys rendered as Kubernetes Secrets (`x-orcinus-secret`); import from `--env-file`.
- **One-click databases** — Postgres / MySQL / MariaDB / MongoDB / Redis (StatefulSet + PVC + Secret) with a ready-to-use connection string.
- **Runtime config** — resource limits/requests, command override, persistent volume mounts (PVC), ingress path, exec health checks.
- **Day-2 ops** — streaming logs, CPU/RAM metrics, scale, autoscale (HPA), rollback, progressive delivery (canary / blue-green).
- **More** — one-click templates, push-to-deploy webhooks, scheduled DB backups (local / S3), notifications (Slack/Discord/Telegram/email), multi-node, per-branch preview deployments, audit log.

## Install

```bash
# Release binary (linux/darwin, amd64/arm64) — checksum-verified:
curl -fsSL https://raw.githubusercontent.com/orcinustools/kapibara/main/install.sh | sh

# Or via Go (UI is embedded, no Node needed):
go install github.com/orcinustools/kapibara/cmd/kapibara@latest
```

Update an installed binary in place (checksum-verified):

```bash
kapibara update              # self-update to the latest release
kapibara update --check      # only report whether an update is available
kapibara update --path /usr/local/bin/kapibara --version v0.5.3
```

## Quick start

Deploy an app straight from a local directory — the server builds and runs it:

```bash
kapibara login --server https://kapibara.example.com
kapibara projects create shop
kapibara up --project shop --name web           # build (railpack) + deploy
# → https://web.apps.example.com  (auto domain + TLS, port 3000)
```

`up` packs the current directory (honoring `.dockerignore`/`.gitignore`), uploads
it, and streams the server-side build + deploy log. See the
[Deploy guide](./docs/DEPLOY-GUIDE.md) for databases, secrets, and more.

## Build & run from source

```bash
make build            # → bin/kapibara (UI embedded; needs Node+npm for the UI)
make test             # unit + integration tests

# Run the control-plane (needs an orcinus API + cluster):
orcinus api --addr :8899 --token "$TOKEN"        # the engine
KAPIBARA_ORCINUS_URL=http://localhost:8899 \
KAPIBARA_ORCINUS_TOKEN=$TOKEN \
KAPIBARA_JWT_SECRET=$(openssl rand -hex 16) \
KAPIBARA_KUBECONFIG=~/.orcinus/kubeconfig \
KAPIBARA_CLUSTER_CONTAINER=orcinus \
  bin/kapibara serve                              # → http://localhost:9000
```

Open `http://localhost:9000` and register the first user (becomes the admin).
For a full production install (wildcard DNS, TLS, deploying Kapibara itself onto
the cluster), see [`docs/SETUP.md`](./docs/SETUP.md) and [`deploy/`](./deploy/).

## CLI

The same binary is also a REST client — deploy from your laptop to any Kapibara
server without opening the UI:

```bash
kapibara login --server https://kapibara.example.com
kapibara up --project shop --name api --build railpack \
  --path . --env-file .env --mount data:/var/lib/data --volume-size 2Gi
kapibara app deploy --project shop --name web --repo https://github.com/you/web
kapibara deploy --project shop -f docker-compose.yml     # deploy a compose file
kapibara db create --project shop --name db --engine postgres --deploy
kapibara deployment list --project shop                  # deploy history
kapibara deployment redeploy <deploymentId>              # roll back to a snapshot
```

The `login` token + server are saved to `~/.kapibara/cli.json` (override with
`KAPIBARA_URL` / `KAPIBARA_TOKEN`, or point at a throwaway file via
`KAPIBARA_CLI_CONFIG`).

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `KAPIBARA_ADDR` | `:9000` | Listen address |
| `KAPIBARA_DATABASE_URL` | `~/.kapibara/kapibara.db` | SQLite path or `postgres://…` |
| `KAPIBARA_JWT_SECRET` | (random) | Signs sessions / API tokens |
| `KAPIBARA_ORCINUS_URL` | `http://localhost:8899` | Orcinus API |
| `KAPIBARA_ORCINUS_TOKEN` | — | Orcinus bearer token |
| `KAPIBARA_KUBECONFIG` | `~/.orcinus/kubeconfig` | Cluster access (logs/metrics/backup) |
| `KAPIBARA_APPS_DOMAIN` | — | Base wildcard domain → `<app>.<apps-domain>` |
| `KAPIBARA_CLUSTER_CONTAINER` | `orcinus` | k3s node (import images without a registry) |
| `KAPIBARA_REGISTRY_UPSTREAM` / `KAPIBARA_REGISTRY_PUBLIC` | — | Built-in registry gateway |
| `KAPIBARA_INCLUSTER_BUILD` / `KAPIBARA_BUILDKIT_ADDR` | — | Server-side builds (BuildKit); see [`deploy/`](./deploy/) |

## API

REST under `/api/v1`. Auth: `Authorization: Bearer <session-jwt | kap_token>`.
Main resources: `auth/*`, `orgs`, `projects`, `.../deploy`, `.../apps`,
`.../databases`, `deployments/{id}/redeploy`, `.../logs`, `.../metrics`,
`.../services/{svc}/scale|rollback`, `templates`, `.../backups`,
`orgs/{id}/notifications`, `nodes`, `audit`, `webhooks/{secret}`.

## Documentation

- [`docs/SETUP.md`](./docs/SETUP.md) — install & run Kapibara, wildcard DNS, TLS.
- [`docs/DEPLOY-GUIDE.md`](./docs/DEPLOY-GUIDE.md) — secrets, one-click databases, deploying apps (image/Git/local/compose) with a domain, wiring app ↔ DB.
- [`deploy/`](./deploy/) — deploy **Kapibara itself** onto an Orcinus cluster (compose + RBAC + `Dockerfile` + optional BuildKit) with a domain and TLS.
- **Agent skill** — `kapibara skill` prints the `orcinus.yml` authoring guide; `--example` writes a starter, `--write <path>` installs it. Source: [`cmd/kapibara/skill.md`](./cmd/kapibara/skill.md).

## Releases

Push a `vX.Y.Z` tag → GitHub Actions ([`.github/workflows/release.yml`](./.github/workflows/release.yml))
builds cross-platform binaries with GoReleaser and publishes checksummed archives.

## License

MIT.
