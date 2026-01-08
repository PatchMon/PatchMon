# OIDC Authentication for PatchMon

This document covers OpenID Connect (OIDC) authentication setup for PatchMon, including configuration, database requirements, and troubleshooting.

## Overview

PatchMon supports OIDC authentication with any compliant identity provider (IdP), including:
- Authentik
- Keycloak
- Okta
- Azure AD
- Google Workspace

Features:
- Single Sign-On (SSO) login
- Automatic user provisioning on first login
- Group-based role mapping (admin/user)
- Optional: Disable local password authentication

## Prerequisites

1. An OIDC-compatible Identity Provider
2. A configured OAuth2/OIDC application in your IdP
3. PatchMon database migrations applied (see [Database Requirements](#database-requirements))

## Database Requirements

OIDC support requires additional database columns. The migration file is located at:
`backend/prisma/migrations/20260102000000_add_oidc_and_audit_logs/migration.sql`

### Required Schema Changes

```sql
-- Add OIDC fields to users table
ALTER TABLE "users" ADD COLUMN IF NOT EXISTS "oidc_sub" TEXT;
ALTER TABLE "users" ADD COLUMN IF NOT EXISTS "oidc_provider" TEXT;

-- Make password_hash nullable for OIDC-only users
ALTER TABLE "users" ALTER COLUMN "password_hash" DROP NOT NULL;

-- Create unique index on oidc_sub
CREATE UNIQUE INDEX IF NOT EXISTS "users_oidc_sub_key" ON "users"("oidc_sub");

-- Create audit_logs table
CREATE TABLE IF NOT EXISTS "audit_logs" (
    "id" TEXT NOT NULL,
    "event" TEXT NOT NULL,
    "user_id" TEXT,
    "target_user_id" TEXT,
    "ip_address" TEXT,
    "user_agent" TEXT,
    "request_id" TEXT,
    "details" JSONB,
    "success" BOOLEAN NOT NULL DEFAULT true,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "audit_logs_pkey" PRIMARY KEY ("id")
);

-- Create indexes for audit_logs
CREATE INDEX IF NOT EXISTS "audit_logs_event_idx" ON "audit_logs"("event");
CREATE INDEX IF NOT EXISTS "audit_logs_user_id_idx" ON "audit_logs"("user_id");
CREATE INDEX IF NOT EXISTS "audit_logs_created_at_idx" ON "audit_logs"("created_at");
```

### Applying Migrations

**For existing installations:**
```bash
# Using Prisma
cd backend
npx prisma migrate deploy

# Or run SQL directly against your database
psql -U patchmon_user -d patchmon_db -f backend/prisma/migrations/20260102000000_add_oidc_and_audit_logs/migration.sql
```

**Important:** The `password_hash` column MUST be nullable for OIDC-only users (they don't have passwords).

## Configuration

### Environment Variables

Add these to your backend environment (docker-compose.yml or .env file):

#### Required OIDC Settings

| Variable | Description | Example |
|----------|-------------|---------|
| `OIDC_ENABLED` | Enable OIDC authentication | `true` |
| `OIDC_ISSUER_URL` | IdP discovery URL | `https://auth.example.com/application/o/patchmon/` |
| `OIDC_CLIENT_ID` | OAuth client ID from IdP | `patchmon-client-id` |
| `OIDC_CLIENT_SECRET` | OAuth client secret from IdP | `your-client-secret` |
| `OIDC_REDIRECT_URI` | Callback URL (must match IdP config) | `https://patchmon.example.com/api/v1/auth/oidc/callback` |

#### Optional OIDC Settings

| Variable | Description | Default |
|----------|-------------|---------|
| `OIDC_SCOPES` | Scopes to request (space-separated) | `openid email profile groups` |
| `OIDC_AUTO_CREATE_USERS` | Auto-create users on first login | `true` |
| `OIDC_DEFAULT_ROLE` | Default role for new users | `user` |
| `OIDC_DISABLE_LOCAL_AUTH` | Hide username/password login fields | `false` |
| `OIDC_BUTTON_TEXT` | Text for SSO login button | `Login with SSO` |

#### Group-Based Role Mapping

| Variable | Description | Default |
|----------|-------------|---------|
| `OIDC_ADMIN_GROUP` | IdP group name for admin role | `PatchMon Admins` |
| `OIDC_USER_GROUP` | IdP group name for user role | `PatchMon Users` |
| `OIDC_SYNC_ROLES` | Update role on every login | `true` |

### Docker Compose Configuration

When using Docker Compose with environment variables, you must explicitly pass OIDC variables to the backend service:

```yaml
services:
  backend:
    environment:
      # ... other variables ...

      # OIDC Configuration
      OIDC_ENABLED: ${OIDC_ENABLED:-false}
      OIDC_ISSUER_URL: ${OIDC_ISSUER_URL}
      OIDC_CLIENT_ID: ${OIDC_CLIENT_ID}
      OIDC_CLIENT_SECRET: ${OIDC_CLIENT_SECRET}
      OIDC_REDIRECT_URI: ${OIDC_REDIRECT_URI}
      OIDC_SCOPES: ${OIDC_SCOPES:-openid email profile groups}
      OIDC_AUTO_CREATE_USERS: ${OIDC_AUTO_CREATE_USERS:-true}
      OIDC_DEFAULT_ROLE: ${OIDC_DEFAULT_ROLE:-user}
      OIDC_DISABLE_LOCAL_AUTH: ${OIDC_DISABLE_LOCAL_AUTH:-false}
      OIDC_BUTTON_TEXT: ${OIDC_BUTTON_TEXT:-Login with SSO}
      OIDC_ADMIN_GROUP: ${OIDC_ADMIN_GROUP:-PatchMon Admins}
      OIDC_USER_GROUP: ${OIDC_USER_GROUP:-PatchMon Users}
      OIDC_SYNC_ROLES: ${OIDC_SYNC_ROLES:-true}
```

Then set the actual values in a `.env` file or your deployment environment:

```bash
# .env file
OIDC_ENABLED=true
OIDC_ISSUER_URL=https://auth.example.com/application/o/patchmon/
OIDC_CLIENT_ID=your-client-id
OIDC_CLIENT_SECRET=your-client-secret
OIDC_REDIRECT_URI=https://patchmon.example.com/api/v1/auth/oidc/callback
OIDC_DISABLE_LOCAL_AUTH=true
OIDC_ADMIN_GROUP=PatchMon Admins
OIDC_USER_GROUP=PatchMon Users
```

## Identity Provider Setup

### Authentik Configuration

1. Create a new OAuth2/OIDC Provider in Authentik
2. Set the redirect URI to: `https://your-patchmon-domain/api/v1/auth/oidc/callback`
3. Enable the following scopes: `openid`, `email`, `profile`, `groups`
4. Create groups for role mapping (e.g., "PatchMon Admins", "PatchMon Users")
5. Assign users to appropriate groups
6. Copy the Client ID and Client Secret to your PatchMon configuration
7. Set `OIDC_ISSUER_URL` to your Authentik application URL (e.g., `https://auth.example.com/application/o/patchmon/`)

### Keycloak Configuration

1. Create a new Client in your Keycloak realm
2. Set Access Type to `confidential`
3. Add the redirect URI: `https://your-patchmon-domain/api/v1/auth/oidc/callback`
4. Enable required scopes
5. Create groups and map them to your PatchMon roles
6. Set `OIDC_ISSUER_URL` to: `https://keycloak.example.com/realms/your-realm`

## First-Time Setup with OIDC

When no users exist in the database, PatchMon displays a setup wizard. With OIDC-only mode (`OIDC_DISABLE_LOCAL_AUTH=true`), you have two options:

### Option 1: Create First User via OIDC (Recommended)

1. Ensure your IdP user is in the admin group (e.g., "PatchMon Admins")
2. Set `OIDC_AUTO_CREATE_USERS=true`
3. Login via OIDC - the first user will be created as admin

### Option 2: Create Placeholder User via SQL

If Option 1 doesn't work due to the setup wizard blocking access:

```sql
-- Insert a placeholder admin user to bypass setup wizard
INSERT INTO users (id, username, email, role, active, created_at, updated_at)
VALUES (
  gen_random_uuid()::text,
  'placeholder-admin',
  'placeholder@local',
  'admin',
  true,
  NOW(),
  NOW()
);
```

After your first OIDC login, delete the placeholder:
```sql
DELETE FROM users WHERE username = 'placeholder-admin';
```

## Troubleshooting

### Common Issues

#### "column users.oidc_sub does not exist"
The OIDC database migration hasn't been applied. Run the migration SQL or use `npx prisma migrate deploy`.

#### "null value in column password_hash violates not-null constraint"
The migration to make `password_hash` nullable wasn't applied. Run:
```sql
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;
```

#### OIDC_DISABLE_LOCAL_AUTH Not Working
Environment variables must be explicitly passed in docker-compose.yml. Add:
```yaml
OIDC_DISABLE_LOCAL_AUTH: ${OIDC_DISABLE_LOCAL_AUTH:-false}
```

#### "What's New" Popup Appears Every Time
This was a bug where OIDC users' accepted release notes weren't being loaded on login. Fixed in the `/profile` endpoint to include `accepted_release_notes_versions`. Ensure you're running the latest code.

#### Login Fails with "Invalid state" or "PKCE error"
1. Ensure the redirect URI in PatchMon matches exactly what's configured in your IdP
2. Check that cookies are enabled (OIDC uses httpOnly cookies for session management)
3. Verify your IdP supports PKCE (most modern IdPs do)

#### Docker Build Not Picking Up Changes
Use `--no-cache` flag when rebuilding:
```bash
docker compose build --no-cache backend
```

Or in Komodo, add `--no-cache` to the Extra Args field in Build configuration.

### Debug Logging

Enable debug logging to troubleshoot OIDC issues:

```yaml
environment:
  LOG_LEVEL: debug
```

Check backend logs for OIDC-related messages:
```bash
docker compose logs -f backend | grep -i oidc
```

## Security Considerations

1. **Client Secret**: Keep `OIDC_CLIENT_SECRET` secure. Never commit it to version control.

2. **HTTPS Required**: OIDC callbacks should always use HTTPS in production.

3. **Token Storage**: OIDC tokens are stored in httpOnly cookies, not localStorage, to prevent XSS attacks.

4. **Session Management**: Sessions use secure, httpOnly cookies with proper SameSite attributes.

5. **Role Sync**: With `OIDC_SYNC_ROLES=true`, user roles update on every login based on IdP groups. Disabling this means roles are only set on first login.

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/auth/oidc/login` | Initiates OIDC login flow |
| `GET /api/v1/auth/oidc/callback` | Handles IdP callback (redirect URI) |
| `GET /api/v1/auth/oidc/config` | Returns OIDC frontend configuration |

## Changes from Standard PatchMon

The OIDC feature branch includes these modifications:

### Database Schema
- Added `oidc_sub` and `oidc_provider` columns to `users` table
- Made `password_hash` nullable
- Added `audit_logs` table for security logging

### Backend Changes
- `backend/src/routes/oidcRoutes.js` - OIDC authentication routes
- `backend/src/auth/oidc.js` - OIDC client configuration
- `backend/src/routes/authRoutes.js` - Updated `/profile` endpoint to include accepted release notes
- `backend/src/utils/auditLogger.js` - Security audit logging

### Frontend Changes
- `frontend/src/pages/Login.jsx` - SSO button and conditional field display
- `frontend/src/contexts/AuthContext.jsx` - Cookie-based auth for OIDC, credentials include for API calls
