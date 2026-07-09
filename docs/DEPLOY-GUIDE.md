# Kapibara Deploy Guide — Secrets, Databases & Apps with a Domain

This guide walks the full path a user takes to ship an application on Kapibara:
import secrets, provision managed databases (Postgres + Redis), deploy an app
behind a public domain with automatic TLS, and connect that app to the
databases — all from the CLI.

It mirrors an end-to-end run verified against a live `orcinus` cluster where
`*.apps.example.com` points at the server and cert-manager issues Let's
Encrypt certificates.

---

## Prerequisites

- A running Kapibara server (control-plane), e.g. `http://localhost:9000`, wired
  to an `orcinus` cluster engine (`KAPIBARA_ORCINUS_URL` / `KAPIBARA_ORCINUS_TOKEN`).
- For TLS: the server started with `KAPIBARA_ACME_EMAIL=you@example.com`, a
  wildcard DNS record (`*.apps.example.com → server IP`), and cert-manager +
  an ingress (traefik) on ports 80/443 in the cluster.
- The `kapibara` CLI binary (`make build` → `bin/kapibara`).

Config precedence for the CLI: a saved login (`~/.kapibara/cli.json`) wins over
`KAPIBARA_URL` / `KAPIBARA_TOKEN` env vars. Override the config path with
`KAPIBARA_CLI_CONFIG`.

---

## 1. Log in

```bash
kapibara login --server http://localhost:9000 --email you@example.com --password '••••••'
# caches token + first org id into ~/.kapibara/cli.json
```

`--password` may be supplied via `KAPIBARA_PASSWORD`; add `--totp 123456` if 2FA
is enabled.

---

## 2. Import secrets

Cluster secrets are stored **write-only**: the list endpoint returns only names
and key counts — values are never returned.

```bash
# From individual KEY=VALUE pairs:
kapibara secret put demo-db --data DATABASE_URL='postgres://…' --data REDIS_URL='redis://…'

# Or import a whole .env file:
kapibara secret put app-env --env-file ./.env

kapibara secret list          # names + key counts only
kapibara secret rm demo-db
```

These map to the cluster secret API (`/api/v1/secrets`). A compose service
references such keys by listing them under `x-orcinus-secret` (see §5).

---

## 3. Create & deploy managed databases

Supported engines: `postgres`, `mysql`, `mariadb`, `mongo`, `redis`.

Databases are created and deployed via the API (the web UI does this too; the
CLI focuses on compose/app deploys). Using the REST API directly:

```bash
# create a project
curl -sX POST $API/orgs/$ORG/projects -H "$AUTH" -d '{"name":"demo"}'

# create + deploy Postgres
curl -sX POST $API/projects/$PID/databases -H "$AUTH" \
  -d '{"name":"pg","engine":"postgres"}'                 # → returns db id
curl -sX POST $API/databases/$PGID/deploy -H "$AUTH"     # waits for ready

# create + deploy Redis
curl -sX POST $API/projects/$PID/databases -H "$AUTH" \
  -d '{"name":"cache","engine":"redis"}'
curl -sX POST $API/databases/$RDID/deploy -H "$AUTH"
```

Each database is provisioned as a **StatefulSet + PVC + ClusterIP Service**. The
Service gives the database its in-cluster DNS name, equal to the database name:

| Engine   | In-cluster host | Port  | Connection string                              |
|----------|-----------------|-------|------------------------------------------------|
| postgres | `pg`            | 5432  | `postgres://user:pass@pg:5432/app`             |
| redis    | `cache`         | 6379  | `redis://cache:6379`                           |
| mysql    | `<name>`        | 3306  | `mysql://user:pass@<name>:3306/app`            |
| mongo    | `<name>`        | 27017 | `mongodb://user:pass@<name>:27017/app`         |

The deploy response and `GET /databases/{id}` include the full
`connectionString` (with password) so you can wire it into an app. Credentials
default to user `kapibara`, db `app`, and a generated password if unset.

> **Note:** a managed database publishes its engine port so orcinus creates the
> ClusterIP Service — this is what makes it reachable by name from other
> services in the project. (Prior to this being fixed, DBs deployed without a
> Service and were unreachable.)

---

## 4. Deploy an app behind a domain with TLS

Prebuilt image, one command, public HTTPS host:

```bash
kapibara app deploy --project demo --name adminer \
  --build image --image adminer --port 8080 \
  --domain adminer.apps.example.com --tls \
  --env ADMINER_DEFAULT_SERVER=pg
```

- `--domain` + `--tls` render `x-orcinus-expose: ingress`, `x-orcinus-host`, and
  `x-orcinus-tls: letsencrypt`; cert-manager then issues a Let's Encrypt
  certificate for the host over HTTP-01.
- Build types: `image` (prebuilt), `dockerfile` / `nixpacks` (`--repo` git URL,
  built on the server), and the deploy streams logs with `--follow`.

Verify: `https://adminer.apps.example.com` serves 200 with a valid LE cert.

---

## 5. Connect an app to the databases

Apps receive database credentials as environment variables; mark sensitive ones
as cluster Secrets with `--secret`:

```bash
kapibara app deploy --project demo --name api \
  --build image --image your/api:latest --port 3000 \
  --domain api.apps.example.com --tls \
  --env DATABASE_URL='postgres://kapibara:PASS@pg:5432/app' \
  --env REDIS_URL='redis://cache:6379' \
  --secret DATABASE_URL          # stored as a cluster Secret, not plain env
```

Flags added for this flow:

| Flag        | Effect                                                        |
|-------------|---------------------------------------------------------------|
| `--env K=V` | Set an environment variable (repeatable).                     |
| `--secret K`| Mark an `--env` key as a cluster Secret (`x-orcinus-secret`). |
| `--command` | Override the container command (repeatable, in order).        |

Because every unit in a project lands in the same cluster namespace, the app
resolves the databases by their short service names (`pg`, `cache`) — the same
hosts that appear in the connection strings from §3.

Re-running `app deploy` with the same `--name` **updates** the app (env/domain/
image changes take effect on the next deploy) instead of erroring.

---

## 5b. Push your own images via the built-in registry gateway

When the in-cluster registry is enabled and Kapibara is started with
`KAPIBARA_REGISTRY_UPSTREAM` + `KAPIBARA_REGISTRY_PUBLIC`, Kapibara's own HTTPS
host doubles as a **Docker registry gateway** (Docker token-auth): **pushes need
a Kapibara login, pulls are anonymous** so the cluster fetches images with no
pull secret.

```bash
# 1. Log in with your Kapibara account (password or a kap_ API token).
docker login kapibara.example.com -u you@example.com

# 2. Tag + push under a scope (e.g. your org/user id) and push.
docker build -t kapibara.example.com/<scope>/myapp:1 .
docker push  kapibara.example.com/<scope>/myapp:1
```

Then deploy it with the short form `kapibara/<scope>/myapp:1` — at deploy time
Kapibara rewrites that to `kapibara.example.com/<scope>/myapp:1` so the cluster
pulls it back through the gateway:

```bash
kapibara app deploy --project demo --name myapp \
  --build image --image kapibara/<scope>/myapp:1 \
  --port 8080 --domain myapp.apps.example.com --tls
```

This is the recommended path when Kapibara runs **in-cluster** (a pod can't build
from Git): build on your machine (or CI), push to the gateway, deploy the image.

## 6. Worked example (verified end-to-end)

```bash
# login
kapibara login --server http://localhost:9000 --email you@example.com --password '••••••'

# databases (via API): postgres "pg" + redis "cache" → both deploy: success
#   pg    → Service pg:5432,    authenticated `select version()` → PostgreSQL 16.14
#   cache → Service cache:6379, SET/GET round-trips OK

# import the connection strings as a cluster secret
kapibara secret put demo-db --data DATABASE_URL="$PG_CS" --data REDIS_URL="$RD_CS"

# a web app on a public domain that connects to postgres
kapibara app deploy --project demo --name adminer \
  --build image --image adminer --port 8080 \
  --domain adminer.apps.example.com --tls --env ADMINER_DEFAULT_SERVER=pg

# an app that carries the DB creds (DATABASE_URL kept as a Secret)
kapibara app deploy --project demo --name connector \
  --build image --image nicolaka/netshoot --command sleep --command infinity \
  --env DATABASE_URL="$PG_CS" --env REDIS_URL="$RD_CS" --secret DATABASE_URL --follow=false
```

Observed results:

- `adminer.apps.example.com` → **HTTP 200**, valid **Let's Encrypt** cert
  (`CN=adminer.apps.example.com`), and a real login into Postgres shows the
  `public` schema + SQL console.
- From the `connector` app pod: TCP to `pg:5432` **open**, and a raw
  `PING` to `cache:6379` returns **`+PONG`** — proving app → DB connectivity.

---

## Troubleshooting

- **App can't resolve `pg` / `cache`:** confirm the database Service exists
  (`kubectl get svc` should list `pg`, `cache`). Managed DBs must publish their
  port to get a Service; redeploy the database if it was created before this fix.
- **TLS not issued:** the host must resolve publicly to the cluster and be
  reachable on :80 for the HTTP-01 challenge; the server needs
  `KAPIBARA_ACME_EMAIL` set. Check `kubectl get certificate` for readiness.
- **Secret value not visible:** by design — secret values are write-only and
  never returned; only names + key counts are listed.
