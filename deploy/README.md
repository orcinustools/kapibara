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
kubectl apply -f deploy/rbac.yaml
```

Kapibara runs in-cluster and uses its ServiceAccount to read pod logs and
metrics; this binds that access to the namespace's `default` ServiceAccount.
(Skip only if you don't need the Logs/Metrics tabs.)

---

## 4. Configure & deploy

Edit `kapibara.compose.yml`:

- `image:` → your pushed image
- `x-orcinus-host:` → `kapibara.example.com`
- `KAPIBARA_ORCINUS_URL` / `KAPIBARA_ORCINUS_TOKEN` → how the pod reaches the
  orcinus engine API (an in-cluster Service, or the node/host IP:8899)
- `KAPIBARA_ACME_EMAIL` → your email
- `KAPIBARA_JWT_SECRET` → `openssl rand -hex 32`

Deploy it through orcinus:

```bash
orcinus deploy -f deploy/kapibara.compose.yml --project kapibara --wait
```

This creates a Deployment + Service + Ingress (+ a PVC for `/data` and a Secret
for the marked env keys).

---

## 5. Verify

```bash
orcinus ps kapibara                       # pod Running
kubectl get ingress,certificate | grep kapibara   # cert READY=True
curl -s https://kapibara.example.com/healthz      # {"engineHealthy":true,...}
```

Open `https://kapibara.example.com`, register the first account (becomes the
platform admin), then follow the [Deploy Guide](../docs/DEPLOY-GUIDE.md) to ship
apps and databases.

---

## Notes

- **Git builds from an in-cluster control-plane:** building images from Git needs
  Docker on the host running Kapibara. A pod has no Docker, so set
  `KAPIBARA_REGISTRY` and run builds where Docker is available, or use the
  prebuilt-image / compose deploy paths from inside the cluster. Image, compose,
  and one-click database deploys work fully in-cluster.
- **Namespace:** the manifests assume `default`. If you deploy elsewhere, set
  `KAPIBARA_NAMESPACE` and update the subject namespace in `rbac.yaml`.
