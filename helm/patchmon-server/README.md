# PatchMon Helm Chart

A production-ready Helm chart for deploying PatchMon, a comprehensive Linux patch monitoring and management system.

## Features

- Complete Stack Deployment: PostgreSQL 18-alpine, Redis 8-alpine, PatchMon Server, and Guacamole daemon (guacd)
- Highly Configurable: Extensive values.yaml with sensible defaults
- Security First: Non-root containers, user-provided secrets, seccomp profiles, minimal capabilities
- Auto-scaling: HPA support for server (not recommended, see note above)
- Flexible Naming: Global name overrides for multi-tenant deployments
- Persistent Storage: Configurable storage classes and sizes
- Dependency Management: Built-in init containers for service dependencies with security contexts
- Ingress Support: TLS and cert-manager integration
- Resource Management: Configurable resource limits and requests
- Production Ready: Complies with Kubernetes restricted PodSecurity policy

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- PV provisioner support in the underlying infrastructure (for persistent volumes)
- (Optional) cert-manager for automatic TLS certificate management
- (Optional) Metrics Server for HPA functionality

## Installation

### Quick Start

```bash
# Install from OCI registry with default values (latest version)
helm install patchmon oci://ghcr.io/patchmon/helm/patchmon-server \
  --namespace patchmon \
  --create-namespace

# Install with custom values
helm install patchmon oci://ghcr.io/patchmon/helm/patchmon-server \
  --namespace patchmon \
  --create-namespace \
  --values custom-values.yaml

# Or install a specific version
helm install patchmon oci://ghcr.io/patchmon/helm/patchmon-server \
  --version 2.0.3 \
  --namespace patchmon \
  --create-namespace

# Pull the chart first, then install
helm pull oci://ghcr.io/patchmon/helm/patchmon-server --untar
helm install patchmon ./patchmon -n patchmon --create-namespace -f custom-values.yaml
```

**Note**: Browse available versions at https://github.com/RuTHlessBEat200/PatchMon-helm/releases

### Production Deployment

**Secret Management Best Practices:**

The chart supports integration with secure secret management tools:

- **[KSOPS](https://github.com/viaduct-ai/kustomize-sops)** - Encrypt secrets in Git using Mozilla SOPS
- **[Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets)** - Encrypt secrets that only the cluster can decrypt
- **[External Secrets Operator](https://external-secrets.io/)** - Sync secrets from external secret stores (Vault, AWS Secrets Manager, etc.)
- **[Vault](https://www.vaultproject.io/)** - Enterprise-grade secret management

**Production example** ([values-prod.yaml](examples/values-prod.yaml)):

```yaml
global:
  storageClass: "proxmox-data"

fullnameOverride: "patchmon-prod"

# Use KSOPS to manage secrets in production or other secure methods

server:
  env:
    serverProtocol: https
    serverHost: patchmon.example.com
    serverPort: "443"
    corsOrigin: https://patchmon.example.com
  existingSecret: "patchmon-secrets"
  existingSecretJwtKey: "jwt-secret"
  existingSecretAiEncryptionKey: "ai-encryption-key"
  oidc:
    enabled: false
    existingSecretClientSecretKey: "oidc-client-secret"

database:
  auth:
    existingSecret: patchmon-secrets
    existingSecretPasswordKey: postgres-password

redis:
  auth:
    existingSecret: patchmon-secrets
    existingSecretPasswordKey: redis-password

secret:
  create: false

ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: patchmon.example.com
      paths:
        - path: /
          pathType: Prefix
          service:
            name: server
            port: 3000
  tls:
    - secretName: patchmon-tls
      hosts:
        - patchmon.example.com
```

**Deploy with production values:**

```bash
helm install patchmon oci://ghcr.io/patchmon/helm/patchmon-server \
  --namespace patchmon \
  --create-namespace \
  --values examples/values-prod.yaml
```

## Configuration

### Global Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `global.imageRegistry` | Global Docker registry override (takes priority over component registries) | `""` |
| `global.imagePullSecrets` | Global image pull secrets | `[]` |
| `global.storageClass` | Global storage class for all PVCs | `""` |
| `nameOverride` | Override chart name | `""` |
| `fullnameOverride` | Override full resource names | `""` |

### Database Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `database.enabled` | Enable PostgreSQL deployment | `true` |
| `database.image.repository` | PostgreSQL image repository | `postgres` |
| `database.image.tag` | PostgreSQL image tag | `18-alpine` |
| `database.host` | External database host (disables internal deployment) | `nil` |
| `database.port` | External database port | `nil` |
| `database.auth.database` | Database name | `patchmon_db` |
| `database.auth.username` | Database user | `patchmon_user` |
| `database.auth.password` | Database password (**must be set or use existingSecret**) | `""` |
| `database.persistence.size` | Database PVC size | `5Gi` |
| `database.persistence.storageClass` | Storage class for database | Uses global |
| `database.updateStrategy.type` | StatefulSet update strategy | `RollingUpdate` |
| `database.resources.requests.memory` | Memory request | `128Mi` |
| `database.resources.limits.memory` | Memory limit | `1Gi` |

### Redis Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `redis.enabled` | Enable Redis deployment | `true` |
| `redis.image.repository` | Redis image repository | `redis` |
| `redis.image.tag` | Redis image tag | `8-alpine` |
| `redis.auth.password` | Redis password (**must be set or use existingSecret**) | `""` |
| `redis.persistence.size` | Redis PVC size | `5Gi` |
| `redis.persistence.storageClass` | Storage class for Redis | Uses global |
| `redis.updateStrategy.type` | StatefulSet update strategy | `RollingUpdate` |
| `redis.resources.requests.memory` | Memory request | `10Mi` |
| `redis.resources.limits.memory` | Memory limit | `512Mi` |

### Server Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `server.enabled` | Enable server deployment | `true` |
| `server.image.registry` | Server image registry | `ghcr.io` |
| `server.image.repository` | Server image repository | `patchmon/patchmon-server` |
| `server.image.tag` | Server image tag | `2.0.2` |
| `server.replicaCount` | Number of server replicas (**keep at 1**, see note at top) | `1` |
| `server.jwtSecret` | JWT secret (**must be set or use existingSecret**) | `""` |
| `server.aiEncryptionKey` | AI encryption key (**must be set or use existingSecret**) | `""` |
| `server.env.serverProtocol` | Server protocol | `http` |
| `server.env.serverHost` | Server hostname | `patchmon.example.com` |
| `server.env.serverPort` | Server port | `80` |
| `server.env.corsOrigin` | CORS origin URL | `http://patchmon.example.com` |
| `server.autoscaling.enabled` | Enable HPA (not recommended, see note at top) | `false` |
| `server.autoscaling.minReplicas` | Minimum replicas | `1` |
| `server.autoscaling.maxReplicas` | Maximum replicas | `10` |
| `server.resources.requests.memory` | Memory request | `256Mi` |
| `server.resources.limits.memory` | Memory limit | `2Gi` |

### Guacd Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `guacd.enabled` | Enable Guacamole proxy daemon | `true` |
| `guacd.image.repository` | guacd image repository | `guacamole/guacd` |
| `guacd.image.tag` | guacd image tag | `latest` |
| `guacd.service.port` | guacd service port | `4822` |
| `guacd.resources.requests.memory` | Memory request | `32Mi` |
| `guacd.resources.limits.memory` | Memory limit | `512Mi` |

### Ingress Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `ingress.enabled` | Enable ingress | `true` |
| `ingress.className` | Ingress class name | `""` |
| `ingress.annotations` | Ingress annotations | nginx proxy settings |
| `ingress.hosts` | Ingress hosts configuration | `patchmon.example.com` |
| `ingress.tls` | TLS configuration | Commented out by default |

## Advanced Configuration

### Multi-Tenant Deployment

Deploy multiple instances with name overrides:

```yaml
fullnameOverride: "patchmon-tenant1"

server:
  env:
    serverHost: tenant1.patchmon.example.com
    corsOrigin: https://tenant1.patchmon.example.com

ingress:
  hosts:
    - host: tenant1.patchmon.example.com
      # ... rest of config
```

### Custom Image Registry

Use a custom registry for all images:

```yaml
global:
  imageRegistry: "registry.example.com"
```

This will override component-specific registries and pull all images from your registry:
- `registry.example.com/postgres:18-alpine`
- `registry.example.com/redis:8-alpine`
- `registry.example.com/patchmon/patchmon-server:2.0.2`
- `registry.example.com/guacamole/guacd:latest`
- `registry.example.com/busybox:latest` (init containers)

Without `global.imageRegistry`, components use their default registries:
- Database/Redis: `docker.io`
- Server/guacd: `ghcr.io` / `docker.io`

### External Secrets

Use existing secrets instead of setting values directly:

```yaml
database:
  auth:
    existingSecret: "my-db-secret"
    existingSecretPasswordKey: "password"

redis:
  auth:
    existingSecret: "my-redis-secret"
    existingSecretPasswordKey: "password"

server:
  existingSecret: "my-server-secret"
  existingSecretJwtKey: "jwt-secret"
  existingSecretAiEncryptionKey: "ai-encryption-key"
  oidc:
    existingSecretClientSecretKey: "oidc-client-secret"
```

### Disable Components

```yaml
# Use an external database
database:
  enabled: false

server:
  env:
    # Configure external database connection via DATABASE_URL or host/port overrides
```

## Upgrading

```bash
# Upgrade with new values
helm upgrade patchmon oci://ghcr.io/patchmon/helm/patchmon-server \
  -n patchmon \
  -f values.yaml

# Upgrade and wait for rollout
helm upgrade patchmon oci://ghcr.io/patchmon/helm/patchmon-server \
  -n patchmon \
  -f values.yaml \
  --wait --timeout 10m
```

**Secret Handling on Upgrades:**

Secrets must be set explicitly in your values files or managed externally. The chart will fail to install or upgrade if required secrets are not provided.

## Uninstalling

```bash
# Uninstall the release
helm uninstall patchmon -n patchmon

# Clean up PVCs (if needed)
kubectl delete pvc -n patchmon -l app.kubernetes.io/instance=patchmon
```

## Troubleshooting

### Check Pod Status

```bash
kubectl get pods -n patchmon
kubectl describe pod <pod-name-n patchmon
kubectl logs <pod-name-n patchmon
```

### Check Init Container Logs

```bash
kubectl logs <pod-name-n patchmon -c wait-for-database
kubectl logs <pod-name-n patchmon -c wait-for-redis
kubectl logs <pod-name-n patchmon -c wait-for-guacd
```

### Check Service Connectivity

```bash
# Test database connection
kubectl exec -n patchmon -it statefulset/patchmon-server -- nc -zv patchmon-database 5432

# Test Redis connection
kubectl exec -n patchmon -it statefulset/patchmon-server -- nc -zv patchmon-redis 6379

# Check server health
kubectl exec -n patchmon -it statefulset/patchmon-server -- wget -qO- http://localhost:3000/health
```

### Common Issues

1. **Pods stuck in Init state**: Check if database, redis, and guacd services are running
2. **PVC binding issues**: Verify storage class is available: `kubectl get sc`
3. **Image pull errors**: Check image registry credentials and `imagePullSecrets`
4. **Ingress not working**: Verify ingress controller is installed and cert-manager is configured
5. **Agents appear offline**: Ensure `server.replicaCount` is `1` — see the note at the top of this document

## Development

### Lint the Chart

```bash
helm lint ./patchmon
```

### Template Rendering

```bash
# Render templates with default values
helm template patchmon ./patchmon

# Render with custom values
helm template patchmon ./patchmon -f custom-values.yaml

# Debug template rendering
helm template patchmon ./patchmon --debug
```

### Dry Run Installation

```bash
helm install patchmon ./patchmon -n patchmon --dry-run --debug
```

## License

This Helm chart is distributed under the same license as PatchMon.

## Support

For issues and questions:
- GitHub Issues: https://github.com/patchmon/patchmon/issues
- Documentation: https://patchmon.net/docs

## Contributing

Contributions are welcome! Please submit pull requests or issues on GitHub.
