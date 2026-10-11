#!/usr/bin/env bash
# Creates or updates .env for the PatchMon Docker stack. Must keep working as:
#   bash -c "$(curl -fsSL https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/docker/setup-env.sh)"
#   ./setup-env.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-.}")" && pwd)"
cd "$SCRIPT_DIR"

UPSTREAM="${UPSTREAM:-https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main}"

if [ -t 0 ]; then
  PROMPT_FD=0
elif (: </dev/tty) 2>/dev/null; then
  exec 3</dev/tty
  PROMPT_FD=3
else
  PROMPT_FD=""
fi

ask() {
  if ! read -r -u "$PROMPT_FD" -p "$1" "${2?}"; then
    echo ""
    printf -v "$2" '%s' ''
  fi
}

env_get() {
  grep -E "^$1=" .env 2>/dev/null | head -n1 | cut -d= -f2- | sed -E "s/^\"(.*)\"$/\1/; s/^'(.*)'$/\1/" || true
}

env_set() {
  local key="$1" value="$2" pattern tmp
  if grep -qE "^${key}=" .env; then
    pattern="^${key}="
  else
    pattern="^#[[:space:]]*${key}="
  fi
  tmp="$(mktemp)"
  _K="$key" _V="$value" _P="$pattern" awk '
    !done && $0 ~ ENVIRON["_P"] { print ENVIRON["_K"] "=" ENVIRON["_V"]; done = 1; next }
    { print }
    END { if (!done) print ENVIRON["_K"] "=" ENVIRON["_V"] }
  ' .env > "$tmp"
  cat "$tmp" > .env
  rm -f "$tmp"
}

ensure_secret() {
  local key="$1" bytes="$2"
  if [ -z "$(env_get "$key")" ]; then
    env_set "$key" "$(openssl rand -hex "$bytes")"
    echo "  generated $key"
  fi
}

env_append() {
  [ -z "$(tail -c1 .env)" ] || echo >> .env
  printf '%s\n' "$1" >> .env
}

user_commented() {
  local key="$1" l
  while IFS= read -r l; do
    grep -Fxq -- "$l" env.example || return 0
  done < <(grep -E "^#[[:space:]]*${key}=" .env || true)
  return 1
}

append_missing_keys() {
  local line key
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      [[:upper:]_]*=*) key="${line%%=*}" ;;
      *) continue ;;
    esac
    if ! grep -qE "^${key}=" .env && ! user_commented "$key"; then
      env_append "$line"
      echo "  added $key from env.example"
    fi
  done < env.example
}

if [ ! -f "./docker-compose.yml" ]; then
  echo "docker-compose.yml not found. Downloading from upstream..."
  if ! curl -fsSL -o docker-compose.yml "$UPSTREAM/docker/docker-compose.yml"; then
    echo "Error: Failed to download docker-compose.yml." >&2
    exit 1
  fi
  echo "docker-compose.yml downloaded."
fi

if [ ! -f "./env.example" ]; then
  echo "env.example not found. Downloading from upstream..."
  if ! curl -fsSL -o env.example "$UPSTREAM/docker/env.example"; then
    echo "Error: Failed to download env.example." >&2
    exit 1
  fi
  echo "env.example downloaded."
fi

if [ -f .env ]; then
  echo "Updating existing .env, secrets preserved"
  append_missing_keys
else
  echo "Creating .env from env.example"
  cp env.example .env
  chmod 600 .env
fi

ensure_secret POSTGRES_PASSWORD 32
ensure_secret REDIS_PASSWORD 32
ensure_secret JWT_SECRET 64
ensure_secret SESSION_SECRET 64
ensure_secret AI_ENCRYPTION_KEY 64

echo "Done. .env is ready."
echo ""

if [ -n "$PROMPT_FD" ]; then
echo "=== CORS Origin Configuration ==="
echo "PatchMon runs on port 3000 by default. If using a reverse proxy or different host, enter the full URL you will use to access it."
echo ""

current_proxy=$(env_get TRUST_PROXY)
default_proxy=n
if [ "$current_proxy" = "true" ]; then
  default_proxy=y
fi
ask "Will you be accessing PatchMon via a reverse proxy (nginx, Caddy, etc.)? (y/n) [$default_proxy]: " use_proxy
use_proxy=${use_proxy:-$default_proxy}
if [ "$use_proxy" = "y" ] || [ "$use_proxy" = "Y" ]; then
  TRUST_PROXY_VALUE="true"
  echo "TRUST_PROXY will be set to true (server will trust X-Forwarded-* headers)."
else
  TRUST_PROXY_VALUE="false"
fi
env_set TRUST_PROXY "$TRUST_PROXY_VALUE"

echo ""

current_tz=$(env_get TZ)
current_tz=${current_tz:-UTC}
ask "Do you want to change the timezone from $current_tz? (y/n) [n]: " change_tz
change_tz=${change_tz:-n}
if [ "$change_tz" = "y" ] || [ "$change_tz" = "Y" ]; then
  echo "Examples: Europe/London, America/New_York, America/Los_Angeles, Asia/Tokyo, Australia/Sydney"
  while true; do
    ask "Enter timezone (IANA format, e.g. Europe/London) [$current_tz]: " tz_input
    tz_input=$(echo "${tz_input:-$current_tz}" | tr -d ' ')
    if [ -z "$tz_input" ]; then
      tz_input="UTC"
    fi
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
  env_set TZ "$TZ_VALUE"
  echo "TZ will be set to: $TZ_VALUE"
else
  echo "Keeping timezone: $current_tz."
fi

echo ""

current_cors=$(env_get CORS_ORIGIN)
cors_origins=()
if [ -n "$current_cors" ]; then
  IFS=',' read -ra cors_origins <<< "$current_cors"
fi

if [ ${#cors_origins[@]} -eq 0 ]; then
  cors_origins=("http://localhost:3000")
fi

echo "PatchMon runs on port 3000 by default. Include the port in your URL unless you are"
echo "terminating it at a reverse proxy on a standard port (e.g. https://patchmon.example.com)."
echo "Examples:  http://192.168.1.10:3000   https://patchmon.local:3000   https://patchmon.example.com"
echo ""
ask "What URL will you use to access PatchMon? [${cors_origins[0]}]: " input_url
if [ -n "$input_url" ]; then
  cors_origins=("$input_url")
fi

while true; do
  echo ""
  echo "Current CORS origins:"
  for i in "${!cors_origins[@]}"; do
    echo "  $((i + 1)). ${cors_origins[$i]}"
  done
  echo ""
  ask "Add (a), Remove (r), or Done (d) [d]: " action
  action=${action:-d}
  case "$action" in
    a|A)
      ask "Enter URL to add: " new_url
      if [ -n "$new_url" ]; then
        cors_origins+=("$new_url")
      fi
      ;;
    r|R)
      if [ ${#cors_origins[@]} -eq 0 ]; then
        echo "No origins to remove."
      else
        idx=""
        ask "Enter number to remove (1-${#cors_origins[@]}): " idx
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

if [ ${#cors_origins[@]} -gt 0 ]; then
  CORS_VALUE=$(IFS=','; echo "${cors_origins[*]}")
  echo ""
  echo "CORS_ORIGIN will be set to: $CORS_VALUE"
  env_set CORS_ORIGIN "$CORS_VALUE"
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
  echo "  (no CORS_ORIGIN configured, edit .env before starting)"
fi
echo ""
echo "Start PatchMon with:"
echo ""
echo "  docker compose up -d"
echo ""
echo "Edit .env to configure PORT if needed (default: 3000)."
else
  echo "No terminal available, so the interactive prompts were skipped."
  echo "CORS_ORIGIN was left at: $(env_get CORS_ORIGIN)"
  echo "Set it to the URL you will use to access PatchMon by editing .env, or run this script from a terminal."
  echo ""
  echo "NOTE: Before starting for the first time, ensure DNS and any reverse proxy"
  echo "are configured to point to this host before running:"
  echo ""
  echo "  docker compose up -d"
fi
