# Skill: Deploy an app on Kapibara

Use this when a user asks to deploy an app on **Kapibara** (which runs on the
**orcinus** cluster engine). There are two ways — **prefer the first**:

1. **`kapibara up`** (default) — deploy a single app straight from **source** (a
   local directory or a Git repo). The **server** builds it (Railpack auto-detect
   or a Dockerfile) and deploys it. No Dockerfile, no manifest, no Docker, no
   `kubectl`.
2. **`orcinus.yml`** (docker-compose) — only for **multi-service** stacks, wiring
   **managed databases**, or **prebuilt/public** images.

## Default path: `kapibara up`

1. Inspect the project: language/stack, the **port** it listens on, the **env**
   it needs, and whether it needs a database, cache, or persistent volume.
2. Create any databases first (see "Managed databases"), then, from the app dir:

   ```bash
   kapibara up --project <p> --name <app>
   ```

   That minimal form already: builds with **Railpack** (auto-detects Node, Go,
   Python, Ruby, PHP, …), exposes it at **`<name>.<appsDomain>` with TLS**, and
   defaults the **port to 3000** (also set as `$PORT`). Add flags as needed:

   ```bash
   kapibara up --project <p> --name <app> --build railpack \
     --path . --port <port> --env-file .env --env KEY=VALUE --secret KEY \
     --mount data:/var/lib/app --volume-size 1Gi --domain <host> --tls
   ```

   It packs the directory (honors `.dockerignore`, else `.gitignore`; always
   excludes `.git`), uploads it, and **streams the server-side build + deploy
   log**. Follow later with `kapibara deployment status <id>`.

Notes:

- `--build railpack` (default) needs no Dockerfile. `--build dockerfile` uses the
  repo's Dockerfile (`--dockerfile <path>`, `--context-dir <subdir>` for a
  subfolder).
- **Git instead of local source**: same flags, but `kapibara app deploy --repo
  <git-url> --build railpack …` (the server clones and builds).
- **Monorepos** with several apps: deploy each app separately with
  `--context-dir apps/<name>` — do **not** `up` the repo root (Railpack finds no
  single start command and the container will crash-loop).
- **Databases**: create them first and pass the connection string via `--env` /
  `--env-file` (see "Managed databases").
- Requires the server to have **in-cluster builds** enabled (a BuildKit daemon).
  If it doesn't, either build an image yourself and push it (see "Images & the
  registry"), or use `orcinus.yml` with a prebuilt image. Check with
  `kapibara info`.

## When to use `orcinus.yml` instead

Author a compose file when the deploy is **more than one service** (e.g.
frontend + API + worker together), uses **prebuilt/public images**, or you want a
committed declarative manifest. Then:

```bash
kapibara deploy --project <name> -f orcinus.yml     # async; streams pod status
```

`orcinus.yml` is a standard **docker-compose** file annotated with `x-orcinus-*`
hints that orcinus converts to Kubernetes objects.

### Rules that matter

- One `services:` entry per container. Standard compose keys work: `image`,
  `ports`, `environment`, `command`, `volumes`, `healthcheck`,
  `deploy.replicas`, `deploy.resources`.
- **A service must declare `ports:` to get a Service** — without it there is no
  in-cluster DNS name, so databases/backends become unreachable. Always list the
  port a peer connects to.
- Services reach each other by **service name** (e.g. `db:5432`, `redis:6379`).
- Expose to the internet with `x-orcinus-expose: ingress` + `x-orcinus-host` +
  `x-orcinus-tls: letsencrypt`. The host must resolve to the cluster and
  ports 80/443 must be reachable for the ACME HTTP-01 challenge. Two public
  services (e.g. frontend + API) each get their own host. If the user gives no
  host, derive `<service>.<appsDomain>` (`kapibara info` shows the apps domain).
- Put sensitive env keys in `x-orcinus-secret` so they render as a Kubernetes
  Secret instead of plain env. Never hard-code long-lived credentials.
- Named volumes become a PVC sized by `x-orcinus-volume-size`; stateful services
  use `x-orcinus-controller: statefulset`.

### `x-orcinus-*` hints (per service)

| Key | Purpose |
|---|---|
| `x-orcinus-expose` | `ingress` \| `nodeport` \| `loadbalancer` \| `clusterip` |
| `x-orcinus-host` | Ingress host, e.g. `app.example.com` (comma-separate for several) |
| `x-orcinus-path` | Ingress path prefix (default `/`) |
| `x-orcinus-port` | Backend service port (defaults from `ports:`) |
| `x-orcinus-tls` | cert-manager ClusterIssuer name (`letsencrypt`) → HTTPS cert |
| `x-orcinus-secret` | List of env keys to store as a Secret |
| `x-orcinus-controller` | `deployment` (default) \| `statefulset` \| `daemonset` |
| `x-orcinus-volume-size` | PVC size for named volumes (e.g. `5Gi`) |
| `x-orcinus-autoscale-min` / `-max` / `-cpu` / `-memory` | Horizontal autoscaler |
| `x-orcinus-rollout` | Progressive delivery (`canary` \| `blue-green`) |
| `x-orcinus-node-selector` | Node placement label |
| `x-orcinus-image-pull-secret` | Pull secret name for private images |
| `x-orcinus-ingress-class` | Ingress class (default cluster ingress) |

### Template — frontend + API + managed datastores

```yaml
services:
  api:
    image: registry/shop/api:1        # built & pushed to the Kapibara registry
    ports: ["3000"]
    x-orcinus-expose: ingress
    x-orcinus-host: api.apps.example.com
    x-orcinus-tls: letsencrypt
    environment:
      NODE_ENV: production
      PORT: "3000"
      DATABASE_URL: postgresql://app:secret@db:5432/app  # managed db "db"
      REDIS_URL: redis://redis:6379                      # managed db "redis"
    x-orcinus-secret: [DATABASE_URL]
    deploy:
      resources:
        limits: { cpus: "0.5", memory: 512M }

  web:
    image: registry/shop/web:1
    ports: ["3000"]
    x-orcinus-expose: ingress
    x-orcinus-host: shop.apps.example.com
    x-orcinus-tls: letsencrypt
    environment:
      ORIGIN: https://shop.apps.example.com
```

(`db` and `redis` are managed databases created with `kapibara db`, not compose
services — that is why they are not declared here.)

## Managed databases

Prefer **managed databases** over hand-rolled DB services. Create them, then
reference them by service name from the app (via `up`'s `--env`/`--env-file` or a
compose `environment:`):

```bash
kapibara db create --project <p> --name db    --engine postgres --deploy
kapibara db create --project <p> --name redis --engine redis    --deploy
kapibara db info <id>   # copy the connection string
```

```
DATABASE_URL=postgresql://<user>:<pass>@db:5432/app   # managed db named "db"
REDIS_URL=redis://redis:6379                          # managed db named "redis"
```

A backend that runs migrations on startup (e.g. Prisma) needs the DB reachable
at deploy time — deploy the databases **before** (or in the same project as) the
app.

## Images & the registry (only when the server can't build)

If in-cluster builds are enabled, skip this — `up` / `app deploy` build on the
server. Otherwise build the image where you are and push it to Kapibara's
registry gateway (the CLI handles the login), then reference the short
`registry/<project>/<image>:<tag>` form:

- **`kapibara image build`** — full Dockerfile, runs `RUN` steps. Needs Docker.
  ```bash
  kapibara image build --project worker --name api --tag v1 -f apps/api/Dockerfile .
  ```
- **`kapibara image pack`** — assemble a base image + a directory in-process
  (go-containerregistry), **no Docker**. For prebuilt artifacts (static sites, Go
  binaries). Builds `linux/amd64` from any host OS.
  ```bash
  kapibara image pack --project web --name site --tag v1 \
    --base nginx:alpine --dir ./dist --dest /usr/share/nginx/html --port 80
  ```

Both push to the gateway; reference `image: registry/<project>/<image>:<tag>` in
`orcinus.yml`. Kapibara rewrites it at deploy time to the org-scoped pull path
and the cluster pulls it back through the gateway — no pull secret needed.

## Troubleshoot (watch the streamed deploy log)

- **`CrashLoopBackOff`, exits immediately (exit 0)** → Railpack found no start
  command (often a monorepo root with no root `start` script). Point `up` at the
  actual app (`--context-dir apps/<name>`) or set `--command`.
- **`CrashLoopBackOff`** (other) → app started but exited: bad config, DB
  unreachable, failed migration. Check logs; confirm `DATABASE_URL`/host.
- **`ImagePullBackOff` / `ErrImagePull`** → image not pullable: referenced
  `registry/...` but never pushed, a typo, or a public image that doesn't exist.
- **Pod stuck `Pending`** → no schedulable node (resource/disk pressure) or a PVC
  that can't bind.
- **TLS not `Ready`** → the host must resolve to the cluster and :80 be reachable;
  check `orcinus kubectl get certificate`.

## Deploy checklist

- [ ] Prefer `kapibara up` (single app from source); use `orcinus.yml` only for multi-service / prebuilt-image stacks.
- [ ] The app's listen port matches `--port` (or `ports:`); it binds `$PORT` / `0.0.0.0`.
- [ ] Managed databases created **first**; connection strings passed as env (secrets marked).
- [ ] Public services get a host + TLS (auto `<name>.<appsDomain>`, or explicit).
- [ ] Monorepo: deploy each app with `--context-dir`, not the repo root.
- [ ] The DNS host resolves to the cluster (for TLS issuance).
