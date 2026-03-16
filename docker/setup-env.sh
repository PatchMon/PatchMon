#!/usr/bin/env bash
# =============================================================================
# PatchMon Docker - Environment Setup Script
# =============================================================================
# Downloads docker-compose.yml and env.example if not already present,
# then:
# 1. Copies env.example to .env
# 2. Generates and injects POSTGRES_PASSWORD, REDIS_PASSWORD (32 hex)
# 3. Generates and injects JWT_SECRET, SESSION_SECRET, AI_ENCRYPTION_KEY (64 hex)
# 4. Interactively configures CORS_ORIGIN, TRUST_PROXY, and TZ
#
# Run from any directory:
#   curl -fsSL https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/docker/setup-env.sh | bash
# Or if already downloaded:
#   ./setup-env.sh
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

UPSTREAM="https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/docker"

# -----------------------------------------------------------------------------
# Download docker-compose.yml if not present
# -----------------------------------------------------------------------------
if [ ! -f "./docker-compose.yml" ]; then
  echo "docker-compose.yml not found. Downloading from upstream..."
  if ! curl -fsSL -o docker-compose.yml "$UPSTREAM/docker-compose.yml"; then
    echo "Error: Failed to download docker-compose.yml." >&2
    exit 1
  fi
  echo "docker-compose.yml downloaded."
fi

# -----------------------------------------------------------------------------
# Ensure env.example exists locally, or download if missing
# -----------------------------------------------------------------------------
if [ ! -f "./env.example" ]; then
  echo "env.example not found. Downloading from upstream..."
  if ! curl -fsSL -o env.example "$UPSTREAM/env.example"; then
    echo "Error: Failed to download env.example." >&2
    exit 1
  fi
  echo "env.example downloaded."
fi

echo "Copying env.example to .env"
cp env.example .env

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
echo "============================================================"
echo " Setup complete!"
echo "============================================================"
echo ""
echo "NOTE: Before starting for the first time, ensure that:"
echo "  - DNS records are configured to point your domain(s) to this host"
echo "  - If using a reverse proxy (nginx, Caddy, Traefik, etc.), configure"
echo "    it to forward traffic to this host on port 3000"
echo ""
echo "Access your PatchMon server using the following URL(s):"
if [ -n "${CORS_VALUE:-}" ]; then
  IFS=',' read -ra _display_origins <<< "$CORS_VALUE"
  for _url in "${_display_origins[@]}"; do
    echo "  -> $_url"
  done
else
  echo "  (no CORS_ORIGIN configured — edit .env before starting)"
fi
echo ""
echo "Start PatchMon with:"
echo ""
echo "  docker compose up -d"
echo ""
echo "Edit .env to configure PORT if needed (default: 3000)."
else
  echo "Non-interactive mode: skipping CORS_ORIGIN configuration. Edit .env to set CORS_ORIGIN and PORT if needed."
  echo ""
  echo "NOTE: Before starting for the first time, ensure DNS and any reverse proxy"
  echo "are configured to point to this host before running:"
  echo ""
  echo "  docker compose up -d"
fi
