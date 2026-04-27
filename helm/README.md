# PatchMon Helm Chart

Portable Helm chart for PatchMon across EKS, AKS, kubeadm, kind, and other conformant Kubernetes clusters.

Chart path: `helm/patchmon`

Example profiles:

- `helm/examples/values-eks.yaml`
- `helm/examples/values-external-db-redis.yaml`
- `helm/examples/values-local.yaml`
- `helm/examples/values-kind.yaml`
- `helm/examples/values-persistent-nfs.yaml`

## Security model (passwords/secrets)

This chart is **secret-first**. Runtime credentials should come from a Kubernetes Secret, not from plain values files.

Default mode in `values.yaml`:

- `secret.create: false`
- `secret.existingSecretName: patchmon-app-secrets`

Required keys in the referenced secret:

- `JWT_SECRET`
- `SESSION_SECRET`
- `AI_ENCRYPTION_KEY`
- `POSTGRES_PASSWORD` (required when in-chart Postgres is enabled)
- `REDIS_PASSWORD` (required when in-chart Redis is enabled)
- `DATABASE_URL` (required only when using external Postgres with `database.enabled=false`)

---

## Manual install (EKS, internal DB + internal Redis)

### Prerequisites

- A working EKS cluster and `kubectl` context set to it
- `helm` and `openssl` installed
- A default storage class that can provision PVCs (for example `gp3`)

### 1) Verify Kubernetes context

```bash
kubectl config current-context
kubectl get nodes
```

### 2) Create namespace

```bash
kubectl create namespace patchmon --dry-run=client -o yaml | kubectl apply -f -
```

### 3) Set runtime ingress values (always pass these)

```bash
INGRESS_CLASS="alb"
INGRESS_HOST="patchmon.example.com"
```

`CORS_ORIGIN` is auto-derived when `config.corsOrigin` is empty:

- `http://<INGRESS_HOST>` when no ingress TLS is configured
- `https://<INGRESS_HOST>` when ingress TLS is configured

### 4) Create secret values

This profile uses internal Postgres and Redis services (`database`, `redis`) and builds `DATABASE_URL` directly in the server deployment.

```bash
JWT_SECRET="$(openssl rand -hex 64)"
SESSION_SECRET="$(openssl rand -hex 64)"
AI_ENCRYPTION_KEY="$(openssl rand -hex 64)"
POSTGRES_PASSWORD="$(openssl rand -hex 32)"
REDIS_PASSWORD="$(openssl rand -hex 32)"
```

### 5) Create Kubernetes secret

```bash
kubectl -n patchmon create secret generic patchmon-app-secrets \
  --from-literal=JWT_SECRET="$JWT_SECRET" \
  --from-literal=SESSION_SECRET="$SESSION_SECRET" \
  --from-literal=AI_ENCRYPTION_KEY="$AI_ENCRYPTION_KEY" \
  --from-literal=POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
  --from-literal=REDIS_PASSWORD="$REDIS_PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -
```

### 6) (Optional) Validate chart before install

```bash
helm lint ./helm/patchmon
helm template patchmon ./helm/patchmon -f ./helm/examples/values-eks.yaml > /tmp/patchmon-rendered.yaml
```

### 7) Install chart

```bash
helm upgrade --install patchmon ./helm/patchmon \
  --namespace patchmon \
  --create-namespace \
  --wait --timeout 20m \
  -f ./helm/examples/values-eks.yaml \
  --set-string ingress.className="${INGRESS_CLASS}" \
  --set-string ingress.hosts[0].host="${INGRESS_HOST}"
```

### 8) Verify

```bash
kubectl -n patchmon get pods
kubectl -n patchmon get svc
kubectl -n patchmon get pvc
kubectl -n patchmon get ingress
kubectl -n patchmon get ingress patchmon-ingress -o jsonpath='{.spec.ingressClassName}{"\n"}'
kubectl -n patchmon get ingress patchmon-ingress -o jsonpath='{.spec.rules[0].host}{"\n"}'
kubectl -n patchmon get configmap patchmon-config -o jsonpath='{.data.CORS_ORIGIN}{"\n"}'
helm status patchmon -n patchmon
```

### 9) Get ingress hostname (ALB)

```bash
kubectl -n patchmon get ingress patchmon-ingress -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'; echo
```

---

## Troubleshooting (EKS)

If install is stuck on `--wait`, run:

```bash
kubectl -n patchmon get pods
kubectl -n patchmon logs deploy/patchmon-database --tail=200
kubectl -n patchmon logs deploy/patchmon-guacd --tail=200
kubectl -n patchmon get events --sort-by=.lastTimestamp | tail -n 40
```

Known notes:

- Postgres `lost+found` error is handled by setting `PGDATA=/var/lib/postgresql/data/pgdata` in this chart.
- EKS example uses HTTP ALB by default. If you enable HTTPS, add an ACM certificate annotation in ingress values.
- If guacd fails with `exec format error` on ARM/Graviton nodes, use `guacd.image.tag: "1.6.0"` (already set in `helm/examples/values-eks.yaml`).

---

## Universal install pattern (any cluster)

Always pass your ingress class + hostname at install/upgrade time:

```bash
INGRESS_CLASS="nginx"                # e.g. nginx, alb, traefik
INGRESS_HOST="patchmon.example.com"  # your DNS host

helm upgrade --install patchmon ./helm/patchmon \
  -n patchmon --create-namespace \
  --wait --timeout 20m \
  --set ingress.enabled=true \
  --set-string ingress.className="${INGRESS_CLASS}" \
  --set-string ingress.hosts[0].host="${INGRESS_HOST}" \
  --set-string ingress.hosts[0].paths[0].path="/" \
  --set-string ingress.hosts[0].paths[0].pathType="Prefix"
```

Optional explicit override:

```bash
--set-string config.corsOrigin="https://${INGRESS_HOST}"
```

---

## Deployment option: external DB + external Redis (any cluster)

1) Prepare secret with external `DATABASE_URL` and other keys.
2) Deploy:

```bash
kubectl -n patchmon create secret generic patchmon-app-secrets \
  --from-literal=JWT_SECRET="$JWT_SECRET" \
  --from-literal=SESSION_SECRET="$SESSION_SECRET" \
  --from-literal=AI_ENCRYPTION_KEY="$AI_ENCRYPTION_KEY" \
  --from-literal=DATABASE_URL='postgresql://patchmon_user:REPLACE_DB_PASSWORD@my-postgres.example.internal:5432/patchmon_db' \
  --from-literal=REDIS_PASSWORD="$REDIS_PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -
```

```bash
helm upgrade --install patchmon ./helm/patchmon \
  -n patchmon --create-namespace \
  --wait --timeout 20m \
  -f ./helm/examples/values-external-db-redis.yaml
```

---

## Deployment option: kubeadm/local ingress with internal DB/Redis (ephemeral)

1) Create secret:

```bash
JWT_SECRET="$(openssl rand -hex 64)"
SESSION_SECRET="$(openssl rand -hex 64)"
AI_ENCRYPTION_KEY="$(openssl rand -hex 64)"
POSTGRES_PASSWORD="$(openssl rand -hex 32)"
REDIS_PASSWORD="$(openssl rand -hex 32)"

kubectl -n patchmon create secret generic patchmon-app-secrets \
  --from-literal=JWT_SECRET="$JWT_SECRET" \
  --from-literal=SESSION_SECRET="$SESSION_SECRET" \
  --from-literal=AI_ENCRYPTION_KEY="$AI_ENCRYPTION_KEY" \
  --from-literal=POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
  --from-literal=REDIS_PASSWORD="$REDIS_PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -
```

2) Deploy:

```bash
helm upgrade --install patchmon ./helm/patchmon \
  -n patchmon --create-namespace \
  --wait --timeout 20m \
  -f ./helm/examples/values-local.yaml
```

---

## Deployment option: kind

```bash
helm upgrade --install patchmon ./helm/patchmon \
  -n patchmon --create-namespace \
  --wait --timeout 20m \
  -f ./helm/examples/values-kind.yaml
```

---

## Deployment option: persistent NFS (internal DB/Redis)

```bash
helm upgrade --install patchmon ./helm/patchmon \
  -n patchmon --create-namespace \
  --wait --timeout 20m \
  -f ./helm/examples/values-persistent-nfs.yaml
```

---

## Validation and quality checks

```bash
helm lint ./helm/patchmon
helm template patchmon ./helm/patchmon -f ./helm/examples/values-eks.yaml
helm template patchmon ./helm/patchmon -f ./helm/examples/values-local.yaml
```

---

## Day-2 operations

```bash
helm status patchmon -n patchmon
helm history patchmon -n patchmon
kubectl -n patchmon get pods,svc,ingress,pvc
kubectl -n patchmon logs deploy/patchmon-server --tail=200 -f
```

Rollback:

```bash
helm rollback patchmon <REVISION> -n patchmon
```

Uninstall:

```bash
helm uninstall patchmon -n patchmon
```

If persistence is enabled and you want a full reset:

```bash
kubectl -n patchmon delete pvc --all
```

---

## Update management (safe release flow)

1. Pin image tags in `helm/patchmon/values.yaml` (avoid mutable tags).
2. Bump `helm/patchmon/Chart.yaml`:
   - `version` for chart changes
   - `appVersion` for PatchMon app version
3. Run `helm lint` and `helm template` on all target profiles.
4. Upgrade with `--wait --timeout`.
5. Verify health, then promote to next environment.
6. Keep rollback revision handy (`helm history`).
