# Kapibara — Rencana Proyek (PLAN)

> **Kapibara** adalah PaaS self-hosted open-source (alternatif Vercel/Heroku/Netlify)
> yang mengambil model & fitur dari [Dokploy](https://github.com/dokploy/dokploy),
> tetapi **mengganti cluster engine (Docker Swarm) dengan
> [Orcinus](../orcinus)** — runtime cluster berbasis Kubernetes yang menjalankan
> `docker-compose.yml` secara native.
>
> Kapibara = **control-plane** (UI + API). Orcinus = **cluster engine**.

- **Target:** full feature parity dengan Dokploy.
- **Stack:** Backend **Go** (satu binary, embed UI) + frontend **React (Vite + TypeScript)**.
- **Engine:** Orcinus via HTTP REST API dan/atau import langsung `pkg/engine`.

---

## 1. Konsep & Posisi

Dokploy adalah lapisan orkestrasi di atas Docker Swarm + Traefik. Kapibara
memindahkan lapisan itu ke atas Orcinus:

| Lapisan | Dokploy | Kapibara |
|---|---|---|
| UI / Control-plane | Next.js + tRPC | Go API + React SPA (single binary) |
| State / DB | Postgres (Drizzle) | Postgres (control-plane state) |
| Cluster engine | Docker Swarm | **Orcinus** (K8s runtime, compose-native) |
| Reverse proxy / TLS | Traefik + Let's Encrypt | Traefik (bawaan orcinus) + `cert-manager` |
| Build | Nixpacks / Dockerfile / Buildpacks | idem (image → registry) |

**Prinsip integrasi:** Kapibara tidak menulis manifest Kubernetes. Ia menyusun
`docker-compose.yml` + anotasi `x-orcinus-*` lalu menyerahkannya ke Orcinus
(`POST /api/v1/deploy` atau `engine.Deploy`). Semua konversi compose→k8s,
prune, ownership label, auto-install plugin ditangani Orcinus.

---

## 2. Arsitektur

```
┌───────────────────────────────────────────────────────────┐
│  Browser (React SPA)                                        │
└───────────────┬───────────────────────────────────────────┘
                │ REST/JSON + WebSocket (logs, events)
┌───────────────▼───────────────────────────────────────────┐
│  kapibara-server (Go, single binary)                        │
│  ┌──────────┬───────────┬───────────┬──────────────────┐   │
│  │ Auth/RBAC│ Projects/ │ Build svc │ Deploy orchestr. │   │
│  │ Orgs     │ Apps/DB   │ (nixpacks)│ (compose+x-orc-*)│   │
│  ├──────────┴───────────┴───────────┴──────────────────┤   │
│  │ Git providers │ Webhooks │ Notif │ Backup │ Scheduler│   │
│  └──────────────────────────────────────────────────────┘  │
│         │ Postgres (control-plane state)                     │
└─────────┼─────────────────────┬────────────────────────────┘
          │ orcinus HTTP API     │ container build/push
┌─────────▼──────────┐  ┌────────▼─────────┐
│  Orcinus engine     │  │  Registry        │
│  (cluster runtime)  │  │  (embedded/ext)  │
│  Traefik, cert-mgr, │  └──────────────────┘
│  metrics, storage…  │
└─────────┬───────────┘
          │ kubeconfig
┌─────────▼───────────────────────────────────────────────┐
│  Cluster (single / multi-node / HA) — orcinus cluster    │
└──────────────────────────────────────────────────────────┘
```

### Komponen backend (Go)
- `cmd/kapibara` — multicall CLI: `serve`, `migrate`, `admin`, `agent`.
- `pkg/api` — HTTP server (chi/echo), REST + WebSocket, auth middleware.
- `pkg/store` — Postgres + migrations (sqlc atau GORM), model domain.
- `pkg/auth` — sesi, JWT/cookie, RBAC, API token, 2FA.
- `pkg/orcinus` — client ke Orcinus (HTTP API) + adapter opsional ke `pkg/engine`.
- `pkg/build` — builder: Nixpacks, Dockerfile, Buildpacks, static, Docker image.
- `pkg/compose` — generator `docker-compose.yml` + `x-orcinus-*` dari model app/DB.
- `pkg/git` — integrasi GitHub/GitLab/Bitbucket/Gitea + webhook.
- `pkg/deploy` — orkestrasi build→push→deploy, antrian job, status/rollback.
- `pkg/notify` — Slack/Discord/Telegram/email/webhook.
- `pkg/backup` — dump DB + upload S3-compatible (MinIO via plugin storage).
- `pkg/scheduler` — cron jobs (backup, scheduled tasks).
- `web/` — React SPA (Vite + TS), di-`embed` ke binary saat rilis.

---

## 3. Pemetaan Fitur Dokploy → Kapibara/Orcinus

| Fitur Dokploy | Cara Kapibara mewujudkannya lewat Orcinus |
|---|---|
| Deploy app dari Git | Clone → build (nixpacks/Dockerfile) → push image → compose + deploy |
| Deploy Docker image | Compose 1 service → `deploy` |
| **Docker Compose app** | Kirim compose apa adanya ke `POST /deploy` (native orcinus) |
| Database 1-klik (PG/MySQL/Mongo/Redis/MariaDB) | Compose + `x-orcinus-controller: statefulset`, `x-orcinus-volume-size`, `x-orcinus-secret`. Postgres HA opsional via operator (cnpg, lihat `examples/app-with-cnpg`) |
| Domain + SSL otomatis | `x-orcinus-expose: ingress`, `x-orcinus-host`, `x-orcinus-tls` + plugin `cert-manager` (auto-install saat deploy) |
| Env vars & secrets | Secrets API orcinus + `x-orcinus-secret`; env non-rahasia via compose `environment` |
| Logs real-time | Stream via orcinus `logs` / kubectl passthrough → WebSocket ke UI |
| Monitoring CPU/RAM | Plugin `metrics-server` + `prometheus`/`grafana`; UI baca metrics |
| Rollback | `POST /projects/{p}/services/{s}/rollback` |
| Scaling / autoscale | `scale` API + `x-orcinus-autoscale-*` (HPA) |
| Deploy strategy / progressive | `update_config` + `x-orcinus-rollout` (argo-rollouts, auto-install) |
| Templates (one-click apps) | Katalog compose ala `examples/` (WordPress, Redis, monitoring, …) |
| Backups → S3 | `pkg/backup` + plugin `storage` (MinIO) atau S3 eksternal |
| Notifikasi | `pkg/notify` |
| Multi-node / HA cluster | `orcinus cluster init/join`, docs `CLUSTER.md`, `HA-STORAGE.md` |
| Preview deployments | Project/namespace ephemeral per-branch/PR + deploy |
| Roles & permissions | RBAC di control-plane (Owner/Admin/Member + per-project) |
| Webhook auto-deploy | `pkg/git` webhook → trigger job deploy |
| API + CLI | REST API kapibara + `kapibara` CLI (mirror UI) |
| Scheduled jobs / cron | `pkg/scheduler` → CronJob via compose/manifest |
| Volumes | compose `volumes` → PVC (orcinus) |

---

## 4. Model Data (control-plane)

Entitas inti (Postgres):

- **Organization** (multi-tenant) → **User** ↔ **Membership** (role).
- **Project** — grup logis; memetakan ke `project`/namespace orcinus.
- **Application** — sumber (git/image/compose), builder, env, domains, resources.
- **Database** — tipe (pg/mysql/mongo/redis/mariadb), versi, volume, kredensial.
- **Compose** — deployment compose mentah.
- **Deployment** — histori build/deploy (status, log, commit, image tag).
- **Domain** — host, path, TLS, service target.
- **EnvVar / Secret** — key/value, scoped ke app/project.
- **GitProvider** — kredensial GitHub/GitLab/Bitbucket/Gitea.
- **Registry** — kredensial registry (push/pull).
- **Backup / Schedule** — jadwal + destinasi S3.
- **Notification** — channel + trigger event.
- **ApiToken**, **AuditLog**, **Server/Node** (info cluster).

---

## 5. Milestones (bertahap menuju full parity)

Setiap milestone menghasilkan sesuatu yang bisa dipakai end-to-end.

### M0 — Scaffold & fondasi
- Layout repo Go + `web/` React, Makefile, lint, CI.
- `cmd/kapibara serve`, config (env/file), health/version.
- Postgres + migrations, `pkg/store` dasar.
- Client `pkg/orcinus` (HTTP) + smoke test ke orcinus API.
- **Deliverable:** server jalan, konek ke orcinus, `GET /cluster` tampil di UI.

### M1 — Auth, Org, Project (multi-tenant)
- Registrasi/login, sesi, API token, RBAC dasar.
- CRUD Organization, Project, Membership.
- UI shell (layout, navigasi, tema).
- **Deliverable:** user bisa login & kelola project.

### M2 — Deploy Docker Compose (jalur tersingkat ke nilai)
- Editor compose di UI → simpan → deploy ke orcinus (`/deploy`, `wait`).
- Tampilkan objek terpasang, pods (`/projects/{p}/pods`), status.
- Hapus project (`DELETE`).
- **Deliverable:** deploy compose end-to-end lewat kapibara.

### M3 — Applications dari Git + Build
- Git connect (GitHub OAuth/App dulu), pilih repo/branch.
- Builder: Nixpacks & Dockerfile → build image → push ke registry.
- Generate compose 1-service + `x-orcinus-expose/host` → deploy.
- Histori Deployment + streaming build log (WebSocket).
- **Deliverable:** "push repo → live URL".

### M4 — Domains, TLS, Env & Secrets
- Kelola domain/host per app, path routing.
- TLS otomatis via `x-orcinus-tls` + auto-install `cert-manager` (ACME email).
- Env vars (compose) + Secrets (orcinus secrets API + `x-orcinus-secret`).
- **Deliverable:** app publik dengan HTTPS + konfigurasi env/secret.

### M5 — Databases 1-klik
- Wizard: Postgres/MySQL/MariaDB/MongoDB/Redis (versi, size, kredensial).
- Generate compose statefulset + volume + secret; expose internal.
- Connection string ter-inject ke app (linking).
- **Deliverable:** provision DB + hubungkan ke app.

### M6 — Logs, Monitoring, Scaling, Rollback
- Log streaming per service (WebSocket) + filter.
- Metrics (metrics-server/prometheus) → grafik CPU/RAM di UI.
- Scale manual + autoscale (`x-orcinus-autoscale-*`).
- Rollback ke revisi sebelumnya; deploy strategy (`x-orcinus-rollout`).
- **Deliverable:** day-2 ops lengkap dari UI.

### M7 — Templates & Webhook auto-deploy
- Katalog template (dari `examples/` orcinus + template kapibara).
- Deploy template 1-klik dengan parameter (domain, kredensial).
- Webhook Git (push/PR) → auto-deploy; deploy on merge.
- **Deliverable:** one-click apps + CI/CD dasar.

### M8 — Backups, Notifikasi, Scheduler
- Backup DB terjadwal → S3/MinIO (plugin `storage`), restore.
- Notifikasi Slack/Discord/Telegram/email pada event deploy/error.
- Scheduled jobs / cron (CronJob via orcinus).
- **Deliverable:** operasional produksi (backup + alerting).

### M9 — Multi-node, HA, Preview deploy, polish
- Kelola node (`cluster init/join/status`) + tampilan cluster.
- HA storage (docs `HA-STORAGE.md`), Postgres operator (cnpg).
- Preview deployment per-branch/PR (namespace ephemeral, auto-teardown).
- Audit log, RBAC lanjutan, 2FA, dokumentasi, `install.sh`, rilis (goreleaser).
- **Deliverable:** parity penuh + siap rilis publik.

---

## 6. Integrasi Orcinus — detail teknis

- **Mode koneksi:**
  1. *HTTP client* (default): kapibara jalan sebagai proses terpisah, panggil
     `orcinus api` (bearer token). Paling loose-coupled.
  2. *In-process* (opsional): import `github.com/orcinustools/orcinus/pkg/engine`
     dan panggil `engine.Deploy(...)` langsung — hilangkan network hop untuk
     deployment. Dipertimbangkan setelah M2 bila perlu latensi/atomisitas.
- **Auth:** kapibara simpan `ORCINUS_API_TOKEN` (per-cluster), kirim
  `Authorization: Bearer`.
- **Endpoint yang dipakai:** `deploy`, `convert` (preview manifest di UI),
  `projects`, `projects/{p}/pods`, `scale`, `rollback`, `secrets`, `plugins`,
  `cluster`, `version`, `healthz`.
- **Mapping identitas:** `Project` kapibara → `project` orcinus (label ownership).
  App/DB jadi service dalam compose project tsb.
- **Plugin lifecycle:** kapibara memicu install plugin via `POST /plugins/{name}`
  (cert-manager, storage, metrics-server, argo-rollouts) sesuai fitur yang dipakai;
  orcinus juga auto-install saat deploy bila terdeteksi `x-orcinus-tls`/`rollout`.

---

## 7. Keputusan teknis (usulan, bisa direvisi)

| Area | Usulan | Alasan |
|---|---|---|
| HTTP router | chi | ringan, idiomatik, mudah middleware |
| DB access | sqlc + pgx | type-safe, tanpa ORM berat |
| Migrations | goose / atlas | sederhana, reproducible |
| Auth | cookie session + JWT untuk API token | web + API |
| Realtime | WebSocket (gorilla/coder) | logs & events |
| Frontend | React + Vite + TS + Tailwind + shadcn/ui | cepat, modern, mirip UX dokploy |
| Build engine | Nixpacks (default) + Dockerfile + Buildpacks | parity dokploy |
| Registry | embedded (distribution) atau eksternal | fleksibel |
| Job queue | in-process worker + tabel jobs (Postgres) | tanpa dependency ekstra dulu |
| Packaging | single binary (embed SPA) + goreleaser + install.sh | ikuti pola orcinus |

---

## 8. Risiko & catatan

- **Orcinus butuh cluster berjalan** (container runtime + kubeconfig). Kapibara
  harus menangani skenario "belum ada cluster" (wizard `cluster init`).
- **Build memerlukan Docker/daemon** di node builder — perlu strategi builder
  (host daemon vs BuildKit in-cluster).
- **Postgres HA / storage HA** untuk produksi mengandalkan plugin/operator orcinus;
  perlu diverifikasi di M9.
- **Preview deploy** menambah kompleksitas namespace lifecycle & cleanup.
- **Parity penuh itu besar** — urutan M0→M9 dirancang agar tiap tahap sudah
  memberi nilai (compose deploy sudah berguna sejak M2).

---

## 9. Langkah berikutnya

1. Konfirmasi PLAN ini (atau revisi milestone/prioritas).
2. Mulai **M0**: scaffold repo Go + React, `serve`, koneksi ke orcinus.
3. Siapkan lingkungan dev: satu cluster orcinus (`orcinus cluster init`) +
   `orcinus api` untuk integrasi.
</content>
</invoke>
