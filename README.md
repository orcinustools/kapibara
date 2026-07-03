# Kapibara 🦫

Self-hosted PaaS (alternatif Vercel/Heroku/Dokploy) dengan **[Orcinus](../orcinus)**
sebagai cluster engine. Kapibara = control-plane (API + UI); Orcinus menjalankan
`docker-compose.yml` di atas Kubernetes.

- **Backend:** Go, single binary.
- **Frontend:** React + TypeScript + Vite SPA, di-embed ke binary (`pkg/webui/dist`).
- **Engine:** Orcinus via HTTP API + akses cluster langsung (client-go) untuk logs/metrics/exec.
- **Store:** SQLite (pure-Go, dev) atau Postgres (prod), via GORM.

Build UI + binary: `make build` (butuh Node+npm untuk UI). Dev UI: `cd web && npm run dev` (proxy ke API :9000).
Browser e2e: `node web/e2e.mjs` (Playwright).

Lihat [`PLAN.md`](./PLAN.md) untuk arsitektur & milestone.

## Build & run

```bash
make build            # → bin/kapibara (UI embedded)
make test             # unit + integration tests

# Jalankan (butuh orcinus API + cluster)
orcinus api --addr :8899 --token "$TOKEN"        # engine
KAPIBARA_ORCINUS_URL=http://localhost:8899 \
KAPIBARA_ORCINUS_TOKEN=$TOKEN \
KAPIBARA_JWT_SECRET=$(openssl rand -hex 16) \
KAPIBARA_KUBECONFIG=~/.orcinus/kubeconfig \
KAPIBARA_CLUSTER_CONTAINER=orcinus \
  bin/kapibara serve                              # → http://localhost:9000
```

Buka `http://localhost:9000`, register user pertama (jadi admin).

## Konfigurasi (env)

| Var | Default | Fungsi |
|---|---|---|
| `KAPIBARA_ADDR` | `:9000` | Alamat listen |
| `KAPIBARA_DATABASE_URL` | `~/.kapibara/kapibara.db` | SQLite path atau `postgres://…` |
| `KAPIBARA_JWT_SECRET` | (acak) | Penandatangan sesi |
| `KAPIBARA_ORCINUS_URL` | `http://localhost:8899` | Orcinus API |
| `KAPIBARA_ORCINUS_TOKEN` | — | Bearer token orcinus |
| `KAPIBARA_KUBECONFIG` | `~/.orcinus/kubeconfig` | Akses cluster (logs/metrics/backup) |
| `KAPIBARA_CLUSTER_CONTAINER` | `orcinus` | Node k3s (import image tanpa registry) |
| `KAPIBARA_REGISTRY` / `KAPIBARA_BUILD_PUSH` | — | Registry image (mode produksi) |

## Fitur (parity Dokploy)

- **Auth & multi-tenant:** user, organization, project, membership, RBAC, API token (`kap_…`).
- **Deploy Docker Compose** langsung ke cluster (native orcinus).
- **Applications dari Git:** builder Dockerfile / Nixpacks / prebuilt image → deploy. Tanpa registry, image di-import ke containerd k3s.
- **Domains + TLS:** `x-orcinus-expose/host/tls` + cert-manager (ACME).
- **Env & Secrets:** secret di-extract ke `secretKeyRef` (via `x-orcinus-secret`).
- **Database 1-klik:** Postgres/MySQL/MariaDB/MongoDB/Redis (StatefulSet + PVC + secret) + connection string.
- **Day-2 ops:** logs streaming, metrics (CPU/RAM), scale, autoscale (HPA), rollback, progressive delivery.
- **Templates** one-click (WordPress, Redis, n8n, …).
- **Webhook auto-deploy** (push-to-deploy).
- **Backups** DB terjadwal (cron) → local / S3-compatible.
- **Notifikasi:** Slack, Discord, Telegram, webhook, email.
- **Multi-node** (daftar node), **preview deployments** per-branch (project ephemeral), **audit log**.

## API

REST di `/api/v1`. Auth: `Authorization: Bearer <session-jwt | kap_token>`.
Endpoint utama: `auth/*`, `orgs`, `projects`, `.../deploy`, `.../apps`, `.../databases`,
`.../logs`, `.../metrics`, `.../services/{svc}/scale|rollback`, `templates`,
`.../backups`, `orgs/{id}/notifications`, `nodes`, `audit`, `webhooks/{secret}`.

## Status / catatan

Semua milestone M0–M9 + follow-up terimplementasi & diverifikasi e2e terhadap cluster orcinus nyata:
isolasi per-unit orcinus project (tiap app/db/compose punya prune scope sendiri), S3 backup (MinIO),
2FA (TOTP), **SPA React penuh** (login/2FA, projects, project detail dengan tab
apps/databases/compose/deployments/logs/templates/backups, cluster+plugins, settings, audit) — diverifikasi
lewat browser e2e (Playwright), dan **TLS ACME production** (sertifikat Let's Encrypt terpercaya diterbitkan
end-to-end untuk `*.apps.jonggrang.dev` via cert-manager HTTP-01). Semua fitur terverifikasi e2e nyata.

## License

MIT.
