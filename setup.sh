#!/bin/bash
# Native installation is no longer supported; this script only points at the Docker routes.

set -e

BOLD='\033[1m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo ""
echo -e "${BOLD}PatchMon Installation Guide${NC}"
echo "════════════════════════════════════════════════════════════════"
echo ""
echo -e "${YELLOW}Native (bare-metal) installation is no longer supported.${NC}"
echo ""
echo "PatchMon now runs as a Docker container. This gives you automatic"
echo "database migrations, bundled agent binaries, compliance content,"
echo "and simple one-command updates."
echo ""
echo -e "${BOLD}Option 1: Full setup (installs Docker if needed)${NC}"
echo ""
echo -e "  ${CYAN}curl -fsSL https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/setup-docker.sh | sudo bash${NC}"
echo ""
echo "  This will:"
echo "  • Install Docker Engine + Compose if not already present"
echo "  • Create /opt/patchmon with docker-compose.yml and .env"
echo "  • Generate any missing secrets (an existing .env keeps its secrets and settings)"
echo "  • Ask for your access URL, reverse proxy setting and timezone, even when piped through sudo bash"
echo "  • Start PatchMon"
echo ""
echo -e "${BOLD}Option 2: Already have Docker?${NC}"
echo ""
echo -e "  ${CYAN}mkdir patchmon && cd patchmon${NC}"
echo -e "  ${CYAN}bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/docker/setup-env.sh)\"${NC}"
echo -e "  ${CYAN}docker compose up -d${NC}"
echo ""
echo "  Works on Linux and macOS. Re-running it on an existing .env keeps your secrets."
echo ""
echo -e "${BOLD}Option 3: Update an existing installation${NC}"
echo ""
echo -e "  ${CYAN}curl -fsSL https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/setup-docker.sh | sudo bash -s -- --update${NC}"
echo ""
echo "  Or from your PatchMon directory:"
echo -e "  ${CYAN}docker compose pull && docker compose up -d${NC}"
echo ""
echo "Documentation: https://patchmon.net/docs"
echo ""
