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

## Dokumentasi

- [`docs/SETUP.md`](./docs/SETUP.md) — install & jalankan Kapibara, DNS wildcard, TLS.
- [`docs/DEPLOY-GUIDE.md`](./docs/DEPLOY-GUIDE.md) — import secret, database 1-klik, deploy app (image/Git/compose) + domain, koneksi app↔DB.

Rilis: push tag `vX.Y.Z` → GitHub Actions ([`.github/workflows/release.yml`](./.github/workflows/release.yml)) build binari lintas-platform via GoReleaser.

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
- **App runtime config:** resource limits/reservations (CPU/RAM), command override, persistent volume mounts (PVC), ingress path, exec health check — semua editable (bukan create-only lagi).
- **Rollback ke deployment historis:** `deployment redeploy` me-apply ulang snapshot compose lama (untuk app = image lama tanpa rebuild).
- **Templates** one-click (WordPress, Redis, n8n, …).
- **Webhook auto-deploy** (push-to-deploy).
- **Backups** DB terjadwal (cron) → local / S3-compatible.
- **Notifikasi:** Slack, Discord, Telegram, webhook, email.
- **Multi-node** (daftar node), **preview deployments** per-branch (project ephemeral), **audit log**.

## CLI (deploy dari lokal ke server)

Binary yang sama (`bin/kapibara`) juga menjadi client REST — deploy dari laptop
ke server kapibara mana pun tanpa membuka UI:

```bash
kapibara login --server http://server:9000 --email you@example.com --password …
kapibara projects create shop
kapibara deploy --project shop -f docker-compose.yml        # deploy compose
kapibara app deploy --project shop --name web --build image \
  --image nginx:alpine --port 80 \
  --cpu-limit 0.5 --memory-limit 512M \
  --mount data:/var/lib/data --volume-size 2Gi             # app + resources + PVC
kapibara deployment list --project shop                     # riwayat deploy
kapibara deployment redeploy <deploymentId>                 # rollback ke snapshot lama
```

Token & server hasil `login` disimpan di `~/.kapibara/cli.json` (override lewat
`KAPIBARA_URL` / `KAPIBARA_TOKEN` bila belum login). E2E CLI: `scripts/cli-e2e.sh`.

## API

REST di `/api/v1`. Auth: `Authorization: Bearer <session-jwt | kap_token>`.
Endpoint utama: `auth/*`, `orgs`, `projects`, `.../deploy`, `.../apps`, `.../databases`,
`deployments/{id}/redeploy`, `.../logs`, `.../metrics`, `.../services/{svc}/scale|rollback`, `templates`,
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
