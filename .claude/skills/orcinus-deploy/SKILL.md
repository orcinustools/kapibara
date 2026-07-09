# Skill: Author an `orcinus.yml` deployment file

Use this when a user asks to deploy an app on **Kapibara / orcinus**. Produce an
`orcinus.yml` — a standard **docker-compose** file annotated with `x-orcinus-*`
hints that orcinus converts to Kubernetes objects — then deploy it.

## How to use

1. Inspect the project: language, how it starts, the port it listens on, and
   whether it needs a database, cache, or persistent storage.
2. Decide each service's image (see "Images & the registry" below):
   - **Public image** → reference it directly (`nginx:alpine`, `postgres:16`).
   - **Your own code** → build it, push to the Kapibara registry, and reference
     `registry/<project>/<image>:tag`.
3. Prefer **managed databases** over hand-rolled DB services (see below).
4. Write `orcinus.yml` (see the template), then deploy:
   ```bash
   kapibara deploy --project <name> -f orcinus.yml     # streams the deploy log
   ```
   The deploy is **asynchronous**: Kapibara applies it, then streams pod
   readiness until ready or timeout. Follow later with
   `kapibara deployment status <id>`.

## Default app domain

If the user does not specify a host, derive one from the server's base apps
domain: **`<app-name>.<appsDomain>`**. Discover it with `kapibara info`
(`apps domain: apps.example.com` → `<app>.apps.example.com`). Only expose
(ingress + host + TLS) services meant to be public; leave internal services
without a host.

## Rules that matter

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
  services (e.g. frontend + API) each get their own host.
- Put sensitive env keys in `x-orcinus-secret` so they render as a Kubernetes
  Secret instead of plain env. Never hard-code long-lived credentials you intend
  to keep — mark them secret and rotate anything that leaked into a manifest.
- Named volumes become a PVC sized by `x-orcinus-volume-size`; stateful services
  use `x-orcinus-controller: statefulset`.

## `x-orcinus-*` hints (per service)

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

## Images & the registry (build → push → reference)

Kapibara runs **in-cluster** and **cannot build from Git** itself. Build the
image where you are and push it to Kapibara's registry gateway with one CLI
command (it handles the registry login), then reference the short
`registry/<project>/<image>:<tag>` form in the manifest.

Two build modes:

- **`kapibara image build`** — full Dockerfile, runs `RUN` steps. Needs Docker
  on this machine. Monorepos: context is the repo root, `-f` picks the service
  Dockerfile.
  ```bash
  kapibara image build --project worker --name api --tag v1 -f apps/api/Dockerfile .
  ```
- **`kapibara image pack`** — assemble a base image + a directory, in-process
  (go-containerregistry), **no Docker**. For prebuilt artifacts: static sites,
  Go binaries, compiled bundles (no `RUN`). Produces a `linux/amd64` image from
  any host OS.
  ```bash
  kapibara image pack --project web --name site --tag v1 \
    --base nginx:alpine --dir ./dist --dest /usr/share/nginx/html --port 80
  ```

Both push to the gateway and print the reference to use in `orcinus.yml`:

```yaml
    image: registry/<project>/<image>:<tag>
```

Kapibara rewrites that at deploy time to the full, org-scoped pull path
(`<host>/registry/<org>/<project>/<image>:<tag>`) and the cluster pulls it back
through the gateway — no pull secret needed. (Raw `docker push
<host>/registry/...` also works if you can't use the CLI; `kapibara info` shows
the host.)

## Reuse existing managed databases

If the project already has managed databases (created with `kapibara db create`),
**do not declare DB services in the compose** — reference them by their service
name and use the connection string from `kapibara db info <id>`:

```yaml
    environment:
      DATABASE_URL: postgresql://<user>:<pass>@db:5432/app   # managed db named "db"
      REDIS_URL: redis://redis:6379                          # managed db named "redis"
    x-orcinus-secret: [DATABASE_URL]
```

Create them first if needed:

```bash
kapibara db create --project <p> --name db    --engine postgres --deploy
kapibara db create --project <p> --name redis --engine redis    --deploy
kapibara db info <id>   # copy the connection string
```

A backend that runs migrations on startup (e.g. Prisma) needs the DB reachable
at deploy time — deploy the databases before (or in the same project as) the app.

## Template — frontend + API + managed datastores

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

## Deploy & troubleshoot

```bash
kapibara deploy --project shop -f orcinus.yml        # async; streams pod status
kapibara deployment status <id>                      # re-follow a deploy's log
```

Watch the streamed log for pod status:

- **`ImagePullBackOff` / `ErrImagePull`** → the image isn't pullable. Common
  causes: you referenced `registry/...` but never pushed it; a typo in the
  project/image/tag; or a public image name that doesn't exist. Verify with a
  `docker pull` of the resolved path.
- **Pod stuck `Pending`** → no schedulable node (resource/disk pressure) or a
  PVC that can't bind.
- **`CrashLoopBackOff`** → the app started but exited (bad config, DB
  unreachable, failed migration). Check logs; confirm `DATABASE_URL`/host.
- **TLS not `Ready`** → the host must resolve to the cluster and :80 be
  reachable; check `orcinus kubectl get certificate`.

## Deploy checklist

- [ ] Every reachable service has `ports:`.
- [ ] Own-code images are built and **pushed** to the registry before deploy.
- [ ] Public services have `x-orcinus-expose: ingress` + `x-orcinus-host` (+ `x-orcinus-tls`).
- [ ] Credentials are listed in `x-orcinus-secret`; managed DBs referenced by name.
- [ ] Stateful services use `statefulset` + `x-orcinus-volume-size`.
- [ ] The DNS host resolves to the cluster.
