# Deploying Kapibara onto orcinus (with a domain)

This deploys the Kapibara control-plane **as a workload on the orcinus cluster**
itself, reachable at a domain with automatic TLS.

Files:

- [`kapibara.compose.yml`](./kapibara.compose.yml) — the orcinus compose (image,
  env, volume, ingress host + TLS).
- [`rbac.yaml`](./rbac.yaml) — grants the pod read access to pod logs + metrics.
- [`../Dockerfile`](../Dockerfile) — builds the Kapibara image (UI embedded).

Replace `kapibara.example.com` / `you@example.com` with your own values.

---

## 1. Build & push the image

```bash
# From the repo root:
IMAGE=ghcr.io/orcinustools/kapibara:latest   # or your registry

docker build -t "$IMAGE" \
  --build-arg VERSION="$(git describe --tags --always)" \
  --build-arg COMMIT="$(git rev-parse --short HEAD)" .
docker push "$IMAGE"
```

Set `image:` in `kapibara.compose.yml` to `$IMAGE`.

> **No registry?** Import the image straight into the single-node k3s containerd:
> ```bash
> docker save "$IMAGE" | docker exec -i orcinus ctr -n k8s.io images import -
> ```
> For a private registry, create a pull secret and add
> `x-orcinus-image-pull-secret: <secret-name>` to the service.

---

## 2. Point the domain at the cluster

Add a DNS record for the Kapibara host (and, if you'll host apps too, the
wildcard) → your cluster's public IP:

```
kapibara.example.com.   A   203.0.113.10
*.apps.example.com.     A   203.0.113.10
```

TLS is issued by cert-manager over HTTP-01, so the host must be publicly
resolvable and ports 80/443 must reach the ingress.

---

## 3. Grant cluster access (logs & metrics)

```bash
orcinus kubectl apply -f deploy/rbac.yaml
```

Kapibara runs in-cluster and uses its ServiceAccount to read pod logs and
metrics; this binds that access to the namespace's `default` ServiceAccount.
(Skip only if you don't need the Logs/Metrics tabs.)

---

## 4. (Optional) Enable the built-in Docker registry gateway

Kapibara can front an in-cluster registry so users push images to
`https://kapibara.example.com/registry/<project>/<image>` (authenticated) and the
cluster pulls them back — no external registry needed. Install the plugin:

```bash
orcinus plugin install registry     # registry:2 at registry.orcinus-registry.svc:5000
```

Then set in `kapibara.compose.yml`:
`KAPIBARA_REGISTRY_UPSTREAM=http://registry.orcinus-registry.svc:5000` and
`KAPIBARA_REGISTRY_PUBLIC=kapibara.example.com`. Users build & push with
`kapibara image build` (Docker) or `kapibara image pack` (no Docker), and
reference `image: registry/<project>/<image>:<tag>` in a deploy.

## 5. Configure & deploy

Edit `kapibara.compose.yml`:

- `image:` → your pushed image
- `x-orcinus-host:` → `kapibara.example.com`
- `KAPIBARA_ORCINUS_URL` / `KAPIBARA_ORCINUS_TOKEN` → how the pod reaches the
  orcinus engine API (an in-cluster Service, or the node/host IP:8899)
- `KAPIBARA_ACME_EMAIL` → your email
- `KAPIBARA_JWT_SECRET` → `openssl rand -hex 32`
- `KAPIBARA_PUBLIC_URL` → `https://kapibara.example.com`
- `KAPIBARA_APPS_DOMAIN` → `apps.example.com` (base host for deployed apps)
- `KAPIBARA_REGISTRY_UPSTREAM` / `KAPIBARA_REGISTRY_PUBLIC` → see step 4

Deploy it through orcinus:

```bash
orcinus deploy -f deploy/kapibara.compose.yml --project kapibara --wait
```

This creates a Deployment + Service + Ingress (+ a PVC for `/data` and a Secret
for the marked env keys).

---

## 6. Verify

```bash
orcinus ps kapibara                       # pod Running
orcinus kubectl get ingress,certificate | grep kapibara   # cert READY=True
curl -s https://kapibara.example.com/healthz      # {"engineHealthy":true,...}
```

Open `https://kapibara.example.com`, register the first account (becomes the
platform admin), then follow the [Deploy Guide](../docs/DEPLOY-GUIDE.md) to ship
apps and databases.

---

## Notes

- **Building images (in-cluster Kapibara can't build from Git):** build where
  you are and push to the registry gateway (step 4). `kapibara image build` uses
  local Docker for full Dockerfiles; `kapibara image pack` assembles a base +
  files in-process with **no Docker** (great for static sites / Go binaries, and
  builds `linux/amd64` from any host). Then reference `registry/<project>/<image>`.
  Image, compose, and one-click database deploys all work fully in-cluster.
- **Namespace:** the manifests assume `default`. If you deploy elsewhere, set
  `KAPIBARA_NAMESPACE` and update the subject namespace in `rbac.yaml`.
