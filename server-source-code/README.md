# PatchMon Go Server

> **Note:** This README is for development and informational purposes only.
> The recommended and supported way to deploy PatchMon is via Docker.
> See [`docker/README.md`](../docker/README.md) for production deployment instructions.

Go backend for PatchMon — a patch management monitoring and management system.

## Overview

The PatchMon server is a single Go binary that serves both the REST API and the frontend. At build time, the compiled frontend (`frontend/dist/`) is embedded directly into the binary using Go's `embed` package. This means there is no separate web server, reverse proxy, or static file directory required — one binary handles everything and listens on port `3000` by default.

The server also:
- Runs database migrations automatically on startup
- Connects to PostgreSQL for persistent data storage
- Connects to Redis for background job queues (via [Asynq](https://github.com/hibiken/asynq)) and caching
- Serves agent binaries for installation on monitored hosts
- Proxies in-browser RDP connections via a `guacd` sidecar

## Requirements

- **Go 1.26.1** or later
- **PostgreSQL** database
- **Redis** server
- **Node.js** (to build the frontend before embedding — see [Building](#building))
- Environment configuration (see [Environment Variables](#environment-variables))

## Building

The binary embeds the compiled frontend. You must build the frontend first so that `cmd/server/static/frontend/dist/` is populated before running `go build`.

```bash
# 1. Build the frontend (from the repo root)
cd frontend
npm install
npm run build
cd ..

# 2. Copy the frontend dist into the embed path
cp -r frontend/dist server-source-code/cmd/server/static/frontend/dist

# 3. Build the server binary
cd server-source-code
make build
# Output: ./patchmon-server
```

Or using Go directly (step 3 only, after completing steps 1–2):

```bash
go build -o patchmon-server ./cmd/server
```

To cross-compile static Linux binaries for all supported architectures:

```bash
make build-linux
# Outputs: patchmon-server-linux-amd64, patchmon-server-linux-arm64,
#          patchmon-server-linux-386, patchmon-server-linux-arm
```

## Running

### With Docker (recommended)

See [`docker/README.md`](../docker/README.md).

### As a binary (advanced)

It is possible to run PatchMon directly as a binary without Docker, provided you have a PostgreSQL database and Redis server available. The binary is self-contained — no separate frontend server or static file directory is needed.

1. Build the binary (see [Building](#building) above).

2. Copy and configure the environment file:

   ```bash
   cp ../docker/env.example .env
   # Edit .env — at minimum set the values below
   ```

3. Run the server:

   ```bash
   ./patchmon-server
   ```

   The server will:
   - Apply any pending database migrations automatically
   - Connect to PostgreSQL and Redis
   - Serve the API and embedded frontend on port `3000`

   Access PatchMon at `http://localhost:3000` (or the host/port you configured).

## Environment Variables

The server reads configuration from a `.env` file in the working directory, or from environment variables directly. Use [`docker/env.example`](../docker/env.example) as the reference — it contains the full list with descriptions and defaults.

### Required

| Variable | Description |
|---|---|
| `DATABASE_URL` | PostgreSQL connection string, e.g. `postgresql://user:pass@localhost:5432/patchmon_db` |
| `POSTGRES_PASSWORD` | Database password (also used to construct `DATABASE_URL` if set via individual `POSTGRES_*` vars) |
| `REDIS_PASSWORD` | Redis password |
| `JWT_SECRET` | JWT signing secret — generate with `openssl rand -hex 64` |
| `SESSION_SECRET` | Session encryption secret — generate with `openssl rand -hex 64` |
| `AI_ENCRYPTION_KEY` | Encryption key for AI features — generate with `openssl rand -hex 64` |
| `CORS_ORIGIN` | Full URL(s) used to access PatchMon in the browser (comma-separated for multiple) |

### Key Optional Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `3000` | Port the HTTP server listens on |
| `REDIS_HOST` | `redis` | Redis hostname (`localhost` when running outside Docker) |
| `POSTGRES_HOST` | `database` | PostgreSQL hostname (`localhost` when running outside Docker) |
| `TRUST_PROXY` | `false` | Set to `true` when behind a reverse proxy |
| `TZ` | `UTC` | Timezone (IANA format, e.g. `Europe/London`) |

> When running the binary directly (not in Docker), set `REDIS_HOST=localhost` and `POSTGRES_HOST=localhost` (or the relevant hostnames), as the Docker service-name defaults will not resolve.

See [`docker/env.example`](../docker/env.example) for the complete reference.

### Agent Binaries

For the install script (`/api/v1/hosts/install`) to serve agent binaries, the prebuilt agent files must be present. By default the server looks for an `agents/` subdirectory in the current working directory. Override with `AGENT_BINARIES_DIR`:

```bash
AGENT_BINARIES_DIR=/path/to/agents ./patchmon-server
```

If running the patchmon-server on its own then download them from the release or build the agent files and place them in the directory. 

So your directory structure shyour agent sould be:

- ./patchmon-server (server binary you can run)
- ./agents/{your agents}
- ./.env (configured environment file)

Agent binary naming convention:
- `patchmon-agent-linux-amd64`, `patchmon-agent-linux-arm64`, `patchmon-agent-linux-386`, `patchmon-agent-linux-arm`
- `patchmon-agent-freebsd-amd64`, `patchmon-agent-freebsd-386`, `patchmon-agent-freebsd-arm64`, `patchmon-agent-freebsd-arm`

## Development

### Running in development mode

```bash
make run
```

This builds and immediately runs the binary. For live reload, use the Docker development environment instead:

```bash
docker compose -f docker/docker-compose.dev.yml up --watch
```

See [`docker/README.md`](../docker/README.md#development) for full development setup instructions.

### Regenerating sqlc Code

If you modify SQL schema or queries under `internal/sqlc/`, regenerate the Go code:

```bash
make sqlc-generate
```

### Database Migrations

Migrations run automatically on server startup. You can also run them manually:

```bash
DATABASE_URL=postgresql://... make migrate-up
DATABASE_URL=postgresql://... make migrate-down
DATABASE_URL=postgresql://... make migrate-version
```

## Make Targets

| Command | Description |
|---|---|
| `make build` | Build the server binary |
| `make build-migrate` | Build the standalone migrate CLI binary |
| `make build-linux` | Cross-compile server + migrate for Linux (amd64, 386, arm64, arm) |
| `make run` | Build and run the server |
| `make run-pprof` | Build and run with pprof enabled |
| `make test` | Run tests |
| `make test-coverage` | Run tests with HTML coverage report |
| `make fmt` | Format code |
| `make fmt-check` | Verify code is formatted (used in CI) |
| `make vet` | Run `go vet` |
| `make lint` | Run `golangci-lint` |
| `make check` | Run `fmt-check`, `vet`, `lint`, and `test` (pre-commit) |
| `make sqlc-generate` | Regenerate sqlc Go code from SQL |
| `make migrate-up` | Run migrations up (requires `DATABASE_URL`) |
| `make migrate-down` | Run migrations down (requires `DATABASE_URL`) |
| `make migrate-version` | Show current migration version (requires `DATABASE_URL`) |
| `make clean` | Remove build artifacts |
