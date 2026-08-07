#!/bin/bash
# =============================================================================
# PatchMon Docker Setup Script
# =============================================================================
# Full automated setup: installs Docker if needed, creates /opt/patchmon,
# generates secrets, configures access URL, and starts PatchMon.
#
# Install:
#   curl -fsSL https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/setup-docker.sh | sudo bash
#
# Update:
#   curl -fsSL https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/setup-docker.sh | sudo bash -s -- --update
#
# Or from your PatchMon directory:
#   sudo ./setup-docker.sh --update
#
# Supports: Ubuntu 20.04+, Debian 11+, RHEL/CentOS/Rocky/Alma 8+, Fedora 38+
# =============================================================================

set -euo pipefail

# -----------------------------------------------------------------------------
# Constants
# -----------------------------------------------------------------------------
PATCHMON_DIR="/opt/patchmon"
UPSTREAM="https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main"
MIN_DOCKER_VERSION="24"
MIN_COMPOSE_VERSION="2"

# -----------------------------------------------------------------------------
# Colors
# -----------------------------------------------------------------------------
BOLD='\033[1m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# -----------------------------------------------------------------------------
# Helpers
# -----------------------------------------------------------------------------
info()  { echo -e "${CYAN}ℹ${NC} $*"; }
ok()    { echo -e "${GREEN}✓${NC} $*"; }
warn()  { echo -e "${YELLOW}⚠${NC} $*"; }
fail()  { echo -e "${RED}✗${NC} $*" >&2; exit 1; }

command_exists() { command -v "$1" >/dev/null 2>&1; }

version_ge() {
  # Returns 0 if $1 >= $2 (numeric major version comparison)
  [ "$(printf '%s\n' "$1" "$2" | sort -V | head -n1)" = "$2" ]
}

# -----------------------------------------------------------------------------
# Root check
# -----------------------------------------------------------------------------
if [ "$(id -u)" -ne 0 ]; then
  fail "This script must be run as root (or with sudo)."
fi

# -----------------------------------------------------------------------------
# Parse arguments
# -----------------------------------------------------------------------------
MODE="install"
for arg in "$@"; do
  case "$arg" in
    --update)  MODE="update" ;;
    --help|-h)
      echo "Usage: setup-docker.sh [--update]"
      echo ""
      echo "  (no args)   Full installation: Docker + PatchMon"
      echo "  --update    Pull latest image and restart PatchMon"
      exit 0
      ;;
  esac
done

# =============================================================================
# UPDATE MODE
# =============================================================================
if [ "$MODE" = "update" ]; then
  echo ""
  echo -e "${BOLD}PatchMon — Update${NC}"
  echo "════════════════════════════════════════════════════════════════"
  echo ""

  # Find the PatchMon directory
  if [ -f "$PATCHMON_DIR/docker-compose.yml" ]; then
    cd "$PATCHMON_DIR"
  elif [ -f "./docker-compose.yml" ] && grep -q "patchmon-server" ./docker-compose.yml 2>/dev/null; then
    PATCHMON_DIR="$(pwd)"
  else
    fail "Cannot find PatchMon installation. Expected docker-compose.yml in $PATCHMON_DIR or current directory."
  fi

  info "PatchMon directory: $PATCHMON_DIR"

  # Check current image before pull
  CURRENT_IMAGE=$(docker compose images server --format '{{.ID}}' 2>/dev/null || echo "unknown")
  info "Current image: $CURRENT_IMAGE"

  # Pull latest image
  info "Pulling latest PatchMon image..."
  docker compose pull server

  NEW_IMAGE=$(docker compose images server --format '{{.ID}}' 2>/dev/null || echo "unknown")

  if [ "$CURRENT_IMAGE" = "$NEW_IMAGE" ] && [ "$CURRENT_IMAGE" != "unknown" ]; then
    ok "Already running the latest image."
    echo ""
    exit 0
  fi

  # Restart with new image
  info "Restarting PatchMon with new image..."
  docker compose up -d

  # Wait for health
  info "Waiting for PatchMon to become healthy..."
  TIMEOUT=60
  ELAPSED=0
  while [ $ELAPSED -lt $TIMEOUT ]; do
    if docker compose ps server --format '{{.Health}}' 2>/dev/null | grep -qi "healthy"; then
      break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
  done

  if [ $ELAPSED -ge $TIMEOUT ]; then
    warn "Server hasn't reported healthy within ${TIMEOUT}s. Check logs:"
    echo "  docker compose logs -f server"
  else
    ok "PatchMon updated and running."
  fi

  echo ""
  echo "Check logs:  docker compose -f $PATCHMON_DIR/docker-compose.yml logs -f server"
  echo ""
  exit 0
fi

# =============================================================================
# INSTALL MODE
# =============================================================================
echo ""
echo -e "${BOLD}PatchMon — Full Docker Setup${NC}"
echo "════════════════════════════════════════════════════════════════"
echo ""

# -----------------------------------------------------------------------------
# Step 1: Check if PatchMon is already installed
# -----------------------------------------------------------------------------
if [ -f "$PATCHMON_DIR/docker-compose.yml" ] && grep -q "patchmon-server" "$PATCHMON_DIR/docker-compose.yml" 2>/dev/null; then
  warn "PatchMon is already installed at $PATCHMON_DIR"
  echo ""
  if [ -t 0 ]; then
    read -r -p "Do you want to reinstall? This will NOT delete your data volumes. (y/n) [n]: " reinstall
    reinstall=${reinstall:-n}
    if [ "$reinstall" != "y" ] && [ "$reinstall" != "Y" ]; then
      echo ""
      info "To update instead, run: sudo $0 --update"
      exit 0
    fi
  else
    fail "PatchMon already installed. Use --update to update, or remove $PATCHMON_DIR to reinstall."
  fi
fi

# -----------------------------------------------------------------------------
# Step 2: Detect OS
# -----------------------------------------------------------------------------
if [ -f /etc/os-release ]; then
  . /etc/os-release
  OS_ID="${ID:-unknown}"
  OS_VERSION="${VERSION_ID:-0}"
else
  OS_ID="unknown"
  OS_VERSION="0"
fi
info "Detected OS: $OS_ID $OS_VERSION"

# -----------------------------------------------------------------------------
# Step 3: Check / Install Docker Engine
# -----------------------------------------------------------------------------
echo ""
echo -e "${BOLD}Step 1: Docker Engine${NC}"
echo "────────────────────────────────────────────────────────────────"

DOCKER_INSTALLED=false
if command_exists docker; then
  DOCKER_VER=$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo "0")
  DOCKER_MAJOR=$(echo "$DOCKER_VER" | cut -d. -f1)
  if version_ge "$DOCKER_MAJOR" "$MIN_DOCKER_VERSION"; then
    ok "Docker $DOCKER_VER already installed (>= $MIN_DOCKER_VERSION required)"
    DOCKER_INSTALLED=true
  else
    warn "Docker $DOCKER_VER is installed but version $MIN_DOCKER_VERSION+ is required."
  fi
fi

if [ "$DOCKER_INSTALLED" = false ]; then
  info "Docker not found or too old. Installing Docker Engine..."

  case "$OS_ID" in
    ubuntu|debian)
      # Remove old packages
      for pkg in docker.io docker-doc docker-compose podman-docker containerd runc; do
        apt-get remove -y "$pkg" 2>/dev/null || true
      done

      # Install prerequisites
      apt-get update -qq
      apt-get install -y -qq ca-certificates curl gnupg

      # Add Docker GPG key
      install -m 0755 -d /etc/apt/keyrings
      if [ ! -f /etc/apt/keyrings/docker.gpg ]; then
        curl -fsSL "https://download.docker.com/linux/$OS_ID/gpg" | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
        chmod a+r /etc/apt/keyrings/docker.gpg
      fi

      # Add Docker repo
      echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/$OS_ID $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
        > /etc/apt/sources.list.d/docker.list

      apt-get update -qq
      apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

      ok "Docker installed via apt"
      ;;

    centos|rhel|rocky|almalinux|fedora)
      # Remove old packages
      dnf remove -y docker docker-client docker-client-latest docker-common \
        docker-latest docker-latest-logrotate docker-logrotate docker-engine 2>/dev/null || true

      dnf install -y dnf-plugins-core
      dnf config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo 2>/dev/null \
        || dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo

      dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

      ok "Docker installed via dnf"
      ;;

    *)
      fail "Unsupported OS: $OS_ID. Install Docker manually: https://docs.docker.com/engine/install/"
      ;;
  esac

  # Enable and start Docker
  systemctl enable --now docker
  ok "Docker service enabled and started"
fi

# Verify Docker is running
if ! docker info >/dev/null 2>&1; then
  fail "Docker is installed but not running. Try: systemctl start docker"
fi

# -----------------------------------------------------------------------------
# Step 4: Check Docker Compose
# -----------------------------------------------------------------------------
echo ""
echo -e "${BOLD}Step 2: Docker Compose${NC}"
echo "────────────────────────────────────────────────────────────────"

if docker compose version >/dev/null 2>&1; then
  COMPOSE_VER=$(docker compose version --short 2>/dev/null || echo "unknown")
  ok "Docker Compose $COMPOSE_VER available"
else
  fail "Docker Compose plugin not found. Install it: https://docs.docker.com/compose/install/"
fi

# -----------------------------------------------------------------------------
# Step 5: Create PatchMon directory
# -----------------------------------------------------------------------------
echo ""
echo -e "${BOLD}Step 3: PatchMon Setup${NC}"
echo "────────────────────────────────────────────────────────────────"

mkdir -p "$PATCHMON_DIR"
cd "$PATCHMON_DIR"
info "Working directory: $PATCHMON_DIR"

# -----------------------------------------------------------------------------
# Step 6: Download compose file and env template
# -----------------------------------------------------------------------------
info "Downloading docker-compose.yml..."
curl -fsSL -o docker-compose.yml "$UPSTREAM/docker/docker-compose.yml"
ok "docker-compose.yml downloaded"

info "Downloading env.example..."
curl -fsSL -o env.example "$UPSTREAM/docker/env.example"
ok "env.example downloaded"

# -----------------------------------------------------------------------------
# Step 7: Run the env setup script (generates secrets, configures URL)
# -----------------------------------------------------------------------------
echo ""
echo -e "${BOLD}Step 4: Configuration${NC}"
echo "────────────────────────────────────────────────────────────────"

# Download and run setup-env.sh in-place (it handles .env creation, secrets, CORS, timezone)
info "Downloading setup-env.sh..."
curl -fsSL -o setup-env.sh "$UPSTREAM/docker/setup-env.sh"
chmod +x setup-env.sh
ok "Running environment setup..."
echo ""
bash ./setup-env.sh

# -----------------------------------------------------------------------------
# Step 8: Pull images
# -----------------------------------------------------------------------------
echo ""
echo -e "${BOLD}Step 5: Pulling Images${NC}"
echo "────────────────────────────────────────────────────────────────"

info "Pulling Docker images (this may take a minute on first run)..."
docker compose pull
ok "All images pulled"

# -----------------------------------------------------------------------------
# Step 9: Start PatchMon
# -----------------------------------------------------------------------------
echo ""
echo -e "${BOLD}Step 6: Starting PatchMon${NC}"
echo "────────────────────────────────────────────────────────────────"

docker compose up -d
info "Waiting for services to become healthy..."

TIMEOUT=90
ELAPSED=0
while [ $ELAPSED -lt $TIMEOUT ]; do
  HEALTHY=$(docker compose ps --format '{{.Health}}' 2>/dev/null | grep -c "healthy" || true)
  TOTAL=$(docker compose ps --format '{{.Name}}' 2>/dev/null | wc -l || true)
  if [ "$HEALTHY" -ge "$TOTAL" ] && [ "$TOTAL" -gt 0 ]; then
    break
  fi
  sleep 3
  ELAPSED=$((ELAPSED + 3))
  printf "\r  Waiting... %ds / %ds (%s/%s healthy)" "$ELAPSED" "$TIMEOUT" "$HEALTHY" "$TOTAL"
done
echo ""

if [ $ELAPSED -ge $TIMEOUT ]; then
  warn "Not all services healthy within ${TIMEOUT}s. Check: docker compose logs"
else
  ok "All services healthy"
fi

# -----------------------------------------------------------------------------
# Step 10: Summary
# -----------------------------------------------------------------------------
echo ""
echo "════════════════════════════════════════════════════════════════"
echo -e "${GREEN}${BOLD} PatchMon is running!${NC}"
echo "════════════════════════════════════════════════════════════════"
echo ""

CORS_VAL=$(grep -E "^CORS_ORIGIN=" "$PATCHMON_DIR/.env" 2>/dev/null | cut -d= -f2- | tr -d '"' || echo "http://localhost:3000")
IFS=',' read -ra URLS <<< "$CORS_VAL"
echo "  Open PatchMon at:"
for url in "${URLS[@]}"; do
  echo -e "    ${CYAN}→ $url${NC}"
done

echo ""
echo "  Complete the first-time setup in your browser to create"
echo "  your admin account."
echo ""
echo -e "  ${BOLD}Installation directory:${NC}  $PATCHMON_DIR"
echo -e "  ${BOLD}Configuration file:${NC}     $PATCHMON_DIR/.env"
echo -e "  ${BOLD}Compose file:${NC}           $PATCHMON_DIR/docker-compose.yml"
echo ""
echo -e "  ${BOLD}Useful commands:${NC}"
echo "    View logs:      docker compose -f $PATCHMON_DIR/docker-compose.yml logs -f server"
echo "    Stop:           docker compose -f $PATCHMON_DIR/docker-compose.yml down"
echo "    Start:          docker compose -f $PATCHMON_DIR/docker-compose.yml up -d"
echo "    Update:         sudo $0 --update"
echo ""
echo "  Documentation:    https://patchmon.net/docs"
echo ""
