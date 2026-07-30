---
title: "PatchMon Release Notes"
description: "Version-by-version PatchMon release notes — features, fixes, breaking changes, and migration pointers."
---

# PatchMon Release Notes

Each section below documents what changed in a PatchMon release. Versions are listed newest first. The same source files are also served by the application in the admin UI under Release Notes.

## Table of Contents

- [Version 2.0.3](#v2-0-3)
- [Version 2.0.2](#v2-0-2)
- [Version 2.0.1](#v2-0-1)
- [Version 2.0.0](#v2-0-0)
- [Version 1.4.2](#v1-4-2)
- [Version 1.4.1](#v1-4-1)
- [Version 1.4.0](#v1-4-0)
- [Version 1.3.7](#v1-3-7)

---

## Version 2.0.3 {#v2-0-3}


A performance, reliability and security release, load tested against fleets of 1,000+ hosts.

The headline change is hash-gated agent check-in, which cuts steady-state agent traffic by roughly three orders of magnitude. Alongside it: a rebuilt Agent Activity view, server-side pagination on the big list pages, explicit SMTP TLS modes, and a fix for client IP spoofing behind a reverse proxy.

### Highlights

#### Hash-gated agent check-in

Agents used to send their full inventory on every cycle, whether or not anything had changed. From 2.0.3 the agent computes a SHA-256 hash of each content section (packages, repositories, network interfaces, hostname, Docker, compliance) and sends only those hashes on check-in. The server compares them against the hashes it already holds and asks for full content only for the sections that actually changed.

A steady-state hourly check-in drops from roughly a 2 MB POST to a roughly 1 KB ping plus a roughly 200 B response. Across a large fleet that is a significant reduction in both network traffic and database write volume.

Older agents keep working without changes: they fall through to the legacy heartbeat path and continue sending full reports. Update your agents to pick up the benefit.

#### Agent Activity tab

The separate **Package Reports** and **Agent Queue** tabs on the host detail page are replaced by a single **Agent Activity** tab: one timeline of every agent communication cycle.

- Rows are typed as **Ping**, **Full**, **Partial**, **Docker**, **Compliance**, or **Job**.
- Each report row shows section chips: green "Updated" for sections the agent sent fresh data for, muted "Skipped" for sections the server already had a matching hash for.
- The Waiting / Active / Delayed / Failed queue stat cards sit above the table and reflect in-flight server-to-agent jobs.
- Bookmarks against the old `?tab=history` and `?tab=queue` query parameters redirect to `?tab=activity`.

Retention is controlled by the new `AGENT_REPORTS_RETENTION_DAYS` variable (default 30 days, range 7 to 365). A daily sweep at 02:00 removes anything older.

#### Host status pills

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

### Performance

Load testing at 1,000+ hosts with active connections drove the following:

- **Packages list page**: a new `mv_package_stats` materialised view serves the per-package install, update and security counters. This replaces an aggregation across roughly 1.3 million `host_packages` rows on every request, taking the page from around 10 seconds to sub-millisecond for that join. The view refreshes every 2 minutes via the job queue, using `REFRESH ... CONCURRENTLY` so readers are not blocked.
- **Package search no longer 500s in Docker**: searching packages fell back to a parallel sequential scan, which hit the default 64 MB `/dev/shm` ceiling in Docker and failed with `could not resize shared memory segment`. A trigram index on `packages.description` completes the index coverage for the search predicate.
- **Four new partial and covering indexes** on `host_packages` for the dashboard, hosts list and packages list query shapes.
- **`work_mem` is raised to 32 MB** for the transactions running large aggregations. The default 4 MB forced an external on-disk sort at fleet scale. If your connection pooler refuses the setting, the query still runs, just without the benefit.
- **Server-side pagination** on the Hosts, Packages and Repositories pages, with a selectable page size that persists between visits. Hosts offers 25 / 50 / 100 / 200 / 500 (default 50); Packages and Repositories offer 25 / 50 / 100 / 200.
- **New dashboard endpoints** for navigation counts and host filter options, so the sidebar and filter controls no longer piggyback on the full host list payload.

### Patch run reliability

- **Stuck runs are now cleaned up.** A run left in `running` past a configurable stall timeout is marked `timed_out`. The timeout is `PATCH_RUN_STALL_TIMEOUT_MIN` (default 30 minutes, minimum 5). The cleanup sweep now runs every 10 minutes rather than once daily at 00:30.
- **New `agent_disconnected` status.** If an agent's WebSocket drops while a run is in flight, the run is marked `agent_disconnected` instead of sitting at `running` indefinitely. If the agent reconnects and posts a late result for the same run, the row is updated to that final state.
- **Stop Run is now authoritative.** Cancelling applies the change in the database first, then sends a courtesy stop message to the agent if it is connected. Previously an offline or unresponsive agent could leave the row stuck in `running`.

### Email notifications

SMTP destinations now have an explicit **TLS mode** instead of a single "Use TLS" toggle:

- **STARTTLS (recommended)**: connect in plaintext, then require the server to advertise `STARTTLS` and upgrade before anything sensitive is sent. Refuses to send if the server does not advertise it. Typical port 587.
- **Implicit TLS / SSL**: TLS from the first byte. Typical port 465.
- **None (insecure)**: cleartext. PatchMon refuses to send if a username or password is set, rather than leaking credentials onto the wire.
- **Auto**: the previous opportunistic behaviour, kept for backward compatibility. It tries STARTTLS first and falls back to implicit TLS on the same host and port when STARTTLS is not advertised. It never falls back to cleartext.

Existing destinations load as **Auto** and keep working unchanged. Auto always encrypts, so nothing is sent in the clear either way, but it accepts whichever mode the server offers rather than enforcing the one you intended. Open each destination, select the mode your relay actually supports, and save, so a misconfigured or tampered-with server fails closed. Connections are pinned to a TLS 1.2 minimum and certificates are validated against the configured hostname.

Saved email destinations also gain a **Send test email** button. Unlike **Test**, which enqueues a synthetic notification through the worker, this runs a live SMTP probe from the API request and reports which stage failed: `validate`, `dial`, `starttls`, `auth`, or `send`, including the relay's own error message. This is the fastest way to diagnose a TLS or auth problem without reading server logs.

### Security

- **Client IP spoofing behind a reverse proxy is fixed.** PatchMon used chi's `RealIP` middleware, which takes the leftmost entry of `X-Forwarded-For`. That entry is whatever the client sent, so a caller could rotate their own rate-limit bucket and write an arbitrary IP into the audit log and login lockout records. Client IP is now resolved by a trusted-proxy-aware implementation, and the rate limiter reads the resolved value instead of re-parsing the header itself.
- **New `TRUSTED_PROXY_RANGES` variable.** Comma-separated CIDRs or bare IPs of the proxies in front of PatchMon. Leave it empty for a single reverse proxy, which is the usual setup and the correct value for the standard Docker deployment: PatchMon then uses the address your proxy appended, which a client cannot forge. Set it only when proxies are chained, for example Cloudflare in front of Nginx Proxy Manager. It is configurable by environment only and shown read-only in the settings UI, deliberately, since widening it to `0.0.0.0/0` would restore the spoofing problem.
- **New `AGENT_PING_BODY_LIMIT`** (default 8 KiB) caps bodies on the check-in endpoint so the cheap ping path cannot be abused as a payload sink.

### Fixes and maintenance

- **Frontend Docker builds are reproducible again.** The image built with `npm install --force` against a deleted lockfile, so it was free to resolve any version matching the semver ranges. That silently broke the build when `react-icons` 5.7.0 dropped an icon the app used. The image now builds from the committed lockfile with `npm ci`.
- **Consolidated migrations.** The seven in-development migrations for this release are squashed into a single migration, so the 2.0.2 to 2.0.3 upgrade applies as one transaction.
- **Improved dirty-migration recovery documentation.** The operator guide's procedure now uses direct `psql` access to inspect and correct `schema_migrations`, which works on both Docker and Proxmox LXC installs, replacing the previous instructions built around the bundled `migrate` binary.
- **Dependency updates.** Server: chi 5.3.1, pgx 5.10.0, go-oidc 3.20.0, go-redis 9.21.0, `golang.org/x/crypto` 0.54.0. Agent: gopsutil 4.26.6, moby 1.55.0. Frontend: Vite 7, Vitest 4.

### Upgrade

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

---

## Version 2.0.2 {#v2-0-2}


A focused fix release on top of 2.0.1.

### Fixes

- **Database deadlocks under concurrent agent reports**: fixed Postgres deadlocks that dropped agent reports on installations with many hosts. Reports are now ingested in deterministic order with bulk SQL, plus automatic retry. Per-host SQL round-trips collapsed from 2N+1 to 2.
- **Patching schedule timezone**: configured timezone is now honoured when scheduling patching jobs.
- **Agent reports failing with "Invalid request body"**: default `AGENT_UPDATE_BODY_LIMIT` raised from `2mb` to `5mb` so hosts with many packages no longer fail to update.
- **OIDC and other features broken behind a reverse proxy**: `TRUST_PROXY` now defaults to `true`. Most users run PatchMon behind a reverse proxy (Traefik, Caddy, nginx, NPM); the previous `false` default caused OIDC logins to fail and real client IPs to be lost. If you run PatchMon directly on a public IP without a reverse proxy, set `TRUST_PROXY=false` explicitly.
- **Docker healthcheck failing on non-default ports**: the container healthcheck now honours the `PORT` environment variable instead of hardcoding `3000`.

### Startup health check

If you run more than 50 active hosts, PatchMon now logs a warning at startup if `DB_CONNECTION_LIMIT` (default `30`) looks too small, with a recommended value calculated from your host count.

### Upgrade

No special steps. From your `docker-compose.yml` directory:

```bash
docker compose pull
docker compose up -d
```

Migrations run on startup.

---

## Version 2.0.1 {#v2-0-1}


A small follow-up release on top of 2.0.0 covering documentation, packaging, and a couple of important fixes.

### Documentation

- Documentation now lives at [https://patchmon.net/docs](https://patchmon.net/docs). The source of truth is this GitHub repository: the public site builds from `PatchMon/docs/*.md` at deploy time, so corrections and contributions flow through the normal pull-request process.

### Fixes

- **Docker image format on older clients**: fixed an image format issue that prevented older Docker and Podman versions from pulling `ghcr.io/patchmon/patchmon-server:2.0.0`. Image layers are now published with gzip compression instead of zstd, so clients without zstd support (Podman versions before 5.7, and Docker installations without the containerd image store) can pull cleanly. See [issue #679](https://github.com/PatchMon/PatchMon/issues/679).
- **Database migration failure at migration 30**: fixed an upgrade path that could fail at migration `000030` on some installations. The migration is now safe to re-run on a partially-upgraded database, so retries succeed without manual intervention.
- **SMTP / TLS**: the **Use TLS** option for email destinations is now respected end-to-end. Notification and scheduled-report delivery no longer upgrade the connection with **STARTTLS** when the server advertises it if you have turned TLS off in the UI.

### New features

- **Email opt-in for security and instance notifications** is now available.

### Upgrade

No special steps. Pull the new image and restart your stack:

```bash
docker compose pull patchmon-server
docker compose up -d patchmon-server
```

Migrations run on startup as usual.

---

## Version 2.0.0 {#v2-0-0}


### Architectural changes

#### Go

- Backend rewritten from the ground up in Go, replacing Node.js and Next.js. The stack is more scalable, uses less RAM, and is significantly more performant.
- [sqlc](https://sqlc.dev/) is used for type-safe SQL against PostgreSQL (compile-time checked queries instead of ad-hoc ORM access patterns).
- [golang-migrate](https://github.com/golang-migrate/migrate) is used for database migrations (replacing Prisma ORM).
- Structured logging with the standard library `log/slog` for cleaner, machine-parseable logs in production.

#### Background jobs and automation

- Background work is handled by **[asynq](https://github.com/hibiken/asynq)** on Redis instead of BullMQ. PatchMon no longer ships the embedded **Bull Board** stack; queue visibility and triggers live in the existing **Automation** UI, which reduces attack surface, image size, and operational complexity.

#### Docker

- Docker is the officially supported deployment method going forward; bare-metal installs are discontinued. A migration document describes the upgrade path.
- Hardened base images are used. They ship with near-zero CVEs and a smaller footprint.
- No separate frontend container: static React build artifacts are embedded in the Go binary. The container runs that binary (by default on port 3000) with [chi](https://github.com/go-chi/chi): `/api/*` is handled by the server, so nginx inside PatchMon is no longer required. You still use nginx or another reverse proxy in front for TLS termination and public access as usual.
- A Guacamole (guacd) sidecar is included for Windows RDP. It is separate for now; RDP/VNC for Windows is an area we intend to improve.

#### API documentation

- **OpenAPI 3** spec is served at `/api/v1/openapi.json`, with **Swagger UI** under `/api/v1/api-docs` (authenticated) for exploring integration endpoints.

### New features

- **Linux patching**: Deploy updates per host or in bulk, on demand or on a schedule. **Policies** support host/group assignments and exclusions; runs support **approval**, **stop**, **retry validation**, and **live log streaming** over WebSocket.
- **Microsoft Windows agent** (beta) and **FreeBSD** agent support.
- **Windows Updates** (beta Windows agent): server APIs for update results, reboot state, superseded cleanup, and approved-guid sync, aligned with the new Windows agent.
- **Advanced monitoring & alerting**: richer alert lifecycle (including assignment and bulk actions), optional **advanced alert configuration** for tuning and cleanup where your edition includes it.
- **Notifications**: first-class **destinations** (SMTP, webhooks, ntfy), **routes**, **delivery log**, and **scheduled reports** so operational signals leave PatchMon reliably.
- **Environment variables in the GUI**: many settings that were previously only in process environment can be **viewed and edited from the Settings UI** (per-key updates, with sensible validation), so you change less by hand in compose or shell env for day-to-day tuning.
- **OIDC / SSO**: configure OpenID Connect from the same Settings area, including **import from environment** when you are migrating from a file-based or container env setup.

### Other improvements

- **Compliance / OpenSCAP**: SSG and CIS benchmarking content is **bundled in the server binary** at build time. Agents no longer pull scanning content from GitHub; everyone shares one versioned source of truth and less outbound traffic from agents.
- **SSO**: improved sign-in flows and **Entra ID** integration compared to 1.4.x OIDC edge cases (e.g. redirect loops with auto SSO).
- **Dashboard**: additional cards and data surfaces; dashboard layout preferences carry forward in the new UI.
- **Host integration config**: **apply pending config** from the server so integration changes are applied to agents in a controlled, observable way.
- **Settings reliability**: server URL and related configuration are reimplemented on the Go stack with database-backed resolution, addressing classes of “settings did not persist” issues from the Node era.
- **Reverse proxy awareness**: continued correct use of forwarded headers for HTTPS/WSS behind proxies (without the Bull Board-specific HTTP quirks from 1.4.x).
- **Optional admin pprof**: when enabled, CPU/memory profiling endpoints are available to administrators for performance investigation.

### Packaging and editions

- Features are grouped into **capability modules** (e.g. patching policies, advanced alerts, custom branding, Docker inventory, compliance depth, AI assist, remote access). Core workflows stay simple; larger deployments can enable more surface area where their **subscription or license** allows. See in-app **Context** / billing documentation for your tenant.

### Known issues

- **Remote Desktop (RDP)**: there is a known bug with the RDP connection flow in this release. A fix is planned for the next release.

### Migrations

This covers migration for Docker, Proxmox community scripts, and legacy `setup.sh` installs:

[Migrating from 1.4.2 to 2.0.0](https://patchmon.net/docs/patchmon-release-notes#v2-0-0)

---

## Version 1.4.2 {#v1-4-2}

### 🎉 PatchMon 1.4.2

### 📈 Dashboard and UI

- **Editable dashboard**: Dashboard widgets can be edited and re-arranged; a default layout is provided and editing is the default experience.
- **Bull Board missing over HTTP**: The queue monitoring UI (Bull Board) did not appear when the app was served over HTTP (e.g. dev or internal HTTP). It now shows correctly for both HTTP and HTTPS.
- **Ultrawide (21:9) layouts**: Dashboard layout is adjusted for 21:9 and similar ultrawide screens so content uses space better.

---

### 📊 Compliance

- **“Transaction already closed” errors**: Compliance operations could fail with “Transaction already closed: A query cannot be executed on an expired transaction”. The underlying transaction/upsert handling is fixed so these errors no longer occur under normal use.
- **Stuck compliance scans**: Scans that ran for 3+ hours could leave jobs in a “running” state. Automatic cleanup now stops and cleans up these long-running scans.
- **Cancel running scans**: You can cancel a compliance scan that is in progress instead of waiting for it to finish or timeout.
- **Compliance dashboard and tables**: Compliance dashboard rework: new dashboard card, clearer tables for scan results, and scanner status stored per agent/host. Table display and behaviour are improved.
- **Debian compliance scans**: Fixes for Debian-related compliance scans so they run and report correctly.
- **Per-host scanner toggles**: OpenSCAP and Docker Bench can be enabled/disabled per host. OpenSCAP defaults to on when compliance is on; Docker Bench defaults to off. Existing data is preserved via migration.
- **Log safety in compliance routes**: Host IDs are sanitised before being written to logs so user-controlled input cannot inject fake log lines (e.g. via newlines).

---

### 🔐 HTTPS and reverse proxy

- **WebSocket shown as insecure (ws) when using HTTPS**: When PatchMon was behind a reverse proxy (e.g. nginx, Traefik) with HTTPS, the UI could still show the agent connection as insecure (`ws` instead of `wss`). This is fixed by correctly using `X-Forwarded-Proto` (including `https` and `wss`) and the header name used by some proxies (`http_x_forwarded_proto`), so the secure state matches how users actually connect.

---

### 🔑 OIDC and authentication

- **OIDC login/logout loop**: With “auto redirect to OIDC” enabled, some users hit a redirect loop between login and logout. That flow is fixed so OIDC-only setups work as intended.
- **Auto-redirect to OIDC**: When `OIDC_ENABLED=true` and `OIDC_DISABLE_LOCAL_AUTH=true`, the app now automatically redirects to the OIDC provider instead of showing the local login page.

---

### ⚙️ Settings and URL config

- **Settings and URL not saving**: Server URL and related settings (protocol, host, port) could fail to save or be lost after restart. The backend now uses the database as the source of truth for the server URL after initial creation, so URL and environment-related settings persist correctly and are retrieved when loading the settings page.

---

### 🖥️ Agent and hosts

- **Agent download from GitHub**: Fixes for downloading agents from GitHub so installs/updates complete reliably.
- **NanoPi / no disks**: On devices like NanoPi with no disks (or when no disks are detected), the UI could show “null” or errors. Disk handling and display are fixed for “no disks” cases, and related lint issues are addressed.
- **Docker entrypoint agent update**: The non-fatal warning during agent update in the Docker entrypoint was removed to reduce noisy logs.
- **Agent log sanitisation**: OpenSCAP agent logs sanitise output so user-controlled or command output cannot inject newlines into log streams.

---

### 🔗 Integrations

- **Checkmk**: You can export hosts from the Integrations page for use with Checkmk.
- **Discord OAuth2**: Discord login and account linking are supported. The Discord OAuth callback was also updated for CodeQL and security (e.g. no raw OAuth parameters in logs, proper validation).

---

### 🔒 Security and dependencies

- **NPM vulnerabilities**: Dependency bumps and fixes to address known NPM vulnerabilities.
- **License**: License is clearly stated as AGPL v3 in the repo.
- **Code quality and secrets**: Code scanning and CodeQL are enabled.

---

### 📦 Other

- **Fonts**: Fonts are self-hosted where applicable for faster load and fewer external requests via DNS.
- **Biome**: Linting/tooling uses a pinned Biome version for consistent formatting and checks.

### Thank you

I appreciate the whole community for helping with PRs and help testing areas of PatchMon <3

---

## Version 1.4.1 {#v1-4-1}

### 🎉 PatchMon 1.4.1

A maintenance release with OIDC improvements, FreeBSD agent support, installer fixes, and various bug fixes and improvements.

#### 🔐 OIDC Improvements and Hot Fixes
- OIDC authentication fixes and stability improvements
- Hot fixes for edge cases in SSO flows

#### 🖥️ FreeBSD Agent Support
- **Native FreeBSD agent support** — run the PatchMon agent on FreeBSD hosts
- Initial FreeBSD support via community contribution

#### 📦 Native Installer Upgrade Fixes
- Fixes for native installer upgrade paths
- Improved reliability when upgrading existing installations

#### 🐛 Host Table Views Not Saving -> Bug Fix
- Fixed an issue where host table view preferences (columns, sort order, filters) were not being saved
- Table view state now persists correctly across sessions

#### 🔧 Agent Memory Leaks and Improvements
- Addressed memory leaks in the agent
- General agent stability and resource usage improvements

#### 🔒 Better API Integration Scoping
- Improved scoping for Integration API credentials and access
- Tighter integration between API keys and their permitted scope

---

#### 🙏 Acknowledgements

- **@RuTHlessBEat200** — for agent and OIDC fixes
- **@mminkus** — for FreeBSD initial PR
- The rest of the community for their support and help on Discord and GitHub

---

---

## Version 1.4.0 {#v1-4-0}

### 🎉 PatchMon 1.4.0

A major release with security compliance scanning, OIDC SSO, an alerting engine, web SSH terminal, and AI-assisted terminal support.

#### 🛡️ Security Compliance Scanning
- **OpenSCAP CIS Benchmark scanning** directly from the agent (Level 1 / Level 2)
- **Docker Bench for Security** when Docker integration is enabled
- **Compliance dashboard** with fleet-wide scores, pass/fail breakdowns, and scan history
- **Optional auto-remediation** of failed rules during scans

#### 🔐 OIDC Single Sign-On
- **OpenID Connect authentication** with Authentik, Keycloak, Okta, or any OIDC provider
- **Automatic user provisioning** on first OIDC login
- **Group-based role mapping** from your identity provider to PatchMon roles
- **Option to disable local auth** and enforce SSO-only login

#### 🔔 Alerting & Reporting
- **New Reporting page** with filtering by severity, type, status, and assignment
- **Host Down alerts** real time view of host uptime
- **Alert types** including server update, agent update, and host down
- **Per-alert-type configuration** for default severity, auto-assignment, escalation, and retention

#### 💻 Web SSH Terminal
- **Browser-based SSH** to any host from the PatchMon UI
- **Direct and proxy modes** (proxy mode routes through the agent, no SSH port exposure needed)

#### 🤖 AI Terminal Assistant
- **AI chat panel** inside the SSH terminal for command suggestions and troubleshooting
- **Multiple providers** supported: OpenRouter, Anthropic, OpenAI, Google Gemini
- **Context-aware** using your recent terminal output

#### 🖥️ UI Improvements
- **Toast notifications** replacing disruptive `alert()` popups
- **Error boundary** with crash recovery and a copyable error report
- **"Waiting for Connection" screen** with real-time status when onboarding a new host
- **Swagger / OpenAPI docs** served at `/api-docs` on the server


#### 🔧 Other
- **Superuser management permission** (`can_manage_superusers`) for finer-grained RBAC
- **More Scoped API stats** and details on hosts with added flags such as ```?include=stats``` or ```?updates_only=true```


##### Plus Much Much More
---

---

## Version 1.3.7 {#v1-3-7}

### 📝 ALERT : Auto-update of Agent issue

Versions <1.3.6 have an issue where the service does not restart after auto-update. OpenRC systems are unaffected and work correctly.
This means you will unfortunately have to use `systemctl start patchmon-agent` on your systems to load up 1.3.7 agent when it auto-updates shortly.

Very sorry for this, future versions are fixed - I built this release notes notification feature specifically to notify you of this.

---

### 🎉 New Features & Improvements :

**Mobile UI**: Mobile user interface improvements are mostly complete, providing a better experience on mobile devices.

**Systemctl Helper Script**: In future versions (1.3.7+), a systemctl helper script will be available to assist with auto-update service restarts.

**Staggered Agent Intervals**: Agents now report at staggered times to prevent overwhelming the PatchMon server. If the agent report interval is set to 60 minutes, different hosts will report at different times. This is in the `config.yml` as "report_offset: nxyz"

**Reboot Detection Information**: Reboot detection information is now stored in the database. When the "Reboot Required" flag is displayed, hovering over it will show the specific reason why a reboot is needed (Reboot feature still needs work and it will be much better in 1.3.8)

**JSON Report Output**: The `patchmon-agent report --json` command now outputs the complete report payload to the console in JSON format instead of sending it to the PatchMon server. This is very useful for integrating PatchMon agent data with other tools and for diagnostic purposes.

**Persistent Docker Toggle**: Docker integration toggle state is now persisted in the database, eliminating in-memory configuration issues. No more losing Docker settings on container restarts (thanks to the community for initiating this feature).

**Config.yml Synchronization**: The agent now writes and compares the `config.yml` file with the server configuration upon startup, ensuring better synchronization of settings between the agent and server.

**Network Information Page**: Enhanced network information page to display IPv6 addresses and support multiple network interfaces, providing more comprehensive network details.

**Auto-Update Logic Fix**: Fixed an issue where agents would auto-update even when per-host auto-update was disabled. The logic now properly honors both server-wide auto-update settings and per-host auto-update settings.

**Prisma Version Fix**: Fixed Prisma version issues affecting Kubernetes deployments by statically setting the Prisma version in package.json files.

**Hiding Github Version**: Added a toggle in Server Version settings to disable showing the github release notes on the login screen

---
