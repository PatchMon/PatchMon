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
- `edge`: The latest development build with the most recent features and fixes. This tag may often be unstable and is intended only for testing and development purposes.

These tags are available for both backend and frontend images as they are versioned together.

## Quick Start

### Production Deployment

1. Download the [Docker Compose file](docker-compose.yml)
2. Set a database password in the file where it says:
   ```yaml
   environment:
     POSTGRES_PASSWORD: # CREATE A STRONG PASSWORD AND PUT IT HERE
   ```
3. Update the corresponding `DATABASE_URL` with your password in the backend service where it says:
   ```yaml
   environment:
     DATABASE_URL: postgresql://patchmon_user:REPLACE_YOUR_POSTGRES_PASSWORD_HERE@database:5432/patchmon_db
   ```
4. Set a Redis password in the Redis service command where it says:
   ```yaml
   command: redis-server --requirepass your-redis-password-here
   ```
   Note: The Redis service uses a hardcoded password in the command line for better reliability and to avoid environment variable parsing issues.
5. Update the corresponding `REDIS_PASSWORD` in the backend service where it says:
   ```yaml
   environment:
     REDIS_PASSWORD: your-redis-password-here
   ```
6. Generate a strong JWT secret. You can do this like so:
   ```bash
   openssl rand -hex 64
   ```
7. Set a JWT secret in the backend service where it says:
   ```yaml
   environment:
     JWT_SECRET: # CREATE A STRONG SECRET AND PUT IT HERE
   ```
8. Configure environment variables (see [Configuration](#configuration) section)
9. Start the application:
   ```bash
   docker compose up -d
   ```
10. Access the application at `http://localhost:3000`

## Updating

By default, the compose file uses the `latest` tag for both backend and frontend images.

This means you can update PatchMon to the latest version as easily as:

```bash
docker compose up -d --pull
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
   docker compose up -d --pull
   ```

> [!TIP]
> Check the [releases page](https://github.com/PatchMon/PatchMon/releases) for version-specific changes and migration notes.

## Configuration

### Environment Variables

#### Database Service

| Variable            | Description       | Default          |
| ------------------- | ----------------- | ---------------- |
| `POSTGRES_DB`       | Database name     | `patchmon_db`    |
| `POSTGRES_USER`     | Database user     | `patchmon_user`  |
| `POSTGRES_PASSWORD` | Database password | **MUST BE SET!** |

#### Redis Service

| Variable       | Description        | Default          |
| -------------- | ------------------ | ---------------- |
| `REDIS_PASSWORD` | Redis password    | **MUST BE SET!** |

> [!NOTE]
> The Redis service uses a hardcoded password in the command line (`redis-server --requirepass your-password`) instead of environment variables or configuration files. This approach eliminates parsing issues and provides better reliability. The password must be set in both the Redis command and the backend service environment variables.

#### Backend Service

##### Database Configuration

| Variable                   | Description                                          | Default                                          |
| -------------------------- | ---------------------------------------------------- | ------------------------------------------------ |
| `DATABASE_URL`             | PostgreSQL connection string                         | **MUST BE UPDATED WITH YOUR POSTGRES_PASSWORD!** |
| `PM_DB_CONN_MAX_ATTEMPTS`  | Maximum database connection attempts                 | `30`                                             |
| `PM_DB_CONN_WAIT_INTERVAL` | Wait interval between connection attempts in seconds | `2`                                              |

##### Database Connection Pool Configuration (Prisma)

| Variable              | Description                                                | Default |
| --------------------- | ---------------------------------------------------------- | ------- |
| `DB_CONNECTION_LIMIT` | Maximum number of database connections per instance        | `30`    |
| `DB_POOL_TIMEOUT`     | Seconds to wait for an available connection before timeout | `20`    |
| `DB_CONNECT_TIMEOUT`  | Seconds to wait for initial database connection            | `10`    |
| `DB_IDLE_TIMEOUT`     | Seconds before closing idle connections                    | `300`   |
| `DB_MAX_LIFETIME`     | Maximum lifetime of a connection in seconds                | `1800`  |

> [!TIP]
> The connection pool limit should be adjusted based on your deployment size:
> - **Small deployment (1-10 hosts)**: `DB_CONNECTION_LIMIT=15` is sufficient
> - **Medium deployment (10-50 hosts)**: `DB_CONNECTION_LIMIT=30` (default)
> - **Large deployment (50+ hosts)**: `DB_CONNECTION_LIMIT=50` or higher
> 
> Each connection pool serves one backend instance. If you have concurrent operations (multiple users, background jobs, agent checkins), increase the pool size accordingly.

##### Redis Configuration

| Variable        | Description                    | Default |
| --------------- | ------------------------------ | ------- |
| `REDIS_HOST`    | Redis server hostname          | `redis` |
| `REDIS_PORT`    | Redis server port              | `6379`  |
| `REDIS_PASSWORD` | Redis authentication password | **MUST BE UPDATED WITH YOUR REDIS_PASSWORD!** |
| `REDIS_DB`      | Redis database number          | `0`     |

##### Authentication & Security

| Variable                             | Description                                               | Default          |
| ------------------------------------ | --------------------------------------------------------- | ---------------- |
| `JWT_SECRET`                         | JWT signing secret - Generate with `openssl rand -hex 64` | **MUST BE SET!** |
| `JWT_EXPIRES_IN`                     | JWT token expiration time                                 | `1h`             |
| `JWT_REFRESH_EXPIRES_IN`             | JWT refresh token expiration time                         | `7d`             |
| `SESSION_INACTIVITY_TIMEOUT_MINUTES` | Session inactivity timeout in minutes                     | `30`             |
| `DEFAULT_USER_ROLE`                  | Default role for new users                                | `user`           |

##### Server & Network Configuration

| Variable          | Description                                                                                     | Default                 |
| ----------------- | ----------------------------------------------------------------------------------------------- | ----------------------- |
| `PORT`            | Backend API port                                                                                | `3001`                  |
| `SERVER_PROTOCOL` | Frontend server protocol (`http` or `https`)                                                    | `http`                  |
| `SERVER_HOST`     | Frontend server host                                                                            | `localhost`             |
| `SERVER_PORT`     | Frontend server port                                                                            | `3000` (Change to 443 if using a FQDN in `SERVER_HOST` variable)|
| `CORS_ORIGIN`     | CORS origin URL                                                                                 | `http://localhost:3000` |
| `ENABLE_HSTS`     | Enable HTTP Strict Transport Security                                                           | `true`                  |
| `TRUST_PROXY`     | Trust proxy headers - See [Express.js docs](https://expressjs.com/en/guide/behind-proxies.html) | `true`                  |

##### Rate Limiting

| Variable                     | Description                                         | Default  |
| ---------------------------- | --------------------------------------------------- | -------- |
| `RATE_LIMIT_WINDOW_MS`       | Rate limiting window in milliseconds                | `900000` |
| `RATE_LIMIT_MAX`             | Maximum requests per window                         | `5000`   |
| `AUTH_RATE_LIMIT_WINDOW_MS`  | Authentication rate limiting window in milliseconds | `600000` |
| `AUTH_RATE_LIMIT_MAX`        | Maximum authentication requests per window          | `500`    |
| `AGENT_RATE_LIMIT_WINDOW_MS` | Agent API rate limiting window in milliseconds      | `60000`  |
| `AGENT_RATE_LIMIT_MAX`       | Maximum agent requests per window                   | `1000`   |

##### Logging

| Variable         | Description                                      | Default |
| ---------------- | ------------------------------------------------ | ------- |
| `LOG_LEVEL`      | Logging level (`debug`, `info`, `warn`, `error`) | `info`  |
| `ENABLE_LOGGING` | Enable application logging                       | `true`  |

#### Frontend Service

| Variable       | Description              | Default   |
| -------------- | ------------------------ | --------- |
| `BACKEND_HOST` | Backend service hostname | `backend` |
| `BACKEND_PORT` | Backend service port     | `3001`    |

### Volumes

The compose file creates three Docker volumes:

* `postgres_data`: PostgreSQL's data directory.
* `redis_data`: Redis's data directory.
* `agent_files`: PatchMon's agent files.

If you wish to bind either if their respective container paths to a host path rather than a Docker volume, you can do so in the Docker Compose file.

> [!TIP]
> The backend container runs as user & group ID 1000. If you plan to re-bind the agent files directory, ensure that the same user and/or group ID has permission to write to the host path to which it's bound.

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
docker build -f docker/backend.Dockerfile --target development -t patchmon-backend:dev .
docker build -f docker/frontend.Dockerfile --target development -t patchmon-frontend:dev .

# Build production images (default target)
docker build -f docker/backend.Dockerfile -t patchmon-backend:latest .
docker build -f docker/frontend.Dockerfile -t patchmon-frontend:latest .
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
