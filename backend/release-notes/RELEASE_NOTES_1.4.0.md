## 🎉 PatchMon 1.4.0

A major release with security compliance scanning, OIDC SSO, an alerting engine, web SSH terminal, and AI-assisted terminal support.

### 🛡️ Security Compliance Scanning
- **OpenSCAP CIS Benchmark scanning** directly from the agent (Level 1 / Level 2)
- **Docker Bench for Security** when Docker integration is enabled
- **Compliance dashboard** with fleet-wide scores, pass/fail breakdowns, and scan history
- **Optional auto-remediation** of failed rules during scans

### 🔐 OIDC Single Sign-On
- **OpenID Connect authentication** with Authentik, Keycloak, Okta, or any OIDC provider
- **Automatic user provisioning** on first OIDC login
- **Group-based role mapping** from your identity provider to PatchMon roles
- **Option to disable local auth** and enforce SSO-only login

### 🔔 Alerting & Reporting
- **New Reporting page** with filtering by severity, type, status, and assignment
- **Host Down alerts** real time view of host uptime
- **Alert types** including server update, agent update, and host down
- **Per-alert-type configuration** for default severity, auto-assignment, escalation, and retention

### 💻 Web SSH Terminal
- **Browser-based SSH** to any host from the PatchMon UI
- **Direct and proxy modes** (proxy mode routes through the agent, no SSH port exposure needed)

### 🤖 AI Terminal Assistant
- **AI chat panel** inside the SSH terminal for command suggestions and troubleshooting
- **Multiple providers** supported: OpenRouter, Anthropic, OpenAI, Google Gemini
- **Context-aware** using your recent terminal output

### 🖥️ UI Improvements
- **Toast notifications** replacing disruptive `alert()` popups
- **Error boundary** with crash recovery and a copyable error report
- **"Waiting for Connection" screen** with real-time status when onboarding a new host
- **Swagger / OpenAPI docs** served at `/api-docs` on the server


### 🔧 Other
- **Superuser management permission** (`can_manage_superusers`) for finer-grained RBAC
- **More Scoped API stats** and details on hosts with added flags such as ```?include=stats``` or ```?updates_only=true```


#### Plus Much Much More
---