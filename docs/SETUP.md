# Kapibara Setup Guide

How to install and run Kapibara (the control-plane) on top of an `orcinus`
cluster, wire up a wildcard domain, and enable automatic TLS. Once this is done,
follow the [Deploy Guide](./DEPLOY-GUIDE.md) to ship apps and databases.

Throughout, replace `example.com` with your own domain.

---

## 1. Architecture

```
   users ──HTTPS──▶  ingress (traefik) ──▶  your apps + databases
                              ▲                     ▲
   kapibara (control-plane)   │ deploy (compose)    │ runs on
   REST API + web UI  ────────┴──▶ orcinus engine ──┘  k3s/Kubernetes
```

- **Kapibara** — the control-plane you interact with (web UI + REST API + CLI).
  It stores projects/apps/databases and turns them into compose sources.
- **orcinus** — the cluster engine. Kapibara sends it compose; orcinus renders
  Kubernetes objects and applies them to the cluster.
- Kapibara also talks to the Kubernetes API directly (via a kubeconfig) for
  logs and metrics.

Kapibara and orcinus can run on the same host (recommended) or separately.

---

## 2. Prerequisites

- A Linux host running an **orcinus** cluster (k3s), reachable on ports **80/443**
  for ingress, with **cert-manager** and an ingress controller (traefik) installed.
- **Docker** on the host if you want Git/Dockerfile builds (Kapibara runs
  `docker build` and imports the image into the cluster's containerd).
- A **kubeconfig** for the cluster (orcinus writes one, e.g. `~/.orcinus/kubeconfig`).
- A **domain** you control, with a **wildcard DNS record** (see §5).

---

## 3. Install

### Option A — install script (recommended)

Downloads the latest release binary for your OS/arch (linux/darwin, amd64/arm64),
verifies its checksum, and installs to `/usr/local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/orcinustools/kapibara/main/install.sh | sh
```

Overrides: `KAPIBARA_VERSION=v0.1.0` (pin a tag), `KAPIBARA_INSTALL=$HOME/.local/bin`
(install dir). Then: `kapibara version`.

### Option B — `go install`

```bash
go install github.com/orcinustools/kapibara/cmd/kapibara@latest
```

The web UI is embedded (committed under `pkg/webui/dist`), so no Node toolchain
is needed for this path.

### Option C — build from source

```bash
git clone https://github.com/orcinustools/kapibara.git
cd kapibara
make build          # builds the web UI, embeds it, then compiles the binary
./bin/kapibara version
```

---

## 4. Configure

Kapibara is configured entirely through environment variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| `KAPIBARA_ADDR` | `:9000` | HTTP listen address for the API + UI |
| `KAPIBARA_DATABASE_URL` | `<data-dir>/kapibara.db` | SQLite path or `postgres://…` for the control-plane store |
| `KAPIBARA_JWT_SECRET` | (auto, dev only) | Signs session/API tokens — **set a strong value in production** |
| `KAPIBARA_ORCINUS_URL` | `http://localhost:8899` | orcinus engine API base URL |
| `KAPIBARA_ORCINUS_TOKEN` | — | Bearer token for the orcinus API |
| `KAPIBARA_KUBECONFIG` | `~/.orcinus/kubeconfig` | Direct cluster access (logs, metrics) |
| `KAPIBARA_NAMESPACE` | `default` | Namespace orcinus deploys into |
| `KAPIBARA_ACME_EMAIL` | — | Email for Let's Encrypt; enables real TLS issuance |
| `KAPIBARA_CLUSTER_CONTAINER` | `orcinus` | Docker container name of the k3s node (for importing built images) |
| `KAPIBARA_REGISTRY` | — | If set, built images are pushed here instead of imported |
| `KAPIBARA_DATA_DIR` | `~/.kapibara` | Server-local state (SQLite, build cache) |

> Logs/metrics need a working `KAPIBARA_KUBECONFIG`; without it those endpoints
> return 503 but everything else (deploy, databases) still works.

---

## 5. DNS & the app domain

Apps get hostnames under a subdomain you dedicate to Kapibara, e.g.
`*.apps.example.com`. Point a **wildcard A record** at your server's public IP:

```
*.apps.example.com.   A   203.0.113.10
```

Verify:

```bash
dig +short test.apps.example.com     # → 203.0.113.10
```

Every app you expose then gets `https://<app>.apps.example.com`.

---

## 6. TLS (Let's Encrypt)

TLS is issued automatically by cert-manager via the HTTP-01 challenge. Requirements:

1. Start Kapibara with `KAPIBARA_ACME_EMAIL=you@example.com`.
2. The app host resolves publicly to the server (wildcard from §5).
3. Ports 80 and 443 reach the cluster's ingress.

When you deploy an app with a domain + TLS, cert-manager requests a certificate;
check readiness with `orcinus kubectl get certificate`.

---

## 7. Run

### Foreground (quick start)

```bash
export KAPIBARA_ORCINUS_URL=http://localhost:8899
export KAPIBARA_ORCINUS_TOKEN=your-orcinus-token
export KAPIBARA_KUBECONFIG=$HOME/.orcinus/kubeconfig
export KAPIBARA_ACME_EMAIL=you@example.com
export KAPIBARA_JWT_SECRET="$(openssl rand -hex 32)"
kapibara serve
# → kapibara <version> listening on :9000 (engine: http://localhost:8899)
```

### systemd (production)

```ini
# /etc/systemd/system/kapibara.service
[Unit]
Description=Kapibara control-plane
After=network-online.target
Wants=network-online.target

[Service]
User=kapibara
Environment=KAPIBARA_ADDR=:9000
Environment=KAPIBARA_ORCINUS_URL=http://localhost:8899
Environment=KAPIBARA_ORCINUS_TOKEN=your-orcinus-token
Environment=KAPIBARA_KUBECONFIG=/home/kapibara/.orcinus/kubeconfig
Environment=KAPIBARA_ACME_EMAIL=you@example.com
Environment=KAPIBARA_JWT_SECRET=CHANGE-ME
Environment=KAPIBARA_DATA_DIR=/var/lib/kapibara
ExecStart=/usr/local/bin/kapibara serve
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now kapibara
sudo systemctl status kapibara
```

Put Kapibara's own UI behind your ingress/reverse proxy on a host such as
`kapibara.example.com` if you want it publicly reachable.

---

## 8. First login

```bash
curl -s http://localhost:9000/healthz     # {"engineHealthy":true,"status":"ok"}
```

Open the UI (`http://localhost:9000`, or your proxied host) and register the
first account — the first user becomes the platform admin and gets a default
organization. Or via the CLI:

```bash
kapibara login --server http://localhost:9000 --email you@example.com --password '••••••'
```

---

## 9. Next steps

You're ready to deploy. See the **[Deploy Guide](./DEPLOY-GUIDE.md)** for:

- importing secrets (`kapibara secret put …`),
- one-click databases (Postgres, Redis, …),
- deploying apps from a prebuilt image, a **Git repo** (Dockerfile/Nixpacks), or
  a local Docker Compose file,
- exposing an app at `https://<app>.apps.example.com` with automatic TLS,
- wiring app → database connectivity.

---

## 10. Troubleshooting

- **`engineHealthy: false`** — Kapibara can't reach orcinus. Check
  `KAPIBARA_ORCINUS_URL`/`KAPIBARA_ORCINUS_TOKEN` and that orcinus is up.
- **`cluster access unavailable (no kubeconfig)`** on Logs/Metrics — set
  `KAPIBARA_KUBECONFIG` to a readable kubeconfig for the cluster.
- **Git build fails** — the host needs Docker, and `KAPIBARA_CLUSTER_CONTAINER`
  must match the k3s node's container name (default `orcinus`) so the built
  image can be imported. Alternatively set `KAPIBARA_REGISTRY` to push instead.
- **TLS never becomes ready** — confirm the host resolves to the server and
  :80 is reachable for the ACME HTTP-01 challenge; inspect `orcinus kubectl get certificate`.
