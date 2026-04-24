#!/bin/bash
# Migration script: PatchMon 1.4.x (Node stack) -> 2.0.0 (Go binary, native install)
#
# Converts the legacy split-env layout
#   /opt/patchmon/backend/.env   (Node/Prisma: DATABASE_URL, JWT_SECRET, PORT=3001,
#                                  CORS_ORIGIN, TRUST_PROXY, OIDC_*, rate-limits, ...)
#   /opt/patchmon/frontend/.env  (Vite: VITE_APP_NAME, VITE_APP_VERSION)
# into a single /opt/patchmon/.env consumed by the Go binary.
#
# Usage (standalone):
#   ./migrate1-4-2_to_2-0-0.sh <old_backend_env> <old_frontend_env|-> <new_env_output> [redis_password] [local_ip]
#
# Example:
#   ./migrate1-4-2_to_2-0-0.sh \
#       /opt/patchmon/backend/.env \
#       /opt/patchmon/frontend/.env \
#       /opt/patchmon/.env \
#       "$(openssl rand -hex 32)" \
#       192.168.1.50
#
# Pass "-" for the frontend env argument if it does not exist.
#
# The Proxmox community-scripts update_script() fetches this file from the repo
# and runs it after wiping the legacy Node directories. This keeps the env-merge
# logic in one place (tested + reviewed here, not duplicated in shell-one-liners).

set -euo pipefail

usage() {
  cat >&2 <<EOF
Usage: $0 <old_backend_env> <old_frontend_env|-> <new_env_output> [redis_password] [local_ip]
EOF
  exit 1
}

[[ $# -lt 3 ]] && usage

OLD_BACKEND_ENV="$1"
OLD_FRONTEND_ENV="$2"
NEW_ENV="$3"
REDIS_PASSWORD_IN="${4:-}"
LOCAL_IP_IN="${5:-}"

[[ ! -f "${OLD_BACKEND_ENV}" ]] && { echo "ERROR: backend env not found: ${OLD_BACKEND_ENV}" >&2; exit 1; }
[[ "${OLD_FRONTEND_ENV}" != "-" && ! -f "${OLD_FRONTEND_ENV}" ]] && { echo "ERROR: frontend env not found: ${OLD_FRONTEND_ENV}" >&2; exit 1; }

# ──────────────────────────────────────────────────────────────────────────────
# 1. Load legacy backend env into an associative array of active KEY=VALUE
# ──────────────────────────────────────────────────────────────────────────────
# We do NOT `source` the old file because it may contain `${VAR}` interpolations
# that bash would expand to empty strings. We read lines verbatim and split on
# the first `=`. Quotes around values are stripped only when the FIRST and LAST
# character form a matched pair AND the value is not empty.

declare -A OLD
while IFS= read -r line || [[ -n "${line}" ]]; do
  [[ -z "${line}" ]] && continue
  [[ "${line}" =~ ^[[:space:]]*# ]] && continue
  [[ ! "${line}" =~ ^[A-Z_][A-Z0-9_]*= ]] && continue
  key="${line%%=*}"
  value="${line#*=}"
  # Strip surrounding quotes only when first and last chars match and len >= 2.
  # Guards against greedy-match errors on values like `FOO="bar"baz"`.
  if [[ ${#value} -ge 2 ]]; then
    first="${value:0:1}"
    last="${value: -1}"
    if [[ "${first}" == '"' && "${last}" == '"' ]] || [[ "${first}" == "'" && "${last}" == "'" ]]; then
      value="${value:1:${#value}-2}"
    fi
  fi
  OLD["${key}"]="${value}"
done <"${OLD_BACKEND_ENV}"

# ──────────────────────────────────────────────────────────────────────────────
# 2. CORS_ORIGIN / CORS_ORIGINS merge (historic plural variant was used briefly)
# ──────────────────────────────────────────────────────────────────────────────
# Combine both into a single deduplicated comma-separated CORS_ORIGIN.
if [[ -n "${OLD[CORS_ORIGINS]:-}" ]]; then
  combined="${OLD[CORS_ORIGIN]:-},${OLD[CORS_ORIGINS]}"
  OLD[CORS_ORIGIN]="$(echo "${combined}" | tr ',' '\n' | sed '/^$/d' | awk '!seen[$0]++' | tr '\n' ',' | sed 's/,$//')"
  unset 'OLD[CORS_ORIGINS]'
fi

# ──────────────────────────────────────────────────────────────────────────────
# 3. Deterministic transforms required by the Node → Go move
# ──────────────────────────────────────────────────────────────────────────────
# PORT: Node backend listened on 3001 and nginx proxied :80/:443 → :3001.
# Go server serves frontend + API on a single port. Default is 3000.
if [[ "${OLD[PORT]:-}" == "3001" ]]; then
  OLD[PORT]="3000"
fi

# CORS_ORIGIN: drop any entries that point at the old :3001 backend port
# (users may have added internal hosts like http://localhost:3001). Replace
# :3001 with :3000 so the new origin list matches the new server port.
# Also ensure the LXC LAN IP is present so users can hit http://<LOCAL_IP>:3000.
if [[ -n "${OLD[CORS_ORIGIN]:-}" ]]; then
  mapped="$(echo "${OLD[CORS_ORIGIN]}" | sed -E 's/:3001\b/:3000/g')"
  OLD[CORS_ORIGIN]="${mapped}"
fi
if [[ -n "${LOCAL_IP_IN}" ]]; then
  new_origin="http://${LOCAL_IP_IN}:3000"
  if [[ -z "${OLD[CORS_ORIGIN]:-}" ]]; then
    OLD[CORS_ORIGIN]="${new_origin}"
  elif ! echo ",${OLD[CORS_ORIGIN]}," | grep -q ",${new_origin},"; then
    OLD[CORS_ORIGIN]="${OLD[CORS_ORIGIN]},${new_origin}"
  fi
fi

# Drop legacy-only vars — the Go server does not read these.
unset 'OLD[SERVER_PROTOCOL]'
unset 'OLD[SERVER_HOST]'
unset 'OLD[SERVER_PORT]'

# POSTGRES_HOST=database is a Docker-compose remnant; native install is always localhost.
[[ "${OLD[POSTGRES_HOST]:-}" == "database" ]] && OLD[POSTGRES_HOST]="localhost"
# REDIS_HOST=redis is a Docker-compose remnant; native install is always 127.0.0.1.
[[ "${OLD[REDIS_HOST]:-}" == "redis" ]] && OLD[REDIS_HOST]="127.0.0.1"
[[ -z "${OLD[REDIS_HOST]:-}" ]] && OLD[REDIS_HOST]="127.0.0.1"
[[ -z "${OLD[REDIS_PORT]:-}" ]] && OLD[REDIS_PORT]="6379"

# AGENTS_DIR — must point at the absolute native-install path regardless of
# what the old value was (Docker default was "agents", relative to the server
# working dir; Go server still honours relative paths but absolute is clearer).
OLD[AGENTS_DIR]="/opt/patchmon/agents"

# Redis password — the native install requires one; if the old install never
# set it (Node stack happily ran without), use the password passed in by the
# upgrade driver.
if [[ -z "${OLD[REDIS_PASSWORD]:-}" && -n "${REDIS_PASSWORD_IN}" ]]; then
  OLD[REDIS_PASSWORD]="${REDIS_PASSWORD_IN}"
fi

# ──────────────────────────────────────────────────────────────────────────────
# 4. Critical secrets — generate if absent, NEVER overwrite an existing value.
# ──────────────────────────────────────────────────────────────────────────────
# SESSION_SECRET is used as a fallback AES-GCM key when AI_ENCRYPTION_KEY is
# unset (internal/util/encryption.go). If any 1.4.x install ever set it, rows
# encrypted with that key must remain decryptable. DO NOT regenerate if old
# value exists.
if [[ -z "${OLD[SESSION_SECRET]:-}" ]]; then
  OLD[SESSION_SECRET]="$(openssl rand -hex 64)"
fi

# AI_ENCRYPTION_KEY is the primary AES-GCM key. Introduced in 1.5.x; 1.4.x
# installs won't have it. Generating a new one is safe because legacy rows
# were encrypted with SESSION_SECRET (the fallback), which we preserve above.
if [[ -z "${OLD[AI_ENCRYPTION_KEY]:-}" ]]; then
  OLD[AI_ENCRYPTION_KEY]="$(openssl rand -hex 64)"
fi

# JWT_SECRET — if missing, generate. Existing sessions signed with the old
# value will be invalidated (users log in again).
if [[ -z "${OLD[JWT_SECRET]:-}" ]]; then
  OLD[JWT_SECRET]="$(openssl rand -hex 64)"
fi

# ──────────────────────────────────────────────────────────────────────────────
# 5. Write new single .env. Order: core → secrets → server → optional overrides.
# ──────────────────────────────────────────────────────────────────────────────
# Allowlist of optional env vars that the Go server reads. Any value the user
# had set in the legacy env is carried forward; otherwise the line is omitted
# (the server uses its built-in defaults).

OPTIONAL_VARS=(
  # Server
  TRUST_PROXY ENABLE_HSTS TZ TIMEZONE FRONTEND_URL
  # Logging
  ENABLE_LOGGING LOG_LEVEL
  # Auth / lockout
  MAX_LOGIN_ATTEMPTS LOCKOUT_DURATION_MINUTES SESSION_INACTIVITY_TIMEOUT_MINUTES
  AUTH_BROWSER_SESSION_COOKIES JWT_EXPIRES_IN
  # TFA
  MAX_TFA_ATTEMPTS TFA_LOCKOUT_DURATION_MINUTES
  TFA_REMEMBER_ME_EXPIRES_IN TFA_MAX_REMEMBER_SESSIONS
  # Password policy
  PASSWORD_MIN_LENGTH PASSWORD_REQUIRE_UPPERCASE PASSWORD_REQUIRE_LOWERCASE
  PASSWORD_REQUIRE_NUMBER PASSWORD_REQUIRE_SPECIAL
  # Body / rate limits
  JSON_BODY_LIMIT AGENT_UPDATE_BODY_LIMIT
  RATE_LIMIT_WINDOW_MS RATE_LIMIT_MAX
  AUTH_RATE_LIMIT_WINDOW_MS AUTH_RATE_LIMIT_MAX
  AGENT_RATE_LIMIT_WINDOW_MS AGENT_RATE_LIMIT_MAX
  PASSWORD_RATE_LIMIT_WINDOW_MS PASSWORD_RATE_LIMIT_MAX
  # DB pool / startup
  PM_DB_CONN_MAX_ATTEMPTS PM_DB_CONN_WAIT_INTERVAL
  DB_CONNECTION_LIMIT DB_CONNECT_TIMEOUT DB_TRANSACTION_LONG_TIMEOUT
  # Default role
  DEFAULT_USER_ROLE
  # Redis TLS
  REDIS_TLS REDIS_TLS_VERIFY REDIS_TLS_CA
  REDIS_CONNECT_TIMEOUT_MS REDIS_COMMAND_TIMEOUT_MS
  # OIDC / SSO
  OIDC_ENABLED OIDC_ISSUER_URL OIDC_CLIENT_ID OIDC_CLIENT_SECRET OIDC_REDIRECT_URI
  OIDC_SCOPES OIDC_AUTO_CREATE_USERS OIDC_DEFAULT_ROLE OIDC_DISABLE_LOCAL_AUTH
  OIDC_BUTTON_TEXT OIDC_SESSION_TTL OIDC_POST_LOGOUT_URI OIDC_SYNC_ROLES
  OIDC_ADMIN_GROUP OIDC_SUPERADMIN_GROUP OIDC_HOST_MANAGER_GROUP
  OIDC_READONLY_GROUP OIDC_USER_GROUP OIDC_ENFORCE_HTTPS
  # RDP / Guacamole
  GUACD_PATH GUACD_ADDRESS
  # Agent binaries
  AGENT_BINARIES_DIR
)

write_kv() {
  # Emit KEY=VALUE in a form the Go server's godotenv parser
  # (github.com/joho/godotenv v1.5.1) will interpret verbatim. godotenv treats
  # unquoted `#` preceded by whitespace as a comment start and expands
  # `$[A-Z0-9_]+` in both unquoted and double-quoted values (but NOT in
  # single-quoted values). Escape rules for double-quoted: `\\` → `\`,
  # `\"` → `"`, `\$` → `$` (preserved literal).
  #
  # Strategy: if the value contains ONLY characters safe to write unquoted,
  # emit it raw. Otherwise, wrap in double quotes and escape `\`, `"`, `$`.
  local k="$1" v="$2"
  if [[ -z "${v}" ]]; then
    printf '%s=\n' "${k}" >>"${NEW_ENV}"
    return
  fi
  # Safe chars: alphanumerics, and . + - _ = : / @ , ; ? & %
  # Everything else (whitespace, #, $, `, ", \, !, (, ), [, ], {, }, <, >, *,
  # quotes, etc.) → quote + escape.
  if [[ "${v}" =~ ^[A-Za-z0-9._+=:/@,\;\?\&%-]+$ ]]; then
    printf '%s=%s\n' "${k}" "${v}" >>"${NEW_ENV}"
  else
    local escaped="${v//\\/\\\\}"   # \ → \\
    escaped="${escaped//\"/\\\"}"   # " → \"
    escaped="${escaped//\$/\\\$}"   # $ → \$
    printf '%s="%s"\n' "${k}" "${escaped}" >>"${NEW_ENV}"
  fi
}

redact_db_url() {
  # Show scheme + user, hide password + host. "postgresql://u:p@h/db" → "postgresql://u:…@…"
  local url="$1"
  if [[ "${url}" =~ ^([^:]+://[^:@/]+): ]]; then
    printf '%s:…@…' "${BASH_REMATCH[1]}"
  else
    printf '…'
  fi
}

{
  echo "# PatchMon 2.0.0 .env (converted from 1.4.x by $(basename "$0") on $(date -u +%Y-%m-%dT%H:%M:%SZ))"
  echo "# Full variable reference: https://docs.patchmon.net"
  echo ""
  echo "APP_ENV=${OLD[APP_ENV]:-production}"
  echo ""
  echo "# ── Database ──────────────────────────────────────────────────────────"
} >"${NEW_ENV}"

write_kv DATABASE_URL "${OLD[DATABASE_URL]}"
{
  echo ""
  echo "# ── Redis ─────────────────────────────────────────────────────────────"
} >>"${NEW_ENV}"
write_kv REDIS_HOST "${OLD[REDIS_HOST]}"
write_kv REDIS_PORT "${OLD[REDIS_PORT]}"
write_kv REDIS_PASSWORD "${OLD[REDIS_PASSWORD]:-}"
write_kv REDIS_DB "${OLD[REDIS_DB]:-0}"

{
  echo ""
  echo "# ── Secrets (preserved from 1.4.x when present — do not rotate) ──────"
} >>"${NEW_ENV}"
write_kv JWT_SECRET "${OLD[JWT_SECRET]}"
write_kv SESSION_SECRET "${OLD[SESSION_SECRET]}"
write_kv AI_ENCRYPTION_KEY "${OLD[AI_ENCRYPTION_KEY]}"

{
  echo ""
  echo "# ── Server ────────────────────────────────────────────────────────────"
} >>"${NEW_ENV}"
write_kv PORT "${OLD[PORT]:-3000}"
write_kv CORS_ORIGIN "${OLD[CORS_ORIGIN]:-}"

{
  echo ""
  echo "# ── Agents ────────────────────────────────────────────────────────────"
} >>"${NEW_ENV}"
write_kv AGENTS_DIR "${OLD[AGENTS_DIR]}"

# Optional vars — only write if the legacy env had them explicitly set.
emitted_optional=0
for key in "${OPTIONAL_VARS[@]}"; do
  if [[ -n "${OLD[$key]+x}" ]]; then
    if [[ "${emitted_optional}" -eq 0 ]]; then
      {
        echo ""
        echo "# ── Preserved overrides from 1.4.x backend/.env ──────────────────────"
      } >>"${NEW_ENV}"
      emitted_optional=1
    fi
    write_kv "${key}" "${OLD[$key]}"
  fi
done

# ──────────────────────────────────────────────────────────────────────────────
# 6. Summary to stderr so the caller (Proxmox upgrade script) can log it.
# ──────────────────────────────────────────────────────────────────────────────
{
  echo ""
  echo "Wrote ${NEW_ENV}"
  echo "  PORT         = ${OLD[PORT]:-3000}"
  echo "  DATABASE_URL = $(redact_db_url "${OLD[DATABASE_URL]}")"
  echo "  REDIS_HOST   = ${OLD[REDIS_HOST]}"
  echo "  CORS_ORIGIN  = ${OLD[CORS_ORIGIN]:-}"
  [[ -n "${OLD[SESSION_SECRET]:-}" ]] && echo "  SESSION_SECRET: preserved (length=${#OLD[SESSION_SECRET]})"
} >&2
