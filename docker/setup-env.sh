#!/usr/bin/env bash
# =============================================================================
# PatchMon Docker - Environment Setup Script
# =============================================================================
# 1. Copies ../server/.env.example (or ./env.example) to .env
# 2. Generates and injects JWT_SECRET, SESSION_SECRET, AI_ENCRYPTION_KEY (64 hex)
# 3. Generates and injects POSTGRES_PASSWORD, REDIS_PASSWORD (32 hex)
#
# Run from the docker directory: ./setup-env.sh
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Resolve source: ../server/.env.example, then ../server-source-code/.env.example, then ./env.example, else curl from upstream
SOURCE=""
if [ -f "../server/.env.example" ]; then
  SOURCE="../server/.env.example"
elif [ -f "../server-source-code/.env.example" ]; then
  SOURCE="../server-source-code/.env.example"
elif [ -f "./env.example" ]; then
  SOURCE="./env.example"
else
  echo "No local env example found. Downloading from upstream..."
  SOURCE=$(mktemp)
  if ! curl -fsSL -o "$SOURCE" "https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/docker/env.example"; then
    echo "Error: Failed to download env example." >&2
    rm -f "$SOURCE"
    exit 1
  fi
  trap "rm -f $SOURCE" EXIT
fi

echo "Copying $SOURCE to .env"
cp "$SOURCE" .env

# Generate secrets: one 64-hex for JWT/SESSION/AI, one 32-hex for both passwords
HEX64=$(openssl rand -hex 64)
HEX32=$(openssl rand -hex 32)

# Inject 32-char secrets (same value for POSTGRES_PASSWORD and REDIS_PASSWORD)
sed -i "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$HEX32/" .env
sed -i "s/^REDIS_PASSWORD=.*/REDIS_PASSWORD=$HEX32/" .env

# Inject 64-char secrets (same value for JWT_SECRET, SESSION_SECRET, AI_ENCRYPTION_KEY)
sed -i "s/^JWT_SECRET=.*/JWT_SECRET=$HEX64/" .env
sed -i "s/^AI_ENCRYPTION_KEY=.*/AI_ENCRYPTION_KEY=$HEX64/" .env
sed -i "s/^SESSION_SECRET=.*/SESSION_SECRET=$HEX64/" .env

echo "Done. .env created with generated secrets."
echo ""

# -----------------------------------------------------------------------------
# Interactive CORS_ORIGIN builder (skip if not a TTY, e.g. CI)
# -----------------------------------------------------------------------------
if [ -t 0 ]; then
echo "=== CORS Origin Configuration ==="
echo "PatchMon runs on port 3000 by default. If using a reverse proxy or different host, enter the full URL you will use to access it."
echo ""

# Reverse proxy / TRUST_PROXY
read -r -p "Will you be accessing PatchMon via a reverse proxy (nginx, Caddy, etc.)? (y/n) [n]: " use_proxy
use_proxy=${use_proxy:-n}
if [ "$use_proxy" = "y" ] || [ "$use_proxy" = "Y" ]; then
  TRUST_PROXY_VALUE="true"
  echo "TRUST_PROXY will be set to true (server will trust X-Forwarded-* headers)."
else
  TRUST_PROXY_VALUE="false"
fi
export TRUST_PROXY_VALUE
# Uncomment or replace existing TRUST_PROXY line (matches both "# TRUST_PROXY=..." and "TRUST_PROXY=...")
perl -i -pe 's/^#?\s*TRUST_PROXY=.*/TRUST_PROXY=$ENV{TRUST_PROXY_VALUE}/' .env
if ! grep -q "^TRUST_PROXY=" .env; then
  echo "TRUST_PROXY=$TRUST_PROXY_VALUE" >> .env
fi

echo ""

# Timezone (TZ)
read -r -p "Do you want to change the timezone from UTC? (y/n) [n]: " change_tz
change_tz=${change_tz:-n}
if [ "$change_tz" = "y" ] || [ "$change_tz" = "Y" ]; then
  echo "Examples: Europe/London, America/New_York, America/Los_Angeles, Asia/Tokyo, Australia/Sydney"
  while true; do
    read -r -p "Enter timezone (IANA format, e.g. Europe/London) [UTC]: " tz_input
    tz_input=$(echo "${tz_input:-UTC}" | tr -d ' ')
    if [ -z "$tz_input" ]; then
      tz_input="UTC"
    fi
    # Validate: check zoneinfo path (Linux/macOS) or use zdump
    if [ -f "/usr/share/zoneinfo/$tz_input" ]; then
      TZ_VALUE="$tz_input"
      break
    elif command -v zdump >/dev/null 2>&1 && zdump "$tz_input" >/dev/null 2>&1; then
      TZ_VALUE="$tz_input"
      break
    elif [ "$tz_input" = "UTC" ] || [ "$tz_input" = "Etc/UTC" ] || [ "$tz_input" = "GMT" ] || [ "$tz_input" = "Etc/GMT" ]; then
      TZ_VALUE="$tz_input"
      break
    else
      echo "Invalid timezone: $tz_input. Use IANA format (e.g. America/New_York). Try 'timedatectl list-timezones' for a full list."
    fi
  done
  export TZ_VALUE
  perl -i -pe 's/^#?\s*TZ=.*/TZ=$ENV{TZ_VALUE}/' .env
  if ! grep -q "^TZ=" .env; then
    echo "TZ=$TZ_VALUE" >> .env
  fi
  echo "TZ will be set to: $TZ_VALUE"
else
  echo "Keeping default timezone (UTC)."
fi

echo ""

# Read existing CORS_ORIGIN from .env if present
current_cors=$(grep -E "^CORS_ORIGIN=" .env 2>/dev/null | cut -d= -f2- | tr -d '"' || true)
cors_origins=()
if [ -n "$current_cors" ]; then
  IFS=',' read -ra cors_origins <<< "$current_cors"
fi

# Default if empty
if [ ${#cors_origins[@]} -eq 0 ]; then
  cors_origins=("http://localhost:3000")
fi

# Main URL prompt
read -r -p "What URL will you use to access PatchMon? [http://localhost:3000]: " input_url
if [ -n "$input_url" ]; then
  cors_origins=("$input_url")
fi

# Add/remove loop
while true; do
  echo ""
  echo "Current CORS origins:"
  for i in "${!cors_origins[@]}"; do
    echo "  $((i + 1)). ${cors_origins[$i]}"
  done
  echo ""
  read -r -p "Add (a), Remove (r), or Done (d) [d]: " action
  action=${action:-d}
  case "$action" in
    a|A)
      read -r -p "Enter URL to add: " new_url
      if [ -n "$new_url" ]; then
        cors_origins+=("$new_url")
      fi
      ;;
    r|R)
      if [ ${#cors_origins[@]} -eq 0 ]; then
        echo "No origins to remove."
      else
        read -r -p "Enter number to remove (1-${#cors_origins[@]}): " idx
        if [[ "$idx" =~ ^[0-9]+$ ]] && [ "$idx" -ge 1 ] && [ "$idx" -le ${#cors_origins[@]} ]; then
          unset 'cors_origins[idx-1]'
          cors_origins=("${cors_origins[@]}")
        else
          echo "Invalid number."
        fi
      fi
      ;;
    d|D)
      break
      ;;
    *)
      echo "Invalid choice. Use a, r, or d."
      ;;
  esac
done

# Build final value
if [ ${#cors_origins[@]} -gt 0 ]; then
  CORS_VALUE=$(IFS=','; echo "${cors_origins[*]}")
  echo ""
  echo "CORS_ORIGIN will be set to: $CORS_VALUE"
  # Update .env (use perl to avoid sed escaping issues with URLs)
  export CORS_VALUE
  perl -i -pe 's/^CORS_ORIGIN=.*/CORS_ORIGIN=$ENV{CORS_VALUE}/' .env
  # Ensure the line exists if it was missing
  if ! grep -q "^CORS_ORIGIN=" .env; then
    echo "CORS_ORIGIN=$CORS_VALUE" >> .env
  fi
fi

echo ""
echo "Setup complete. Edit .env to configure PORT if needed (default: 3000)."
else
  echo "Non-interactive mode: skipping CORS_ORIGIN prompt. Edit .env to set CORS_ORIGIN and PORT if needed."
fi
