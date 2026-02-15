#!/bin/bash
# One-time Guacamole database initialization
# Run after guacamole-db is up:
#   docker compose -f docker/docker-compose.dev.yml up -d guacamole-db
#   sleep 5
#   ./docker/guacamole-init-db.sh

set -e
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.dev.yml"
GUAC_PASS="1NS3CU6E_DEV_GUAC_PASS"

# Get guacamole-db container ID
DB_CONTAINER=$(docker compose -f "$COMPOSE_FILE" ps -q guacamole-db 2>/dev/null | head -1)
if [ -z "$DB_CONTAINER" ]; then
  echo "Error: guacamole-db container not running. Start it first with:"
  echo "  docker compose -f docker/docker-compose.dev.yml up -d guacamole-db"
  exit 1
fi

echo "Initializing Guacamole database..."
docker run --rm guacamole/guacamole /opt/guacamole/bin/initdb.sh --postgresql 2>/dev/null | \
  docker exec -e PGPASSWORD="$GUAC_PASS" -i "$DB_CONTAINER" psql -U guacamole_user -d guacamole_db -f - > /dev/null

echo "Guacamole database initialized successfully."
