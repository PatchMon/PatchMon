# PatchMon-Enhanced v1.7.77 - Comprehensive PR Documentation

**PR Title:** `feat: PatchMon-Enhanced v1.7.77 - OIDC, Compliance, SSH Terminal, and Security Improvements`

**Base:** `PatchMon/PatchMon` v1.3.7
**Head:** `MacJediWizard/PatchMon-Enhanced` v1.7.77
**Agent:** `MacJediWizard/PatchMonEnhanced-agent` v1.5.55

---

## Summary

This PR introduces major enhancements to PatchMon including:

1. **OIDC/SSO Authentication** - Full OpenID Connect support with group-based role mapping
2. **Compliance Scanning** - CIS benchmark scanning via OpenSCAP and Docker Bench Security
3. **SSH Terminal Access** - Browser-based SSH terminal for managed hosts
4. **AI Terminal Assistant** - Optional AI-powered command assistance
5. **Security Hardening** - Multiple security vulnerability fixes
6. **Performance Improvements** - Database indexing and query optimization

---

## Related GitHub Issues

### Upstream Issues (PatchMon/PatchMon) - Resolved by This PR

These are feature requests from the upstream PatchMon repository that this PR implements:

| Issue | Title | Status |
|-------|-------|--------|
| [PatchMon/PatchMon#92](https://github.com/PatchMon/PatchMon/issues/92) | Feature Request: Modern Authentication (OIDC, SAML) | **PARTIALLY RESOLVED** (OIDC implemented, SAML not yet) |
| [PatchMon/PatchMon#159](https://github.com/PatchMon/PatchMon/issues/159) | [Feature Request] SSO Support (Authentic / Authelia) | **RESOLVED** |
| [PatchMon/PatchMon#57](https://github.com/PatchMon/PatchMon/issues/57) | Feature request, LDAP authentication | **PARTIALLY RESOLVED** (OIDC implemented, LDAP not yet) |
| [PatchMon/PatchMon#296](https://github.com/PatchMon/PatchMon/issues/296) | [FEATURE] Implement trusted header auth | **RESOLVED** (via OIDC proxy support) |
| [PatchMon/PatchMon#106](https://github.com/PatchMon/PatchMon/issues/106) | Improvement: Validate BetterAuth | **ALTERNATIVE** (OIDC provides similar functionality) |

### PatchMon-Enhanced Open Issues (May Be Resolved)
- **#174** - feat: Compliance Dashboard Improvements
- **#175** - feat: Bulk Compliance Scan Trigger

### PatchMon-Enhanced Closed Issues (Resolved by This PR)

#### OIDC/SSO Authentication
| Issue | Severity | Title |
|-------|----------|-------|
| #1 | CRITICAL | OIDC tokens exposed in URL redirect |
| #2 | CRITICAL | OIDC session store uses in-memory Map |
| #3 | CRITICAL | Missing OIDC nonce validation |
| #7 | HIGH | OIDC email linking without verification check |
| #8 | HIGH | No OIDC subject conflict check before linking |
| #14 | MEDIUM | OIDC cookie sameSite too permissive |
| #15 | MEDIUM | OIDC error messages may leak information |
| #16 | MEDIUM | Missing HTTPS enforcement for OIDC |
| #41 | MEDIUM | OIDC auth sets null token causing inconsistent state |
| #25 | LOW | Missing OIDC logout support |
| #26 | LOW | Missing OIDC token refresh handling |
| #27 | LOW | Missing audit logging for OIDC events |
| #28 | LOW | OIDC username generation may contain special characters |
| #47 | LOW | error_description from OIDC callback is unused |
| #51 | LOW | OIDC callback missing code parameter validation |
| #73 | BUG | SSH Terminal fails to authenticate for OIDC users |
| #75 | BUG | Session validation fails when access_token not provided |
| #119 | Enhancement | Add OIDC avatar/profile picture support |
| #122 | BUG | Fix OIDC profile sync and default scopes |
| #127 | BUG | OIDC users unable to access Queue Monitor |
| #135 | Enhancement | Add superadmin role with OIDC group-based role assignment |
| #136 | Enhancement | Bypass welcome page when OIDC is configured with auto-create |

#### SSH Terminal
| Issue | Severity | Title |
|-------|----------|-------|
| #91 | CRITICAL | Missing Host-Level Authorization in SSH Terminal |
| #73 | BUG | SSH Terminal fails to authenticate for OIDC users |
| #74 | BUG | SSH Terminal WebSocket validation crashes with ERR_INVALID_ARG_TYPE |
| #95 | HIGH | Sensitive Tokens Passed in URL Parameters |
| #160 | Enhancement | Add AI Terminal Assistant with Multi-Provider Support |
| #161 | Enhancement | Improve SSH Terminal AI Assistant UX |

#### Compliance Scanning
| Issue | Severity | Title |
|-------|----------|-------|
| #84 | MEDIUM | Missing input validation on compliance API endpoints |
| #85 | MEDIUM | ComplianceTab triggerScan mutation missing error handling |
| #86 | MEDIUM | compliance_scans schema missing error_message field |
| #87 | LOW | compliance_scans missing index on completed_at |
| #88 | LOW | compliance_scans missing updated_at field |
| #90 | LOW | No rate limiting on compliance scan submission endpoint |
| #121 | Enhancement | Move Docker and Compliance to Integrations nav section |
| #123 | Enhancement | Add Compliance tab to Settings > Integrations page |
| #124 | Enhancement | Add Compliance toggle and Docker tab to host detail page |
| #128 | BUG | Compliance toggle returns 'Invalid integration name' |
| #129 | BUG | Compliance dashboard fails to load - missing database tables |
| #133 | Enhancement | Add compliance setup status display in UI |
| #139 | Enhancement | Add Compliance Integration to Add Host modal |
| #141 | BUG | Add compliance_enabled support to host creation endpoint |
| #143 | Enhancement | Add dynamic scanner info display to Compliance Settings |
| #146 | BUG | Invalid profile_type error when running compliance scans |
| #150 | Enhancement | Compliance Results tab improvements |
| #153 | Enhancement | Improved Compliance Dashboard Metrics |
| #172 | Enhancement | Implement Compliance page improvements |

#### Security Fixes
| Issue | Severity | Title |
|-------|----------|-------|
| #4 | CRITICAL | Bull Board cookie security flags incorrect |
| #33 | CRITICAL | Plaintext API key comparison in settingsRoutes.js |
| #54 | CRITICAL | Unprotected agent removal script endpoint |
| #76 | CRITICAL | Agent WebSocket connections fail - API key comparison doesn't support bcrypt |
| #55 | HIGH | TFA disable endpoint skips password verification |
| #56 | HIGH | API key logged in plaintext at debug level |
| #57 | HIGH | Public /check-admin-users endpoint exposes system info |
| #40 | MEDIUM | JWT token stored in localStorage (XSS risk) |
| #42 | MEDIUM | Missing authentication on /auto-update endpoint |
| #62 | MEDIUM | Bull Board route has permissive CSP with unsafe-eval |
| #105 | MEDIUM | Rate Limiting Only Counts Failed Requests |
| #178 | MEDIUM | Hardcoded localhost fallbacks break production deployments |
| #117 | LOW | Backend security hardening - minor improvements |

#### Code Quality & Other
| Issue | Severity | Title |
|-------|----------|-------|
| #29 | LOW | Hardcoded timeout values should be configurable |
| #48 | LOW | Session cleanup function never called |
| #68 | LOW | Console.log statements should be removed or conditional |
| #118 | LOW | Frontend minor improvements |
| #142 | BUG | TDZ error on Host Detail page - components defined after use |
| #158 | MEDIUM | Remove or guard console.log statements in production |
| #163 | Enhancement | Improve role permissions hierarchy - differentiate user from readonly |

### Agent Issues (PatchMonEnhanced-agent)

#### Open Issues
| Issue | Severity | Title |
|-------|----------|-------|
| #19 | CRITICAL | Invalid Go version 1.25 in go.mod |
| #20 | MEDIUM | TLS verification bypass option should be removed or restricted |
| #21 | CODE-QUALITY | Docker image update checking is commented out |
| #22 | Enhancement | Docker Bench Security Integration Improvements |

#### Closed Issues (Resolved)
| Issue | Severity | Title |
|-------|----------|-------|
| #1 | CRITICAL | TLS Certificate Verification Bypass for Binary Downloads |
| #17 | CRITICAL | Shell injection via fmt.Sprintf in agent restart helpers |
| #2 | HIGH | Binary Updates Downloaded Without Signature Verification |
| #3 | HIGH | Shell Scripts Generated and Executed at Runtime |
| #4 | HIGH | Docker Bench Container Runs with Elevated Privileges |
| #9 | HIGH | Insecure TLS skip verify option |
| #18 | HIGH | TOCTOU race condition in helper script execution |
| #5 | MEDIUM | Goroutine Leak in WebSocket Connection |
| #6 | MEDIUM | Error Messages May Leak Sensitive Information |
| #7 | MEDIUM | Hardcoded Configuration Values Should Be Configurable |
| #10 | MEDIUM | Resource limits missing for downloads and WebSocket |
| #11 | MEDIUM | TOCTOU race condition in helper script execution |
| #12 | MEDIUM | Make binary hash verification mandatory |
| #23 | MEDIUM | WebSocket message inputs not validated - command injection risk |
| #24 | MEDIUM | TOCTOU race condition in credentials file creation |
| #8 | LOW | Directory and File Permissions Should Be More Restrictive |
| #13 | LOW | Go agent stale code and minor improvements |
| #14 | Enhancement | Add comprehensive scanner info to compliance status reports |
| #15 | Enhancement | Add dynamic scanner info display to Compliance Settings |
| #16 | Enhancement | Docker Image CVE Scanning with oscap-docker |

---

## New Features

### 1. OIDC/SSO Authentication

Full OpenID Connect authentication support allowing integration with enterprise identity providers (Okta, Auth0, Keycloak, Azure AD, etc.).

**Capabilities:**
- OIDC login flow with authorization code grant
- Group-to-role mapping for automatic role assignment
- Avatar/profile picture support from OIDC claims
- Auto-create users on first OIDC login
- Optional disable of local authentication
- Session management with configurable TTL
- Superadmin role with group-based assignment

### 2. Compliance Scanning (OpenSCAP + Docker Bench)

Comprehensive security compliance scanning with CIS benchmark support.

**OpenSCAP Features:**
- CIS benchmark scanning for Ubuntu, RHEL, Debian
- SCAP Security Guide (SSG) content support
- Profile discovery and selection
- Real-time scan progress via WebSocket
- Single rule remediation
- SSG version checking and upgrade capability
- Score tracking and trend analysis

**Docker Bench Security Features:**
- CIS Docker Benchmark scanning
- Container security assessment
- Multi-line remediation parsing
- Pass/Fail/Warn result categorization

### 3. SSH Terminal Access

Browser-based SSH terminal for direct host management.

**Features:**
- WebSocket-based terminal connection
- Username caching per host
- Idle timeout with warning
- AI command assistance integration
- Full terminal emulation (xterm.js)

### 4. AI Terminal Assistant

Optional AI-powered command assistance for the SSH terminal.

**Supported Providers:**
- OpenRouter
- Anthropic (Claude)
- OpenAI
- Google Gemini

### 5. Security Improvements

- WebSocket input validation to prevent command injection
- TOCTOU (Time-of-Check to Time-of-Use) race condition fixes
- Credentials file atomic writes
- Shell injection prevention in agent
- Session security enhancements

### 6. Compliance On-Demand Only Mode

Control whether compliance scans run automatically during scheduled reports.

**Behavior:**
- When enabled: Compliance scans only run when triggered from UI
- When disabled: Compliance scans run during scheduled agent reports
- Default: Enabled (on-demand only)

---

## Environment Variables

### New OIDC Variables

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `OIDC_ENABLED` | Boolean | `false` | Enable OIDC authentication |
| `OIDC_ISSUER_URL` | String | - | OIDC provider issuer URL |
| `OIDC_CLIENT_ID` | String | - | OIDC client ID |
| `OIDC_CLIENT_SECRET` | String | - | OIDC client secret |
| `OIDC_REDIRECT_URI` | String | - | OAuth callback URL |
| `OIDC_POST_LOGOUT_URI` | String | - | Post-logout redirect URL |
| `OIDC_SCOPES` | String | `openid profile email` | OIDC scopes to request |
| `OIDC_BUTTON_TEXT` | String | `Login with SSO` | Login button text |
| `OIDC_AUTO_CREATE_USERS` | Boolean | `false` | Auto-create users on first login |
| `OIDC_DEFAULT_ROLE` | String | `user` | Default role for auto-created users |
| `OIDC_SYNC_ROLES` | Boolean | `false` | Sync roles from OIDC groups |
| `OIDC_DISABLE_LOCAL_AUTH` | Boolean | `false` | Disable local authentication |
| `OIDC_SESSION_TTL` | Number | `86400` | Session TTL in seconds |

### OIDC Group Mapping Variables

| Variable | Type | Description |
|----------|------|-------------|
| `OIDC_SUPERADMIN_GROUP` | String | OIDC group for superadmin role |
| `OIDC_ADMIN_GROUP` | String | OIDC group for admin role |
| `OIDC_HOST_MANAGER_GROUP` | String | OIDC group for host_manager role |
| `OIDC_USER_GROUP` | String | OIDC group for user role |
| `OIDC_READONLY_GROUP` | String | OIDC group for readonly role |

### AI Assistant Variables

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `AI_ENCRYPTION_KEY` | String | - | Encryption key for AI API keys (32 bytes hex) |

### Other New Variables

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `DEFAULT_USER_ROLE` | String | `user` | Default role for new users |
| `AUTO_CREATE_ROLE_PERMISSIONS` | Boolean | `false` | Auto-create role permissions |
| `FRONTEND_URL` | String | - | Frontend URL for OIDC callbacks |
| `TRUST_PROXY` | Boolean | `false` | Trust proxy headers |
| `ENABLE_HSTS` | Boolean | `false` | Enable HSTS header |

### Example Environment Configuration (Docker Compose)

```yaml
environment:
  # Database
  DATABASE_URL: postgresql://patchmon_user:${DB_PASSWORD}@database:5432/patchmon_db

  # Security (generate with: openssl rand -hex 32)
  JWT_SECRET: ${JWT_SECRET}

  # Redis
  REDIS_HOST: redis
  REDIS_PORT: 6379
  REDIS_PASSWORD: ${REDIS_PASSWORD}

  # Server URLs
  SERVER_PROTOCOL: https
  SERVER_HOST: patchmon.example.com
  SERVER_PORT: 443
  CORS_ORIGIN: https://patchmon.example.com

  # OIDC Configuration (Optional)
  OIDC_ENABLED: true
  OIDC_ISSUER_URL: https://auth.example.com
  OIDC_CLIENT_ID: patchmon-client-id
  OIDC_CLIENT_SECRET: ${OIDC_CLIENT_SECRET}
  OIDC_REDIRECT_URI: https://patchmon.example.com/api/v1/oidc/callback
  OIDC_POST_LOGOUT_URI: https://patchmon.example.com
  OIDC_SCOPES: openid profile email groups
  OIDC_AUTO_CREATE_USERS: true
  OIDC_SYNC_ROLES: true
  OIDC_DISABLE_LOCAL_AUTH: false

  # OIDC Group-to-Role Mapping
  OIDC_SUPERADMIN_GROUP: "PatchMon SuperAdmins"
  OIDC_ADMIN_GROUP: "PatchMon Admins"
  OIDC_HOST_MANAGER_GROUP: "PatchMon Host Managers"
  OIDC_USER_GROUP: "PatchMon Users"
  OIDC_READONLY_GROUP: "PatchMon Readonly"
```

**Security Note:** Always use environment variable substitution (`${VAR}`) or secrets management for sensitive values like passwords, secrets, and API keys. Never commit actual credentials to version control.

---

## Database Schema Changes

### New Tables

#### `compliance_profiles`
Stores compliance scan profile definitions.

```sql
CREATE TABLE "compliance_profiles" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "type" TEXT NOT NULL,           -- "openscap" or "docker-bench"
    "os_family" TEXT,               -- "ubuntu", "rhel", "debian"
    "version" TEXT,
    "description" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,
    CONSTRAINT "compliance_profiles_pkey" PRIMARY KEY ("id")
);
```

#### `compliance_scans`
Stores compliance scan execution records.

```sql
CREATE TABLE "compliance_scans" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "profile_id" TEXT NOT NULL,
    "started_at" TIMESTAMP(3) NOT NULL,
    "completed_at" TIMESTAMP(3),
    "status" TEXT NOT NULL,         -- "running", "completed", "failed"
    "total_rules" INTEGER NOT NULL DEFAULT 0,
    "passed" INTEGER NOT NULL DEFAULT 0,
    "failed" INTEGER NOT NULL DEFAULT 0,
    "warnings" INTEGER NOT NULL DEFAULT 0,
    "skipped" INTEGER NOT NULL DEFAULT 0,
    "not_applicable" INTEGER NOT NULL DEFAULT 0,
    "score" DOUBLE PRECISION,       -- Percentage 0-100
    "error_message" TEXT,
    "raw_output" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,
    CONSTRAINT "compliance_scans_pkey" PRIMARY KEY ("id")
);
```

#### `compliance_rules`
Stores compliance rule definitions from scan profiles.

```sql
CREATE TABLE "compliance_rules" (
    "id" TEXT NOT NULL,
    "profile_id" TEXT NOT NULL,
    "rule_ref" TEXT NOT NULL,       -- Original rule ID from CIS/OpenSCAP
    "title" TEXT NOT NULL,
    "description" TEXT,
    "rationale" TEXT,
    "severity" TEXT,                -- "low", "medium", "high", "critical"
    "section" TEXT,                 -- CIS section number
    "remediation" TEXT,
    CONSTRAINT "compliance_rules_pkey" PRIMARY KEY ("id")
);
```

#### `compliance_results`
Stores individual rule results from scans.

```sql
CREATE TABLE "compliance_results" (
    "id" TEXT NOT NULL,
    "scan_id" TEXT NOT NULL,
    "rule_id" TEXT NOT NULL,
    "status" TEXT NOT NULL,         -- "pass", "fail", "warn", "skip", "notapplicable", "error"
    "finding" TEXT,
    "actual" TEXT,
    "expected" TEXT,
    "remediation" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "compliance_results_pkey" PRIMARY KEY ("id")
);
```

#### `audit_logs`
Stores security audit events.

```sql
CREATE TABLE "audit_logs" (
    "id" TEXT NOT NULL,
    "event" TEXT NOT NULL,
    "user_id" TEXT,
    "target_user_id" TEXT,
    "ip_address" TEXT,
    "user_agent" TEXT,
    "request_id" TEXT,
    "details" TEXT,                 -- JSON stringified
    "success" BOOLEAN NOT NULL DEFAULT true,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "audit_logs_pkey" PRIMARY KEY ("id")
);
```

### Modified Tables

#### `hosts` - New Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `compliance_enabled` | Boolean | `false` | Compliance scanning enabled |
| `compliance_on_demand_only` | Boolean | `true` | Only run compliance on-demand |

#### `users` - New Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `password_hash` | String | NULL | Now nullable for OIDC users |
| `oidc_sub` | String | NULL | OIDC subject identifier |
| `oidc_provider` | String | NULL | OIDC provider name |
| `avatar_url` | String | NULL | User avatar URL |

#### `role_permissions` - New Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `can_manage_superusers` | Boolean | `false` | Permission to manage superadmin users |

#### `settings` - New Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `ai_enabled` | Boolean | `false` | AI assistant enabled |
| `ai_provider` | String | `openrouter` | AI provider name |
| `ai_model` | String | NULL | AI model identifier |
| `ai_api_key` | String | NULL | Encrypted AI API key |
| `github_repo_url` | String | Changed default | Now points to PatchMon-Enhanced |

### New Indexes

Added performance indexes on:
- `hosts`: `status`, `status + api_id`, `last_update`
- `users`: `role`
- `host_repositories`: `host_id`, `repository_id`, `is_enabled`
- `repositories`: `is_active`, `name`
- `update_history`: `host_id`, `timestamp`, `status`, `host_id + timestamp`
- All compliance tables: Various indexes for query optimization

---

## API Endpoints

### New Route Files

1. **`/api/v1/oidc/*`** - OIDC authentication routes
2. **`/api/v1/compliance/*`** - Compliance scanning routes
3. **`/api/v1/ai/*`** - AI assistant routes

### OIDC Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/oidc/config` | Get OIDC configuration (public) |
| GET | `/oidc/login` | Initiate OIDC login flow |
| GET | `/oidc/callback` | OIDC callback handler |
| POST | `/oidc/logout` | OIDC logout |
| GET | `/oidc/userinfo` | Get current user info |

### Compliance Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/compliance/profiles` | List available scan profiles |
| GET | `/compliance/profiles/:id` | Get profile details |
| POST | `/compliance/scan` | Trigger compliance scan |
| POST | `/compliance/bulk-trigger` | Trigger bulk scans |
| GET | `/compliance/scans` | List scan history |
| GET | `/compliance/scans/:id` | Get scan details |
| GET | `/compliance/scans/:id/results` | Get scan results |
| GET | `/compliance/scans/active` | Get active scans |
| GET | `/compliance/hosts/:hostId/latest` | Get latest scan for host |
| GET | `/compliance/hosts/:hostId/history` | Get scan history for host |
| GET | `/compliance/hosts/:hostId/trend` | Get compliance trend |
| GET | `/compliance/dashboard/stats` | Get dashboard statistics |
| DELETE | `/compliance/scans/:id` | Delete scan record |

### AI Assistant Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/ai/status` | Get AI status (all users) |
| GET | `/ai/providers` | Get available providers |
| GET | `/ai/settings` | Get AI settings (admin) |
| PUT | `/ai/settings` | Update AI settings (admin) |
| POST | `/ai/test` | Test AI connection |
| POST | `/ai/assist` | Get AI assistance |
| POST | `/ai/complete` | Get command completion |

### Host Integration Endpoints (New/Modified)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/hosts/:hostId/integrations` | Get integration status |
| POST | `/hosts/:hostId/integrations/:name/toggle` | Toggle integration |
| GET | `/hosts/:hostId/integrations/:name/status` | Get setup status |
| POST | `/hosts/:hostId/compliance/on-demand-only` | Set compliance mode |
| POST | `/hosts/:hostId/refresh-integration-status` | Refresh status |
| POST | `/hosts/:hostId/refresh-docker` | Refresh Docker data |

---

## WebSocket Protocol

### Server → Agent Messages

| Type | Description | Payload |
|------|-------------|---------|
| `connected` | Connection established | - |
| `settings_update` | Update agent settings | `{ update_interval }` |
| `report_now` | Trigger immediate report | - |
| `update_agent` | Trigger agent update | - |
| `refresh_integration_status` | Refresh integration status | - |
| `docker_inventory_refresh` | Refresh Docker inventory | - |
| `integration_toggle` | Toggle integration | `{ integration, enabled }` |
| `compliance_scan` | Trigger compliance scan | `{ profile_id, profile_type, enable_remediation, fetch_remote_resources }` |
| `upgrade_ssg` | Upgrade SSG content | - |
| `remediate_rule` | Remediate single rule | `{ profile_id, profile_type, rule_id }` |
| `docker_image_scan` | Scan Docker image | `{ image_name, container_name, scan_all_images }` |
| `set_compliance_on_demand_only` | Set compliance mode | `{ on_demand_only }` |
| `update_notification` | Agent update available | `{ version, download_url }` |

### Agent → Server Messages

| Type | Description |
|------|-------------|
| `pong` | Keepalive response |
| `integration_status` | Integration setup status update |
| `compliance_progress` | Scan progress update |
| `compliance_result` | Scan completion with results |
| `ssg_upgrade_result` | SSG upgrade completion |
| `remediation_result` | Remediation completion |

---

## Agent Configuration

### config.yml

```yaml
# PatchMon Agent Configuration
patchmon_server: "https://your-server.com"
api_version: "v1"
credentials_file: "/etc/patchmon/credentials.yml"
log_file: "/etc/patchmon/logs/patchmon-agent.log"
log_level: "info"
skip_ssl_verify: false
update_interval: 60          # Minutes between reports
report_offset: 0             # Seconds offset for report timing

# Integration settings
integrations:
  docker: false              # Enable Docker monitoring

# Compliance settings
compliance_on_demand_only: true  # Only run compliance when triggered from UI
```

### credentials.yml

```yaml
api_id: "your-api-id"
api_key: "your-api-key"
```

---

## Migration Notes

### For New Installations

1. Run standard Prisma migration:
   ```bash
   npx prisma migrate deploy
   ```

2. Default admin user created on first startup

3. Configure OIDC if desired via environment variables

### For Existing Installations (Upgrading from v1.3.7)

1. **Database Migration:**
   ```bash
   npx prisma migrate deploy
   ```

   If migration fails with P3009 error:
   ```bash
   npx prisma migrate resolve --applied 0001_init
   ```

2. **Agent Update:**
   - Download agent v1.5.55 from releases
   - Replace existing binary
   - Restart agent service

3. **Environment Variables:**
   - No required changes for basic functionality
   - Add OIDC variables if enabling SSO
   - Add AI variables if enabling AI assistant

4. **Docker Compose:**
   - Update image tags to latest
   - Add any new environment variables needed

---

## Testing Checklist

- [x] OIDC login flow works with various providers
- [x] Group-to-role mapping assigns correct roles
- [x] Compliance scans complete successfully
- [x] Scan results display correctly in UI
- [x] Single rule remediation works
- [x] Bulk scan trigger works
- [x] SSH terminal connects and functions
- [x] AI assistant provides useful responses
- [x] Compliance on-demand toggle persists
- [x] Agent handles all WebSocket messages
- [x] Database migrations apply cleanly
- [x] Go lint passes (`go fmt`, `go vet`)
- [x] Frontend lint passes (Biome)

---

## Version Information

| Component | Upstream Version | Enhanced Version |
|-----------|------------------|------------------|
| PatchMon (Backend/Frontend) | 1.3.7 | 1.7.77 |
| PatchMon-agent | 1.3.7 | 1.5.55 |

---

## Files Changed Summary

### Backend
- **29 route files** modified/added (+5,417 lines, -962 lines)
- **Prisma schema** with 4 new tables, multiple field additions
- **Services** for OIDC, compliance, AI, WebSocket enhancements

### Frontend
- **New pages**: Compliance dashboard, SSH terminal
- **Enhanced pages**: Host detail with integrations tab
- **New components**: Compliance charts, scan progress, AI panel

### Agent
- **83 commits** with compliance integration, WebSocket handlers
- **New packages**: Compliance scanning, remediation support
- **Security fixes**: Input validation, race condition prevention

---

## Breaking Changes

1. **Password field nullable**: `users.password_hash` is now nullable for OIDC-only users
2. **GitHub repo URL default**: Changed to PatchMon-Enhanced repository
3. **New required permissions**: `can_manage_superusers` added to role_permissions

---

## Contributors

- MacJediWizard
- Claude Opus 4.5 (AI Assistant)

---

**This document is ready for review. Please verify all details before creating the PR.**
