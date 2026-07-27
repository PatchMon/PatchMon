# PatchMon V2.0.3 Release Notes

A performance, reliability and security release, load tested against fleets of 1,000+ hosts.

The headline change is hash-gated agent check-in, which cuts steady-state agent traffic by roughly three orders of magnitude. Alongside it: a rebuilt Agent Activity view, server-side pagination on the big list pages, explicit SMTP TLS modes, and a fix for client IP spoofing behind a reverse proxy.

## Highlights

### Hash-gated agent check-in

Agents used to send their full inventory on every cycle, whether or not anything had changed. From 2.0.3 the agent computes a SHA-256 hash of each content section (packages, repositories, network interfaces, hostname, Docker, compliance) and sends only those hashes on check-in. The server compares them against the hashes it already holds and asks for full content only for the sections that actually changed.

A steady-state hourly check-in drops from roughly a 2 MB POST to a roughly 1 KB ping plus a roughly 200 B response. Across a large fleet that is a significant reduction in both network traffic and database write volume.

Older agents keep working without changes: they fall through to the legacy heartbeat path and continue sending full reports. Update your agents to pick up the benefit.

### Agent Activity tab

The separate **Package Reports** and **Agent Queue** tabs on the host detail page are replaced by a single **Agent Activity** tab: one timeline of every agent communication cycle.

- Rows are typed as **Ping**, **Full**, **Partial**, **Docker**, **Compliance**, or **Job**.
- Each report row shows section chips: green "Updated" for sections the agent sent fresh data for, muted "Skipped" for sections the server already had a matching hash for.
- The Waiting / Active / Delayed / Failed queue stat cards sit above the table and reflect in-flight server-to-agent jobs.
- Bookmarks against the old `?tab=history` and `?tab=queue` query parameters redirect to `?tab=activity`.

Retention is controlled by the new `AGENT_REPORTS_RETENTION_DAYS` variable (default 30 days, range 7 to 365). A daily sweep at 02:00 removes anything older.

### Host status pills

The host detail and hosts list pages now show four independent status pills, each with a hover tooltip, replacing the previous single status chip plus WebSocket badge:

- **WS**: WebSocket control channel state. Green ("WSS") when connected, amber ("WSS reconnecting") while disconnected inside the grace window, red ("WSS offline") once it elapses. The label and icon both change, so the state does not rely on colour alone.
- **Reporting**: report freshness. Green when the agent has reported within its update interval, amber when overdue but the WebSocket is still up, red ("Stale") when overdue and the WebSocket is also down.
- **Reboot pending**: shown only when the host has flagged a pending reboot.
- **Updates**: green "Up to date", amber "Updates pending", red "Security patches required".

The practical gain is that a dropped WebSocket no longer looks identical to a dead host. If **WS** is red but **Reporting** is green, the host is alive and pushing reports, and only the real-time channel is unavailable.

Uptime is now computed live from the host's boot time rather than rendered from a pre-formatted string captured at report time, so it no longer goes stale between reports.

The `host_down` alert is renamed **Host Agent Down** (and `host_recovered` to **Host Agent Recovered**) to reflect what it actually measures, and it now decides using the WebSocket control channel rather than report age alone:

- A host with a **live WebSocket** is never reported down, however long ago it last sent a report.
- A host whose **WebSocket has dropped** is reported down once the disconnect exceeds the configurable threshold, which is expressed in **seconds** (default 30). The Alert Lifecycle table shows a `sec` suffix on that row to distinguish it from the count-based thresholds.
- A host the server has **no WebSocket information for at all** falls back to report cadence, and is reported down after three missed update intervals, which is the pre-2.0.3 behaviour.

This split matters because the two signals move on completely different timescales. The WebSocket reflects reachability within seconds, whilst reports arrive on the update interval (60 minutes by default). Alerting on a 30-second threshold against report age would flag your entire fleet continuously.

## Performance

Load testing at 1,000+ hosts with active connections drove the following:

- **Packages list page**: a new `mv_package_stats` materialised view serves the per-package install, update and security counters. This replaces an aggregation across roughly 1.3 million `host_packages` rows on every request, taking the page from around 10 seconds to sub-millisecond for that join. The view refreshes every 2 minutes via the job queue, using `REFRESH ... CONCURRENTLY` so readers are not blocked.
- **Package search no longer 500s in Docker**: searching packages fell back to a parallel sequential scan, which hit the default 64 MB `/dev/shm` ceiling in Docker and failed with `could not resize shared memory segment`. A trigram index on `packages.description` completes the index coverage for the search predicate.
- **Four new partial and covering indexes** on `host_packages` for the dashboard, hosts list and packages list query shapes.
- **`work_mem` is raised to 32 MB** for the transactions running large aggregations. The default 4 MB forced an external on-disk sort at fleet scale. If your connection pooler refuses the setting, the query still runs, just without the benefit.
- **Server-side pagination** on the Hosts, Packages and Repositories pages, with a selectable page size that persists between visits. Hosts offers 25 / 50 / 100 / 200 / 500 (default 50); Packages and Repositories offer 25 / 50 / 100 / 200.
- **New dashboard endpoints** for navigation counts and host filter options, so the sidebar and filter controls no longer piggyback on the full host list payload.

## Patch run reliability

- **Stuck runs are now cleaned up.** A run left in `running` past a configurable stall timeout is marked `timed_out`. The timeout is `PATCH_RUN_STALL_TIMEOUT_MIN` (default 30 minutes, minimum 5). The cleanup sweep now runs every 10 minutes rather than once daily at 00:30.
- **New `agent_disconnected` status.** If an agent's WebSocket drops while a run is in flight, the run is marked `agent_disconnected` instead of sitting at `running` indefinitely. If the agent reconnects and posts a late result for the same run, the row is updated to that final state.
- **Stop Run is now authoritative.** Cancelling applies the change in the database first, then sends a courtesy stop message to the agent if it is connected. Previously an offline or unresponsive agent could leave the row stuck in `running`.

## Email notifications

SMTP destinations now have an explicit **TLS mode** instead of a single "Use TLS" toggle:

- **STARTTLS (recommended)**: connect in plaintext, then require the server to advertise `STARTTLS` and upgrade before anything sensitive is sent. Refuses to send if the server does not advertise it. Typical port 587.
- **Implicit TLS / SSL**: TLS from the first byte. Typical port 465.
- **None (insecure)**: cleartext. PatchMon refuses to send if a username or password is set, rather than leaking credentials onto the wire.
- **Auto**: the previous opportunistic behaviour, kept for backward compatibility. It tries STARTTLS first and falls back to implicit TLS on the same host and port when STARTTLS is not advertised. It never falls back to cleartext.

Existing destinations load as **Auto** and keep working unchanged. Auto always encrypts, so nothing is sent in the clear either way, but it accepts whichever mode the server offers rather than enforcing the one you intended. Open each destination, select the mode your relay actually supports, and save, so a misconfigured or tampered-with server fails closed. Connections are pinned to a TLS 1.2 minimum and certificates are validated against the configured hostname.

Saved email destinations also gain a **Send test email** button. Unlike **Test**, which enqueues a synthetic notification through the worker, this runs a live SMTP probe from the API request and reports which stage failed: `validate`, `dial`, `starttls`, `auth`, or `send`, including the relay's own error message. This is the fastest way to diagnose a TLS or auth problem without reading server logs.

## Security

- **Client IP spoofing behind a reverse proxy is fixed.** PatchMon used chi's `RealIP` middleware, which takes the leftmost entry of `X-Forwarded-For`. That entry is whatever the client sent, so a caller could rotate their own rate-limit bucket and write an arbitrary IP into the audit log and login lockout records. Client IP is now resolved by a trusted-proxy-aware implementation, and the rate limiter reads the resolved value instead of re-parsing the header itself.
- **New `TRUSTED_PROXY_RANGES` variable.** Comma-separated CIDRs or bare IPs of the proxies in front of PatchMon. Leave it empty for a single reverse proxy, which is the usual setup and the correct value for the standard Docker deployment: PatchMon then uses the address your proxy appended, which a client cannot forge. Set it only when proxies are chained, for example Cloudflare in front of Nginx Proxy Manager. It is configurable by environment only and shown read-only in the settings UI, deliberately, since widening it to `0.0.0.0/0` would restore the spoofing problem.
- **New `AGENT_PING_BODY_LIMIT`** (default 8 KiB) caps bodies on the check-in endpoint so the cheap ping path cannot be abused as a payload sink.

## Fixes and maintenance

- **Frontend Docker builds are reproducible again.** The image built with `npm install --force` against a deleted lockfile, so it was free to resolve any version matching the semver ranges. That silently broke the build when `react-icons` 5.7.0 dropped an icon the app used. The image now builds from the committed lockfile with `npm ci`.
- **Consolidated migrations.** The seven in-development migrations for this release are squashed into a single migration, so the 2.0.2 to 2.0.3 upgrade applies as one transaction.
- **Improved dirty-migration recovery documentation.** The operator guide's procedure now uses direct `psql` access to inspect and correct `schema_migrations`, which works on both Docker and Proxmox LXC installs, replacing the previous instructions built around the bundled `migrate` binary.
- **Dependency updates.** Server: chi 5.3.1, pgx 5.10.0, go-oidc 3.20.0, go-redis 9.21.0, `golang.org/x/crypto` 0.54.0. Agent: gopsutil 4.26.6, moby 1.55.0. Frontend: Vite 7, Vitest 4.

## Upgrade

No special steps. From your `docker-compose.yml` directory:

```bash
docker compose pull
docker compose up -d
```

Migrations run on startup.

**Expect a pause on first boot if you run a large fleet.** This release adds several indexes, a materialised view and a trigram index. Each index build reads every row of the affected table, so on an installation with roughly 1,000 hosts (around 1.3 million package rows) the migration typically takes a few seconds to about 20 seconds on SSD or NVMe, and can run into minutes on slower storage such as a Proxmox LXC on spinning disk. PatchMon does not serve requests while migrations run, so allow for the gap rather than assuming the container has hung. If the process is interrupted part way through, the migration is marked dirty; see the dirty-migration recovery section of the operator guide.

Two things worth doing after the upgrade:

1. **Update your agents** to enable hash-gated check-in. Existing agents keep reporting normally without it, but will continue sending full inventories every cycle.
2. **Set an explicit TLS mode** on each existing email notification destination. They load as **Auto** and keep working, but an explicit mode fails closed on a misconfigured relay.
