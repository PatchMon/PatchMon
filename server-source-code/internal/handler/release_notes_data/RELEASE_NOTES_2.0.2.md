# PatchMon V2.0.2 Release Notes

A focused fix release on top of 2.0.1.

## Fixes

- **Database deadlocks under concurrent agent reports**: fixed Postgres deadlocks that dropped agent reports on installations with many hosts. Reports are now ingested in deterministic order with bulk SQL, plus automatic retry. Per-host SQL round-trips collapsed from 2N+1 to 2.
- **Patching schedule timezone**: configured timezone is now honoured when scheduling patching jobs.
- **Agent reports failing with "Invalid request body"**: default `AGENT_UPDATE_BODY_LIMIT` raised from `2mb` to `5mb` so hosts with many packages no longer fail to update.
- **OIDC and other features broken behind a reverse proxy**: `TRUST_PROXY` now defaults to `true`. Most users run PatchMon behind a reverse proxy (Traefik, Caddy, nginx, NPM); the previous `false` default caused OIDC logins to fail and real client IPs to be lost. If you run PatchMon directly on a public IP without a reverse proxy, set `TRUST_PROXY=false` explicitly.
- **Docker healthcheck failing on non-default ports**: the container healthcheck now honours the `PORT` environment variable instead of hardcoding `3000`.

## Startup health check

If you run more than 50 active hosts, PatchMon now logs a warning at startup if `DB_CONNECTION_LIMIT` (default `30`) looks too small, with a recommended value calculated from your host count.

## Upgrade

No special steps. From your `docker-compose.yml` directory:

```bash
docker compose pull
docker compose up -d
```

Migrations run on startup.
