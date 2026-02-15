# PatchMon Docker

## Overview

PatchMon is a containerised application that monitors system patches and updates. The application consists of four main services:

- **Database**: PostgreSQL 17
- **Redis**: Redis 7 for BullMQ job queues and caching
- **Backend**: Node.js API server
- **Frontend**: React application served via NGINX

## Images

- **Backend**: [ghcr.io/patchmon/patchmon-backend](https://github.com/patchmon/patchmon.net/pkgs/container/patchmon-backend)
- **Frontend**: [ghcr.io/patchmon/patchmon-frontend](https://github.com/patchmon/patchmon.net/pkgs/container/patchmon-frontend)

### Tags

- `latest`: The latest stable release of PatchMon
- `x.y.z`: Full version tags (e.g. `1.2.3`) - Use this for exact version pinning.
- `x.y`: Minor version tags (e.g. `1.2`) - Use this to get the latest patch release in a minor version series.
- `x`: Major version tags (e.g. `1`) - Use this to get the latest minor and patch release in a major version series.
- `edge`: The latest development build in main branch. This tag may often be unstable and is intended only for testing and development purposes.

These tags are available for both backend and frontend images as they are versioned together.

## Quick Start

### Production Deployment

1. Download the Docker Compose file and environment example:
   ```bash
   mkdir patchmon && cd patchmon
   curl -fsSL -o docker-compose.yml https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/docker/docker-compose.yml
   curl -fsSL -o env.example https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/docker/env.example
   ```

2. Create your `.env` file from the example:
   ```bash
   cp env.example .env
   ```

3. Generate and insert the three required secrets:
   ```bash
   sed -i "s/^POSTGRES_PASSWORD=$/POSTGRES_PASSWORD=$(openssl rand -hex 32)/" .env
   sed -i "s/^REDIS_PASSWORD=$/REDIS_PASSWORD=$(openssl rand -hex 32)/" .env
   sed -i "s/^JWT_SECRET=$/JWT_SECRET=$(openssl rand -hex 64)/" .env
   ```

4. Edit `.env` and configure your server access settings (`SERVER_PROTOCOL`, `SERVER_HOST`, `SERVER_PORT`, `CORS_ORIGIN`). The defaults are set for `http://localhost:3000`.

5. Start the application:
   ```bash
   docker compose up -d
   ```

6. Access the application at `http://localhost:3000`

The `docker-compose.yml` reads all configuration from your `.env` file. You do not need to edit the compose file itself.

## Updating

By default, the compose file uses the `latest` tag for both backend and frontend images.

This means you can update PatchMon to the latest version as easily as:

```bash
docker compose pull
docker compose up -d
```

This command will:
- Pull the latest images from the registry
- Recreate containers with updated images
- Maintain your data and configuration

### Version-Specific Updates

If you'd like to pin your Docker deployment of PatchMon to a specific version, you can do this in the compose file.

When you do this, updating to a new version requires manually updating the image tags in the compose file yourself:

1. Update the image tags in `docker-compose.yml`. For example:
   ```yaml
   services:
     backend:
       image: ghcr.io/patchmon/patchmon-backend:1.2.3  # Update version here
      ...
     frontend:
       image: ghcr.io/patchmon/patchmon-frontend:1.2.3  # Update version here
      ...
   ```

2. Then run the update command:
   ```bash
   docker compose pull
   docker compose up -d
   ```

> [!TIP]
> Check the [releases page](https://github.com/PatchMon/PatchMon/releases) for version-specific changes and migration notes.

## Configuration

All configuration is managed through the `.env` file. See `env.example` for a full list of available variables.

### Required Variables

| Variable | Description |
| -------- | ----------- |
| `POSTGRES_PASSWORD` | Database password |
| `REDIS_PASSWORD` | Redis password |
| `JWT_SECRET` | JWT signing secret - Generate with `openssl rand -hex 64` |
| `SERVER_PROTOCOL` | Protocol for agent connections (`http` or `https`) |
| `SERVER_HOST` | Hostname for agent connections |
| `SERVER_PORT` | Port for agent connections |
| `CORS_ORIGIN` | Full URL used to access PatchMon in the browser |

### Optional Variables

The `.env` file also supports optional variables for fine-tuning. These have sensible defaults and do not need to be changed for most deployments:

- **Authentication**: `JWT_EXPIRES_IN`, `JWT_REFRESH_EXPIRES_IN`, `SESSION_INACTIVITY_TIMEOUT_MINUTES`, `DEFAULT_USER_ROLE`
- **Account lockout**: `MAX_LOGIN_ATTEMPTS`, `LOCKOUT_DURATION_MINUTES`
- **Password policy**: `PASSWORD_MIN_LENGTH`, `PASSWORD_REQUIRE_UPPERCASE`, `PASSWORD_REQUIRE_LOWERCASE`, `PASSWORD_REQUIRE_NUMBER`, `PASSWORD_REQUIRE_SPECIAL`
- **Two-Factor Authentication**: `MAX_TFA_ATTEMPTS`, `TFA_LOCKOUT_DURATION_MINUTES`, `TFA_REMEMBER_ME_EXPIRES_IN`, `TFA_MAX_REMEMBER_SESSIONS`
- **OIDC / SSO**: `OIDC_ENABLED`, `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_REDIRECT_URI`, `OIDC_SCOPES`, and more
- **Encryption**: `AI_ENCRYPTION_KEY`, `SESSION_SECRET`
- **Database pool (Prisma)**: `DB_CONNECTION_LIMIT`, `DB_POOL_TIMEOUT`, `DB_CONNECT_TIMEOUT`, `DB_IDLE_TIMEOUT`, `DB_MAX_LIFETIME`
- **Database transaction timeouts**: `DB_TRANSACTION_MAX_WAIT`, `DB_TRANSACTION_TIMEOUT`, `DB_TRANSACTION_LONG_TIMEOUT`
- **Database connection retry**: `PM_DB_CONN_MAX_ATTEMPTS`, `PM_DB_CONN_WAIT_INTERVAL`
- **Rate limiting**: `RATE_LIMIT_WINDOW_MS`, `RATE_LIMIT_MAX`, `AUTH_RATE_LIMIT_WINDOW_MS`, `AUTH_RATE_LIMIT_MAX`, `AGENT_RATE_LIMIT_WINDOW_MS`, `AGENT_RATE_LIMIT_MAX`
- **Logging**: `LOG_LEVEL`, `ENABLE_LOGGING`, `PM_LOG_TO_CONSOLE`, `PRISMA_LOG_QUERIES`
- **Network**: `ENABLE_HSTS`, `TRUST_PROXY`, `CORS_ORIGINS`
- **Body size limits**: `JSON_BODY_LIMIT`, `AGENT_UPDATE_BODY_LIMIT`
- **Timezone**: `TZ`

See `env.example` for the full list with defaults and descriptions.

> [!TIP]
> The connection pool limit should be adjusted based on your deployment size:
> - **Small deployment (1-10 hosts)**: `DB_CONNECTION_LIMIT=15` is sufficient
> - **Medium deployment (10-50 hosts)**: `DB_CONNECTION_LIMIT=30` (default)
> - **Large deployment (50+ hosts)**: `DB_CONNECTION_LIMIT=50` or higher
> 
> Each connection pool serves one backend instance. If you have concurrent operations (multiple users, background jobs, agent checkins), increase the pool size accordingly.

### Volumes

The compose file creates four Docker volumes:

* `postgres_data`: PostgreSQL's data directory.
* `redis_data`: Redis's data directory.
* `agent_files`: PatchMon's agent files.
* `branding_assets`: Custom branding files (logos, favicons) - optional, new in 1.4.0.

If you wish to bind any of their respective container paths to a host path rather than a Docker volume, you can do so in the Docker Compose file.

> [!TIP]
> The backend container runs as user & group ID 1000. If you plan to rebind the agent files or branding assets directory, ensure that the same user and/or group ID has permission to write to the host path to which it's bound.

---

## Docker Swarm Deployment

> [!NOTE]
> This section covers deploying PatchMon to a Docker Swarm cluster. For standard Docker Compose deployments on a single host, use the production deployment guide above.

### Network Configuration

When deploying to Docker Swarm with a reverse proxy (e.g., Traefik), proper network configuration is critical. The default `docker-compose.yml` uses an internal bridge network that enables service-to-service communication:

```yaml
networks:
  patchmon-internal:
    driver: bridge
```

All services (database, redis, backend, and frontend) connect to this internal network, allowing them to discover each other by service name.

**Important**: If you're using an external reverse proxy network (like `traefik-net`), ensure that:

1. All PatchMon services remain on the `patchmon-internal` network for internal communication
2. The frontend service (NGINX) can be configured to also bind to the reverse proxy network if needed
3. Service names resolve correctly within the same network

### Service Discovery in Swarm

In Docker Swarm, service discovery works through:
- **Service Name Resolution**: Service names resolve to virtual IPs within the same network
- **Load Balancing**: Requests to a service name are automatically load-balanced across all replicas
- **Network Isolation**: Services on different networks cannot communicate directly

### Configuration for Swarm with Traefik

If you're using Traefik as a reverse proxy:

1. Keep the default `patchmon-internal` network for backend services
2. Configure Traefik in your Swarm deployment with its own network
3. Ensure the frontend service can reach the backend through the internal network

Example modification for Swarm:

```yaml
services:
  frontend:
    image: ghcr.io/patchmon/patchmon-frontend:latest
    networks:
      - patchmon-internal
    deploy:
      replicas: 1
      labels:
        - "traefik.enable=true"
        - "traefik.http.routers.patchmon.rule=Host(`patchmon.my.domain`)"
        # ... other Traefik labels
```

The frontend reaches the backend via the `patchmon-internal` network using the hostname `backend`, while Traefik routes external traffic to the frontend service.

### Troubleshooting Network Issues

**Error: `host not found in upstream "backend"`**

This typically occurs when:
1. Frontend and backend services are on different networks
2. Services haven't fully started (check health checks)
3. Service names haven't propagated through DNS

**Solution**:
- Verify all services are on the same internal network
- Check service health status: `docker ps` (production) or `docker service ps` (Swarm)
- Wait for health checks to pass before accessing the application
- Confirm network connectivity: `docker exec <container> ping backend`

---

# Development

This section is for developers who want to contribute to PatchMon or run it in development mode.

## Development Setup

For development with live reload and source code mounting:

1. Clone the repository:
   ```bash
   git clone https://github.com/PatchMon/PatchMon.git
   cd PatchMon
   ```

2. Start development environment:
   ```bash
   docker compose -f docker/docker-compose.dev.yml up
   ```
   _See [Development Commands](#development-commands) for more options._

3. Access the application:
   - Frontend: `http://localhost:3000`
   - Backend API: `http://localhost:3001`
   - Database: `localhost:5432`
   - Redis: `localhost:6379`

## Development Docker Compose

The development compose file (`docker/docker-compose.dev.yml`):
- Builds images locally from source using development targets
- Enables hot reload with Docker Compose watch functionality
- Exposes database and backend ports for testing and development
- Mounts source code directly into containers for live development
- Supports debugging with enhanced logging

## Building Images Locally

Both Dockerfiles use multi-stage builds with separate development and production targets:

```bash
# Build development images
docker build -f docker/backend.Dockerfile --target development --provenance=false --sbom=false -t patchmon-backend:dev .
docker build -f docker/frontend.Dockerfile --target development --provenance=false --sbom=false -t patchmon-frontend:dev .

# Build production images (default target)
docker build -f docker/backend.Dockerfile --provenance=false --sbom=false -t patchmon-backend:latest .
docker build -f docker/frontend.Dockerfile --provenance=false --sbom=false -t patchmon-frontend:latest .
```

## Development Commands

### Hot Reload Development
```bash
# Attached, live log output, services stopped on Ctrl+C
docker compose -f docker/docker-compose.dev.yml up

# Attached with Docker Compose watch for hot reload
docker compose -f docker/docker-compose.dev.yml up --watch

# Detached
docker compose -f docker/docker-compose.dev.yml up -d

# Quiet, no log output, with Docker Compose watch for hot reload
docker compose -f docker/docker-compose.dev.yml watch
```

### Rebuild Services
```bash
# Rebuild specific service
docker compose -f docker/docker-compose.dev.yml up -d --build backend

# Rebuild all services
docker compose -f docker/docker-compose.dev.yml up -d --build
```

### Development Ports
The development setup exposes additional ports for debugging:
- **Database**: `5432` - Direct PostgreSQL access
- **Redis**: `6379` - Direct Redis access
- **Backend**: `3001` - API server with development features
- **Frontend**: `3000` - React development server with hot reload

## Development Workflow

1. **Initial Setup**: Clone repository and start development environment
   ```bash
   git clone https://github.com/PatchMon/PatchMon.git
   cd PatchMon
   docker compose -f docker/docker-compose.dev.yml up -d --build
   ```

2. **Hot Reload Development**: Use Docker Compose watch for automatic reload
   ```bash
   docker compose -f docker/docker-compose.dev.yml up --watch --build
   ```

3. **Code Changes**: 
   - **Frontend/Backend Source**: Files are synced automatically with watch mode
   - **Package.json Changes**: Triggers automatic service rebuild
   - **Prisma Schema Changes**: Backend service restarts automatically

4. **Database Access**: Connect database client directly to `localhost:5432`
5. **Redis Access**: Connect Redis client directly to `localhost:6379`
6. **Debug**: If started with `docker compose [...] up -d` or `docker compose [...] watch`, check logs manually:
   ```bash
   docker compose -f docker/docker-compose.dev.yml logs -f
   ```
   Otherwise logs are shown automatically in attached modes (`up`, `up --watch`).

### Features in Development Mode

- **Hot Reload**: Automatic code synchronization and service restarts
- **Enhanced Logging**: Detailed logs for debugging
- **Direct Access**: Exposed ports for database, Redis, and API debugging  
- **Health Checks**: Built-in health monitoring for services
- **Volume Persistence**: Development data persists between restarts
