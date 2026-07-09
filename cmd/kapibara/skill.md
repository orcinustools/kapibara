# Skill: Author an `orcinus.yml` deployment file

Use this when a user asks to deploy an app on **Kapibara / orcinus**. Produce an
`orcinus.yml` — a standard **docker-compose** file annotated with `x-orcinus-*`
hints that orcinus converts to Kubernetes objects — then deploy it.

## How to use

1. Inspect the project (language, how it starts, the port it listens on, whether
   it needs a database or persistent storage).
2. Decide the image source:
   - **Prebuilt / built elsewhere** → reference the image directly.
   - **Kapibara registry gateway** → build locally and
     `docker push <kapibara-host>/<scope>/<name>:<tag>`, then reference it as
     `kapibara/<scope>/<name>:<tag>` (Kapibara rewrites it to the pull address).
3. Write `orcinus.yml` (see the template).
4. Deploy:
   ```bash
   orcinus deploy -f orcinus.yml --project <name> --acme-email you@example.com --wait
   ```
   Or via Kapibara compose: `kapibara deploy --project <name> -f orcinus.yml`.

## Rules that matter

- One `services:` entry per container. Standard compose keys work: `image`,
  `ports`, `environment`, `command`, `volumes`, `healthcheck`,
  `deploy.replicas`, `deploy.resources`.
- **A service must declare `ports:` to get a Service** — without it there is no
  in-cluster DNS name, so databases/backends become unreachable. Always list the
  port a peer connects to.
- Services reach each other by **service name** (e.g. `db:5432`, `cache:6379`).
- Expose to the internet with `x-orcinus-expose: ingress` + `x-orcinus-host` +
  `x-orcinus-tls: letsencrypt`. The host must resolve to the cluster and
  ports 80/443 must be reachable for the ACME HTTP-01 challenge.
- Put sensitive env keys in `x-orcinus-secret` so they render as a Kubernetes
  Secret instead of plain env.
- Named volumes become a PVC sized by `x-orcinus-volume-size`; databases should
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

## Template (web app + Postgres + Redis)

```yaml
services:
  web:
    image: kapibara/acme/web:1        # or a public image, e.g. nginx:alpine
    ports: ["8080"]
    x-orcinus-expose: ingress
    x-orcinus-host: web.apps.example.com
    x-orcinus-tls: letsencrypt
    environment:
      PORT: "8080"
      DATABASE_URL: postgres://app:secret@db:5432/app
      REDIS_URL: redis://cache:6379
    x-orcinus-secret: [DATABASE_URL]
    deploy:
      replicas: 2
      resources:
        limits: { cpus: "0.5", memory: 512M }

  db:
    image: postgres:16
    ports: ["5432"]                    # required so peers can reach db:5432
    x-orcinus-controller: statefulset
    x-orcinus-volume-size: 5Gi
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: app
    x-orcinus-secret: [POSTGRES_PASSWORD]
    volumes: ["db-data:/var/lib/postgresql/data"]

  cache:
    image: redis:7
    ports: ["6379"]
    x-orcinus-controller: statefulset

volumes:
  db-data:
```

## Deploy checklist

- [ ] Every reachable service has `ports:`.
- [ ] Public services have `x-orcinus-expose: ingress` + `x-orcinus-host` (+ `x-orcinus-tls` for HTTPS).
- [ ] Secrets/credentials are listed in `x-orcinus-secret`.
- [ ] Stateful services use `statefulset` + `x-orcinus-volume-size`.
- [ ] The DNS host resolves to the cluster; `--acme-email` is passed for TLS.
