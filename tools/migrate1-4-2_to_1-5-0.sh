#!/bin/bash
# Migration script: PatchMon 1.4.2 → 1.5.0
# Backs up current docker-compose.yml and .env, downloads the new 1.5.0
# versions, then carries forward all matching variable values from the old .env
# and from inline environment values in the old docker-compose.yml.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo ""
echo "=== PatchMon 1.4.2 → 1.5.0 Migration ==="
echo ""

# ── 1. Back up existing files ─────────────────────────────────────────────────

if [[ ! -f docker-compose.yml ]]; then
    echo "ERROR: docker-compose.yml not found in $(pwd). Are you running this from the docker directory?"
    exit 1
fi

if [[ ! -f .env ]]; then
    echo "ERROR: .env not found in $(pwd). Please ensure your .env file exists before migrating."
    exit 1
fi

echo "Bringing down the stack with 'docker compose down'..."
docker compose down

echo ""
echo "Backing up docker-compose.yml → docker-compose-1-4-2.yml"
mv docker-compose.yml docker-compose-1-4-2.yml

echo "Backing up .env → .env-1-4-2"
cp .env .env-1-4-2

# ── 2. Download new 1.5.0 files ───────────────────────────────────────────────

echo ""
echo "Downloading new docker-compose.yml (1.5.0)..."
curl -s -o docker-compose.yml \
    https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/docker/docker-compose.yml

# Strip the setup-instructions block from the top of the new compose file.
# Removes everything from "# To set up your environment..." through the closing
# "# ==...==" banner, leaving the file starting cleanly at "name: patchmon".
sed -i '/^# To set up your/,/^# =\{10,\}/d' docker-compose.yml

echo "Downloading new .env template (1.5.0)..."
curl -s -o .env \
    https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/docker/env.example

# ── 3. Extract inline values from the old docker-compose.yml ──────────────────
#
# In 1.4.2 these variables were defined inline in docker-compose.yml as YAML
# ( "KEY: value" ) rather than being sourced from .env.  We extract them here
# so they can be used as a fallback when the old .env has no value for them.
#
# Variables known to live in the old compose file:
#   REDIS_HOST, REDIS_PORT, REDIS_DB
#   POSTGRES_DB, POSTGRES_USER, POSTGRES_PASSWORD

OLD_COMPOSE="docker-compose-1-4-2.yml"

# Extract a value from "  KEY: value" YAML lines (strips leading whitespace).
extract_from_compose() {
    local key="$1"
    # Match "  KEY: value" - capture everything after ": ", trimming whitespace.
    # Ignore lines where the value is a ${...} interpolation (already in .env).
    local val
    val=$(grep -m1 "^\s\+${key}:\s" "$OLD_COMPOSE" 2>/dev/null \
          | sed "s/.*${key}:[[:space:]]*//" \
          | sed 's/[[:space:]]*$//' \
          || true)
    # Discard if the value is a variable interpolation — .env already covers it
    if [[ "$val" == \$\{* ]]; then
        val=""
    fi
    echo "$val"
}

COMPOSE_REDIS_HOST=$(extract_from_compose "REDIS_HOST")
COMPOSE_REDIS_PORT=$(extract_from_compose "REDIS_PORT")
COMPOSE_REDIS_DB=$(extract_from_compose "REDIS_DB")
COMPOSE_POSTGRES_DB=$(extract_from_compose "POSTGRES_DB")
COMPOSE_POSTGRES_USER=$(extract_from_compose "POSTGRES_USER")
COMPOSE_POSTGRES_PASSWORD=$(extract_from_compose "POSTGRES_PASSWORD")

# ── 4. Carry forward values into the new .env ─────────────────────────────────
#
# Priority order for each variable:
#   1. Old .env  (most explicit — user set it there intentionally)
#   2. Old docker-compose.yml inline value
#   3. New template default (keep as-is)

echo ""
echo "Migrating variable values from .env-1-4-2 and docker-compose-1-4-2.yml into new .env..."

OLD_ENV=".env-1-4-2"
NEW_ENV=".env"
TMP_ENV="$(mktemp)"

# ── CORS_ORIGINS → CORS_ORIGIN merge ──────────────────────────────────────────
# The old .env may have had both CORS_ORIGIN and CORS_ORIGINS (plural).
# Merge them into a single deduplicated comma-separated CORS_ORIGIN value so
# nothing is lost, then remove CORS_ORIGINS from the working copy.

OLD_ENV_WORK="$(mktemp)"
cp "$OLD_ENV" "$OLD_ENV_WORK"

_cors_singular=$(grep -m1 "^CORS_ORIGIN=" "$OLD_ENV_WORK" | cut -d= -f2- || true)
_cors_plural=$(grep -m1 "^CORS_ORIGINS=" "$OLD_ENV_WORK" | cut -d= -f2- || true)

if [[ -n "$_cors_plural" ]]; then
    # Combine both, split on commas, deduplicate, rejoin
    _combined=$(printf '%s,%s' "$_cors_singular" "$_cors_plural" \
        | tr ',' '\n' | sed '/^$/d' | sort -u | tr '\n' ',' | sed 's/,$//')
    # Replace (or insert) CORS_ORIGIN in the working copy and drop CORS_ORIGINS
    if grep -q "^CORS_ORIGIN=" "$OLD_ENV_WORK"; then
        sed -i "s|^CORS_ORIGIN=.*|CORS_ORIGIN=${_combined}|" "$OLD_ENV_WORK"
    else
        echo "CORS_ORIGIN=${_combined}" >> "$OLD_ENV_WORK"
    fi
    sed -i '/^CORS_ORIGINS=/d' "$OLD_ENV_WORK"
    echo "  [cors merge] CORS_ORIGIN=${_combined}"
fi

OLD_ENV="$OLD_ENV_WORK"

# Map of variable name → value extracted from old compose
declare -A COMPOSE_VALS
COMPOSE_VALS["REDIS_HOST"]="$COMPOSE_REDIS_HOST"
COMPOSE_VALS["REDIS_PORT"]="$COMPOSE_REDIS_PORT"
COMPOSE_VALS["REDIS_DB"]="$COMPOSE_REDIS_DB"
COMPOSE_VALS["POSTGRES_DB"]="$COMPOSE_POSTGRES_DB"
COMPOSE_VALS["POSTGRES_USER"]="$COMPOSE_POSTGRES_USER"
COMPOSE_VALS["POSTGRES_PASSWORD"]="$COMPOSE_POSTGRES_PASSWORD"

while IFS= read -r line; do
    # Pure blank lines pass through unchanged
    if [[ -z "$line" ]]; then
        echo "$line" >> "$TMP_ENV"
        continue
    fi

    # Comment lines: check if the old .env had this variable *uncommented* and
    # active. If so, promote the old value (uncommented) into the new .env.
    # This handles optional vars like TRUST_PROXY, OIDC_*, TZ, LOG_LEVEL, etc.
    # that are commented out in the new template but may have been set by the user.
    # Only match "# VAR=" style (space after #), not "#VAR=" inline examples.
    if [[ "$line" =~ ^[[:space:]]*#[[:space:]]+([A-Z_][A-Z0-9_]*)= ]]; then
        commented_var="${BASH_REMATCH[1]}"
        old_active=$(grep -m1 "^${commented_var}=" "$OLD_ENV" 2>/dev/null || true)
        if [[ -n "$old_active" ]]; then
            # User had this set — write it uncommented so it stays active
            echo "$old_active" >> "$TMP_ENV"
            continue
        fi
        # Not set in old .env — keep the comment line as-is
        echo "$line" >> "$TMP_ENV"
        continue
    fi

    # Plain comment / section header lines
    if [[ "$line" =~ ^[[:space:]]*# ]]; then
        echo "$line" >> "$TMP_ENV"
        continue
    fi

    # Active variable line — extract name
    var_name="${line%%=*}"

    # 1. Try the old .env first
    old_env_value=$(grep -m1 "^${var_name}=" "$OLD_ENV" 2>/dev/null || true)

    if [[ -n "$old_env_value" ]]; then
        echo "$old_env_value" >> "$TMP_ENV"
    # 2. Fall back to value extracted from old docker-compose.yml
    elif [[ -n "${COMPOSE_VALS[$var_name]:-}" ]]; then
        echo "${var_name}=${COMPOSE_VALS[$var_name]}" >> "$TMP_ENV"
        echo "  [compose fallback] ${var_name}=${COMPOSE_VALS[$var_name]}"
    else
        # 3. Keep the new template line as-is
        echo "$line" >> "$TMP_ENV"
    fi
done < "$NEW_ENV"

mv "$TMP_ENV" "$NEW_ENV"
rm -f "$OLD_ENV_WORK"

# ── 5. Carry forward the frontend host port into the new docker-compose.yml ───
#
# In 1.4.2 the exposed port was defined on the "frontend" service:
#
#   frontend:
#     ports:
#       - "3000:3000"   ← host:container
#
# In 1.5.0 it is defined on the "server" service.  If the user had changed the
# host-side port (e.g. "8080:3000") we must reflect that in the new compose file
# and also set PORT= in the new .env so the container internal port matches.

echo ""
echo "Checking for custom frontend port in docker-compose-1-4-2.yml..."

# Extract the host port from the first "- \"<host>:3000\"" line under the
# frontend service.  We look for a ports entry of the form "HOST:CONTAINER" or
# just "PORT" (bare).  The container-side port in 1.4.2 was always 3000.
OLD_FRONTEND_HOST_PORT=$(
    awk '
        /^[[:space:]]+frontend:/ { in_frontend=1 }
        in_frontend && /^[[:space:]]+ports:/ { in_ports=1; next }
        in_frontend && in_ports && /^[[:space:]]+-[[:space:]]+["'"'"']?[0-9]+:[0-9]+["'"'"']?/ {
            # Extract host port from "HOST:CONTAINER"
            match($0, /[0-9]+:[0-9]+/)
            pair=substr($0, RSTART, RLENGTH)
            split(pair, a, ":")
            print a[1]
            exit
        }
        in_frontend && in_ports && /^[[:space:]]+-[[:space:]]+["'"'"']?[0-9]+["'"'"']?[[:space:]]*$/ {
            # Bare port — host and container are the same
            match($0, /[0-9]+/)
            print substr($0, RSTART, RLENGTH)
            exit
        }
        # A new top-level service key ends the frontend block
        in_frontend && /^[a-zA-Z]/ { in_frontend=0; in_ports=0 }
    ' "$OLD_COMPOSE"
)

if [[ -z "$OLD_FRONTEND_HOST_PORT" ]]; then
    echo "  No frontend ports entry found — no port migration needed."
elif [[ "$OLD_FRONTEND_HOST_PORT" == "3000" ]]; then
    echo "  Frontend host port was 3000 (default) — no change needed."
else
    echo "  Custom frontend host port detected: ${OLD_FRONTEND_HOST_PORT}"

    # Patch the server.ports line in the new compose file:
    #   "3000:3000"  →  "<custom>:3000"
    # Uses awk to scope the replacement strictly within the "server:" service
    # block, so no other service's ports line is ever touched.
    awk -v port="${OLD_FRONTEND_HOST_PORT}" '
        /^  server:$/         { in_server=1 }
        in_server && /^  [a-z]/ && !/^  server:$/ { in_server=0 }
        in_server             { sub(/"3000:3000"/, "\"" port ":3000\"") }
        { print }
    ' docker-compose.yml > docker-compose.yml.tmp && mv docker-compose.yml.tmp docker-compose.yml
    echo "  Updated docker-compose.yml server ports: \"${OLD_FRONTEND_HOST_PORT}:3000\""

    # Also write PORT= into the new .env so the Go server binds to the right port
    # inside the container (the container-side port stays 3000 in this model, but
    # set it explicitly so it is visible to the user).
    if grep -q "^#\?[[:space:]]*PORT=" "$NEW_ENV"; then
        sed -i "s|^#\?[[:space:]]*PORT=.*|PORT=3000|" "$NEW_ENV"
    else
        echo "PORT=3000" >> "$NEW_ENV"
    fi
    echo "  Ensured PORT=3000 is set in .env (container-internal port)."
    echo ""
    echo "  NOTE: The server container now listens internally on port 3000 and is"
    echo "  exposed to your host on port ${OLD_FRONTEND_HOST_PORT}."
    echo "  Update CORS_ORIGIN in .env to use port ${OLD_FRONTEND_HOST_PORT} if it"
    echo "  references the old port."
fi

# ── 6. Done ───────────────────────────────────────────────────────────────────

echo ""
echo "Migration complete."
echo ""
echo "  docker-compose-1-4-2.yml  ← your old compose file (backup)"
echo "  .env-1-4-2                ← your old .env (backup)"
echo "  docker-compose.yml        ← new 1.5.0 compose file"
echo "  .env                      ← new 1.5.0 .env (values carried over)"
echo ""
echo "Please review the new .env and docker-compose.yml before starting your stack."
echo ""
echo "─────────────────────────────────────────────────────────────────────────────"
echo "CLEANUP REMINDER"
echo "─────────────────────────────────────────────────────────────────────────────"
echo ""
echo "Once you have confirmed everything is working correctly on 1.5.0, you can"
echo "safely remove the following Docker volumes that are no longer needed:"
echo ""
echo "  • patchmon_agent_files"
echo "  • patchmon_branding_assets"
echo ""
echo "To remove them run:"
echo ""
echo "  docker volume rm patchmon_agent_files patchmon_branding_assets"
echo ""
echo "Do NOT do this until you are confident the new stack is running as expected."
echo ""
