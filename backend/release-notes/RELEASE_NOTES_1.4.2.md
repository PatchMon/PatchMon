## 🎉 PatchMon 1.4.2

## 📈 Dashboard and UI

- **Editable dashboard**: Dashboard widgets can be edited and re-arranged; a default layout is provided and editing is the default experience.
- **Bull Board missing over HTTP**: The queue monitoring UI (Bull Board) did not appear when the app was served over HTTP (e.g. dev or internal HTTP). It now shows correctly for both HTTP and HTTPS.
- **Ultrawide (21:9) layouts**: Dashboard layout is adjusted for 21:9 and similar ultrawide screens so content uses space better.

---

## 📊 Compliance

- **“Transaction already closed” errors**: Compliance operations could fail with “Transaction already closed: A query cannot be executed on an expired transaction”. The underlying transaction/upsert handling is fixed so these errors no longer occur under normal use.
- **Stuck compliance scans**: Scans that ran for 3+ hours could leave jobs in a “running” state. Automatic cleanup now stops and cleans up these long-running scans.
- **Cancel running scans**: You can cancel a compliance scan that is in progress instead of waiting for it to finish or timeout.
- **Compliance dashboard and tables**: Compliance dashboard rework: new dashboard card, clearer tables for scan results, and scanner status stored per agent/host. Table display and behaviour are improved.
- **Debian compliance scans**: Fixes for Debian-related compliance scans so they run and report correctly.
- **Per-host scanner toggles**: OpenSCAP and Docker Bench can be enabled/disabled per host. OpenSCAP defaults to on when compliance is on; Docker Bench defaults to off. Existing data is preserved via migration.
- **Log safety in compliance routes**: Host IDs are sanitised before being written to logs so user-controlled input cannot inject fake log lines (e.g. via newlines).

---

## 🔐 HTTPS and reverse proxy

- **WebSocket shown as insecure (ws) when using HTTPS**: When PatchMon was behind a reverse proxy (e.g. nginx, Traefik) with HTTPS, the UI could still show the agent connection as insecure (`ws` instead of `wss`). This is fixed by correctly using `X-Forwarded-Proto` (including `https` and `wss`) and the header name used by some proxies (`http_x_forwarded_proto`), so the secure state matches how users actually connect.

---

## 🔑 OIDC and authentication

- **OIDC login/logout loop**: With “auto redirect to OIDC” enabled, some users hit a redirect loop between login and logout. That flow is fixed so OIDC-only setups work as intended.
- **Auto-redirect to OIDC**: When `OIDC_ENABLED=true` and `OIDC_DISABLE_LOCAL_AUTH=true`, the app now automatically redirects to the OIDC provider instead of showing the local login page.

---

## ⚙️ Settings and URL config

- **Settings and URL not saving**: Server URL and related settings (protocol, host, port) could fail to save or be lost after restart. The backend now uses the database as the source of truth for the server URL after initial creation, so URL and environment-related settings persist correctly and are retrieved when loading the settings page.

---

## 🖥️ Agent and hosts

- **Agent download from GitHub**: Fixes for downloading agents from GitHub so installs/updates complete reliably.
- **NanoPi / no disks**: On devices like NanoPi with no disks (or when no disks are detected), the UI could show “null” or errors. Disk handling and display are fixed for “no disks” cases, and related lint issues are addressed.
- **Docker entrypoint agent update**: The non-fatal warning during agent update in the Docker entrypoint was removed to reduce noisy logs.
- **Agent log sanitisation**: OpenSCAP agent logs sanitise output so user-controlled or command output cannot inject newlines into log streams.

---

## 🔗 Integrations

- **Checkmk**: You can export hosts from the Integrations page for use with Checkmk.
- **Discord OAuth2**: Discord login and account linking are supported. The Discord OAuth callback was also updated for CodeQL and security (e.g. no raw OAuth parameters in logs, proper validation).

---

## 🔒 Security and dependencies

- **NPM vulnerabilities**: Dependency bumps and fixes to address known NPM vulnerabilities.
- **License**: License is clearly stated as AGPL v3 in the repo.
- **Code quality and secrets**: Code scanning and CodeQL are enabled.

---

## 📦 Other

- **Fonts**: Fonts are self-hosted where applicable for faster load and fewer external requests via DNS.
- **Biome**: Linting/tooling uses a pinned Biome version for consistent formatting and checks.

## Thank you

I appreciate the whole community for helping with PRs and 