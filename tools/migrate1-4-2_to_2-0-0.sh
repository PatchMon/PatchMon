#!/bin/bash
# Migrates a PatchMon 1.4.2 Docker stack to the 2.0.0 compose layout.
# Usage: migrate1-4-2_to_2-0-0.sh [compose-directory]   (defaults to the current directory)

set -Eeuo pipefail

UPSTREAM="${UPSTREAM:-https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main}"
TARGET_DIR="${1:-$PWD}"
BACKUP_DIR=""
STACK_DOWN=""

restore_hint() {
  if [[ -n "$STACK_DOWN" && -n "$BACKUP_DIR" ]]; then
    echo "" >&2
    echo "The stack was already stopped and the files may be half replaced. Your originals are in $PWD/$BACKUP_DIR/" >&2
    if [[ -f "$BACKUP_DIR/.env" ]]; then
      echo "Restore with: cp $BACKUP_DIR/docker-compose.yml docker-compose.yml && cp $BACKUP_DIR/.env .env && docker compose up -d" >&2
    else
      echo "Restore with: cp $BACKUP_DIR/docker-compose.yml docker-compose.yml && rm -f .env && docker compose up -d" >&2
    fi
  fi
}

on_error() {
  local rc=$?
  echo "ERROR: migration aborted (exit $rc)." >&2
  restore_hint
  exit "$rc"
}
trap on_error ERR

fail() { echo "ERROR: $*" >&2; restore_hint; exit 1; }

strip_quotes() { sed -E "s/^[[:space:]]+//; s/[[:space:]]+$//; s/^\"(.*)\"$/\1/; s/^'(.*)'$/\1/"; }

env_set() {
  local file="$1" key="$2" value="$3" pattern tmp
  if grep -qE "^${key}=" "$file"; then
    pattern="^${key}="
  else
    pattern="^#[[:space:]]*${key}="
  fi
  tmp="$(mktemp)"
  _K="$key" _V="$value" _P="$pattern" awk '
    !done && $0 ~ ENVIRON["_P"] { print ENVIRON["_K"] "=" ENVIRON["_V"]; done = 1; next }
    { print }
    END { if (!done) print ENVIRON["_K"] "=" ENVIRON["_V"] }
  ' "$file" > "$tmp"
  cat "$tmp" > "$file"
  rm -f "$tmp"
}

resolve_project_name() {
  local name="${COMPOSE_PROJECT_NAME:-}"
  [[ -n "$name" || ! -f .env ]] || name=$(grep -E '^COMPOSE_PROJECT_NAME=' .env | head -n1 | cut -d= -f2- | strip_quotes || true)
  [[ -n "$name" ]] || name=$(sed -nE 's/^name:[[:space:]]*//p' docker-compose.yml | head -n1 | strip_quotes || true)
  [[ -n "$name" ]] || name=$(docker compose config 2>/dev/null | sed -nE 's/^name:[[:space:]]*//p' | head -n1 | strip_quotes || true)
  [[ -n "$name" ]] || name=$(basename "$PWD" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9_-]//g; s/^[-_]*//')
  echo "$name"
}

[ -d "$TARGET_DIR" ] || fail "$TARGET_DIR is not a directory."
cd "$TARGET_DIR"

echo ""
echo "=== PatchMon 1.4.2 to 2.0.0 Migration ==="
echo ""
echo "Compose directory: $PWD"

[ -f docker-compose.yml ] || fail "docker-compose.yml not found in $PWD. Run this from your PatchMon compose directory or pass it as the first argument."
command -v docker >/dev/null 2>&1 || fail "docker is not installed or not on PATH."
docker compose version >/dev/null 2>&1 || fail "The docker compose plugin is required."

if ! grep -qE '^[[:space:]]+frontend:' docker-compose.yml && grep -qE '^[[:space:]]+server:' docker-compose.yml; then
  fail "docker-compose.yml already uses the 2.0.0 layout (server service, no frontend service). Nothing to migrate."
fi

OLD_PROJECT="$(resolve_project_name)"
echo "Compose project name: $OLD_PROJECT"

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

echo ""
echo "Downloading the 2.0.0 docker-compose.yml and env template..."
curl -fsSL -o "$STAGE/docker-compose.yml" "$UPSTREAM/docker/docker-compose.yml" \
  || fail "Download of docker-compose.yml failed. Nothing has been changed."
curl -fsSL -o "$STAGE/env.example" "$UPSTREAM/docker/env.example" \
  || fail "Download of env.example failed. Nothing has been changed."
grep -q '^services:' "$STAGE/docker-compose.yml" \
  || fail "Downloaded docker-compose.yml does not look like a compose file. Nothing has been changed."
grep -q '^POSTGRES_PASSWORD=' "$STAGE/env.example" \
  || fail "Downloaded env.example does not look like the PatchMon template. Nothing has been changed."
echo "Downloads verified."

BACKUP_DIR="backup-1-4-2-$(date +%Y%m%d-%H%M%S)"
if [[ -e "$BACKUP_DIR" ]]; then
  fail "$BACKUP_DIR already exists. Wait a moment and run again."
fi
mkdir "$BACKUP_DIR"
cp -p docker-compose.yml "$BACKUP_DIR/docker-compose.yml"
OLD_COMPOSE="$BACKUP_DIR/docker-compose.yml"
if [[ -f .env ]]; then
  cp -p .env "$BACKUP_DIR/.env"
  OLD_ENV="$BACKUP_DIR/.env"
  echo "Backed up docker-compose.yml and .env to $BACKUP_DIR/"
else
  OLD_ENV="$STAGE/empty.env"
  : > "$OLD_ENV"
  echo "Backed up docker-compose.yml to $BACKUP_DIR/ (no .env found; settings will be read from docker-compose.yml)"
fi
NEW_ENV=".env"

echo ""
echo "Stopping the stack (docker compose down)..."
docker compose down
STACK_DOWN="1"

sed '/^# To set up your/,/^# =\{10,\}/d' "$STAGE/docker-compose.yml" > docker-compose.yml
cp "$STAGE/env.example" "$NEW_ENV"
echo "Installed the 2.0.0 docker-compose.yml and .env template."

extract_from_compose() {
  local key="$1" val
  val=$(grep -m1 -E "^[[:space:]]+(-[[:space:]]+)?${key}(:[[:space:]]|=)" "$OLD_COMPOSE" 2>/dev/null \
    | sed -E "s/^[[:space:]]+(-[[:space:]]+)?${key}(:[[:space:]]*|=)//" \
    | sed -E "/^[\"']/! s/[[:space:]]+#.*$//" \
    | strip_quotes \
    || true)
  if [[ "$val" == *\$\{* ]]; then
    val=""
  fi
  echo "$val"
}

extract_requirepass() {
  grep -m1 -E '^[[:space:]]+command:.*--requirepass[[:space:]]' "$OLD_COMPOSE" 2>/dev/null \
    | sed -E "s/.*--requirepass[[:space:]]+(\"([^\"]*)\"|'([^']*)'|([^[:space:]]+)).*/\2\3\4/" \
    || true
}

compose_fallback() {
  local val
  [[ "$1" == "DATABASE_URL" ]] && return 0
  val=$(extract_from_compose "$1")
  if [[ -z "$val" && "$1" == "REDIS_PASSWORD" ]]; then
    val=$(extract_requirepass)
    [[ "$val" == *\$* ]] && val=""
  fi
  echo "$val"
}

display_value() {
  if [[ "$1" =~ (PASSWORD|SECRET|_KEY|TOKEN) ]]; then
    echo "<hidden>"
  else
    echo "$2"
  fi
}

echo ""
echo "Migrating variable values from the backed-up .env and docker-compose.yml into the new .env..."

OLD_ENV_WORK="$STAGE/old.env"
_cors_singular=$(grep -m1 "^CORS_ORIGIN=" "$OLD_ENV" | cut -d= -f2- || true)
_cors_plural=$(grep -m1 "^CORS_ORIGINS=" "$OLD_ENV" | cut -d= -f2- || true)

if [[ -n "$_cors_plural" ]]; then
  _combined=$(printf '%s,%s' "$_cors_singular" "$_cors_plural" \
    | tr ',' '\n' | sed '/^$/d' | sort -u | tr '\n' ',' | sed 's/,$//')
  { grep -vE '^CORS_ORIGINS?=' "$OLD_ENV" || true; echo "CORS_ORIGIN=${_combined}"; } > "$OLD_ENV_WORK"
  echo "  [cors merge] CORS_ORIGIN=${_combined}"
else
  cp "$OLD_ENV" "$OLD_ENV_WORK"
fi
OLD_ENV="$OLD_ENV_WORK"

TMP_ENV="$STAGE/new.env"
: > "$TMP_ENV"

while IFS= read -r line || [[ -n "$line" ]]; do
  if [[ -z "$line" ]]; then
    echo "$line" >> "$TMP_ENV"
    continue
  fi

  if [[ "$line" =~ ^[[:space:]]*#[[:space:]]+([A-Z_][A-Z0-9_]*)= ]]; then
    commented_var="${BASH_REMATCH[1]}"
    old_active=$(grep -m1 "^${commented_var}=" "$OLD_ENV" 2>/dev/null || true)
    if [[ -n "$old_active" ]]; then
      echo "$old_active" >> "$TMP_ENV"
      continue
    fi
    fallback_value=$(compose_fallback "$commented_var")
    if [[ -n "$fallback_value" ]]; then
      echo "${commented_var}=${fallback_value}" >> "$TMP_ENV"
      echo "  [compose fallback] ${commented_var}=$(display_value "$commented_var" "$fallback_value")"
      continue
    fi
    echo "$line" >> "$TMP_ENV"
    continue
  fi

  if [[ "$line" =~ ^[[:space:]]*# ]]; then
    echo "$line" >> "$TMP_ENV"
    continue
  fi

  var_name="${line%%=*}"
  old_env_value=$(grep -m1 "^${var_name}=" "$OLD_ENV" 2>/dev/null || true)
  fallback_value=$(compose_fallback "$var_name")

  if [[ -n "${old_env_value#*=}" ]]; then
    echo "$old_env_value" >> "$TMP_ENV"
  elif [[ -n "$fallback_value" ]]; then
    echo "${var_name}=${fallback_value}" >> "$TMP_ENV"
    echo "  [compose fallback] ${var_name}=$(display_value "$var_name" "$fallback_value")"
  else
    echo "$line" >> "$TMP_ENV"
  fi
done < "$NEW_ENV"

cat "$TMP_ENV" > "$NEW_ENV"

if [[ "$OLD_PROJECT" != "patchmon" ]]; then
  env_set "$NEW_ENV" COMPOSE_PROJECT_NAME "$OLD_PROJECT"
  echo "  [project] COMPOSE_PROJECT_NAME=${OLD_PROJECT} written so the existing data volumes are reused"
fi

echo ""
echo "Checking for a custom frontend port in the old docker-compose.yml..."

OLD_FRONTEND_HOST_PORT=$(
  awk '
    /^[[:space:]]+frontend:/ { in_frontend=1 }
    in_frontend && /^[[:space:]]+ports:/ { in_ports=1; next }
    in_frontend && in_ports && /^[[:space:]]+-[[:space:]]+["'"'"']?[0-9]+:[0-9]+["'"'"']?/ {
      match($0, /[0-9]+:[0-9]+/)
      pair=substr($0, RSTART, RLENGTH)
      split(pair, a, ":")
      print a[1]
      exit
    }
    in_frontend && in_ports && /^[[:space:]]+-[[:space:]]+["'"'"']?[0-9]+["'"'"']?[[:space:]]*$/ {
      match($0, /[0-9]+/)
      print substr($0, RSTART, RLENGTH)
      exit
    }
    in_frontend && /^[a-zA-Z]/ { in_frontend=0; in_ports=0 }
  ' "$OLD_COMPOSE"
)

if [[ -z "$OLD_FRONTEND_HOST_PORT" ]]; then
  echo "  No frontend ports entry found, no port migration needed."
elif [[ "$OLD_FRONTEND_HOST_PORT" == "3000" ]]; then
  echo "  Frontend host port was 3000 (default), no change needed."
else
  echo "  Custom frontend host port detected: ${OLD_FRONTEND_HOST_PORT}"
  env_set "$NEW_ENV" PORT "$OLD_FRONTEND_HOST_PORT"
  echo "  Set PORT=${OLD_FRONTEND_HOST_PORT} in .env."
  echo ""
  echo "  NOTE: The server container listens on port ${OLD_FRONTEND_HOST_PORT} and is"
  echo "  exposed to your host on the same port."
  echo "  Update CORS_ORIGIN in .env to use port ${OLD_FRONTEND_HOST_PORT} if it"
  echo "  references the old port."
fi

echo ""
echo "Migration complete."
echo ""
echo "  $BACKUP_DIR/docker-compose.yml   your old compose file (backup)"
[[ -f "$BACKUP_DIR/.env" ]] && echo "  $BACKUP_DIR/.env                 your old .env (backup)"
echo "  docker-compose.yml               new 2.0.0 compose file"
echo "  .env                             new 2.0.0 .env (values carried over)"
echo ""
echo "Please review the new .env and docker-compose.yml, then start the stack with:"
echo ""
echo "  docker compose up -d"
echo ""
echo "-----------------------------------------------------------------------------"
echo "CLEANUP REMINDER"
echo "-----------------------------------------------------------------------------"
echo ""
echo "Once you have confirmed everything is working correctly on 2.0.0, you can"
echo "safely remove the following Docker volumes that are no longer needed:"
echo ""
echo "  ${OLD_PROJECT}_agent_files"
echo "  ${OLD_PROJECT}_branding_assets"
echo ""
echo "To remove them run:"
echo ""
echo "  docker volume rm ${OLD_PROJECT}_agent_files ${OLD_PROJECT}_branding_assets"
echo ""
echo "Do NOT do this until you are confident the new stack is running as expected."
echo ""
