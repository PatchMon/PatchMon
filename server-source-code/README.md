# PatchMon Go Server

Go backend for PatchMon, a patch management monitoring system.

## Requirements

- **Go 1.25** or later
- **PostgreSQL** database (compatible with PatchMon schema)
- **Environment configuration** (see below)

## Building

```bash
# Build the binary (output: patchmon-server)
make build
```

Or using Go directly:

```bash
go build -o patchmon-server ./cmd/server
```

## Running

1. Copy the example environment file and configure it:

   ```bash
   cp .env.example .env
   # Edit .env with your database URL and JWT secret
   ```

2. Run the server:

   ```bash
   make run
   ```

   Or build and run manually:

   ```bash
   ./patchmon-server
   ```

## Required Environment Variables

| Variable       | Description                          |
|----------------|--------------------------------------|
| `DATABASE_URL` | PostgreSQL connection string         |
| `JWT_SECRET`   | Secret key for JWT token signing     |

See `.env.example` for all options. The server listens on `PORT` (default: 3001).

### Agent Installation

For the install script (`/api/v1/hosts/install`) to serve agent binaries, set `AGENT_BINARIES_DIR` to a directory containing:

- `patchmon-agent-linux-amd64`, `patchmon-agent-linux-arm64`, `patchmon-agent-linux-386`, `patchmon-agent-linux-arm`
- `patchmon-agent-freebsd-amd64`, `patchmon-agent-freebsd-386`, `patchmon-agent-freebsd-arm64`, `patchmon-agent-freebsd-arm`

Example (when running from the PatchMon monorepo):

```bash
AGENT_BINARIES_DIR=/path/to/PatchMon/agents ./patchmon-server
```

If unset, the server looks for an `agents` subdirectory in the current working directory.

### Custom Branding (Logos, Favicon)

Custom logos are stored in the database and served via `GET /api/v1/settings/logos/{type}`. Default logos remain on disk in the frontend's assets directory. `ASSETS_DIR` is deprecated.

## Regenerating sqlc Code

If you modify SQL schema or queries under `internal/sqlc/`, regenerate the Go code:

```bash
make sqlc-generate
```

## Other Commands

| Command         | Description              |
|-----------------|--------------------------|
| `make test`     | Run tests                |
| `make fmt`      | Format code              |
| `make vet`      | Run go vet               |
| `make lint`     | Run golangci-lint        |
| `make clean`    | Remove built binary      |
