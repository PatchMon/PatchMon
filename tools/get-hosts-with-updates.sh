#!/bin/bash

# Script to get list of hosts with pending updates from PatchMon
# Usage: ./get-hosts-with-updates.sh [--json] [--debug] [--filter MODE] [--url URL] [--username USER] [--password PASS]
#
# Compatible with PatchMon v1.3.7 (uses JWT auth + /dashboard/hosts endpoint)
# Will be removing this in the near future

set -uo pipefail

# Default values
JSON_OUTPUT=false
DEBUG=false
FILTER="outdated"   # all | outdated | security | uptodate
API_URL="${PATCHMON_URL:-http://localhost:3001}"
USERNAME="${PATCHMON_USERNAME:-}"
PASSWORD="${PATCHMON_PASSWORD:-}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --json)       JSON_OUTPUT=true; shift ;;
        --debug)      DEBUG=true; shift ;;
        --filter)     FILTER="$2"; shift 2 ;;
        --url)        API_URL="$2"; shift 2 ;;
        --username)   USERNAME="$2"; shift 2 ;;
        --password)   PASSWORD="$2"; shift 2 ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Fetches hosts from PatchMon and shows which have pending updates."
            echo "Uses your PatchMon login credentials (JWT auth) — works with v1.3.7+."
            echo ""
            echo "Options:"
            echo "  --json              Output results in JSON format"
            echo "  --debug             Enable debug output"
            echo "  --filter MODE       Filter hosts by update status (default: outdated)"
            echo "                        all       — show all hosts"
            echo "                        outdated  — hosts with any pending updates"
            echo "                        security  — hosts with security updates only"
            echo "                        uptodate  — hosts that are fully up to date"
            echo "  --url URL           PatchMon server URL (default: http://localhost:3001)"
            echo "  --username USER     PatchMon login username"
            echo "  --password PASS     PatchMon login password"
            echo "  --help, -h          Show this help message"
            echo ""
            echo "Environment Variables:"
            echo "  PATCHMON_URL        Server URL (e.g. https://pmon.example.com)"
            echo "  PATCHMON_USERNAME   Login username"
            echo "  PATCHMON_PASSWORD   Login password"
            echo ""
            echo "Examples:"
            echo "  $0 --url https://pmon.example.com --username admin --password secret"
            echo "  $0 --filter outdated                    # hosts with any updates pending"
            echo "  $0 --filter security --json             # hosts with security updates (JSON)"
            echo "  $0 --filter uptodate                    # hosts that are up to date"
            echo "  $0 --filter all --json                  # all hosts with their status"
            exit 0
            ;;
        *) echo -e "${RED}Error: Unknown option $1${NC}" >&2; exit 1 ;;
    esac
done

# Validate filter mode
case "$FILTER" in
    all|outdated|security|uptodate) ;;
    *)
        echo -e "${RED}Error: Invalid filter mode '$FILTER'${NC}" >&2
        echo "Valid modes: all, outdated, security, uptodate" >&2
        exit 1
        ;;
esac

# Validate required credentials
if [[ -z "$USERNAME" ]] || [[ -z "$PASSWORD" ]]; then
    echo -e "${RED}Error: PatchMon login credentials are required${NC}" >&2
    echo "Provide via --username/--password flags or PATCHMON_USERNAME/PATCHMON_PASSWORD env vars" >&2
    exit 1
fi

# Check dependencies
for cmd in curl jq; do
    if ! command -v "$cmd" &>/dev/null; then
        echo -e "${RED}Error: $cmd is required but not installed${NC}" >&2
        exit 1
    fi
done

# Normalize URL: strip trailing slash, strip /api/v1 suffix if user included it
API_URL="${API_URL%/}"
API_URL="${API_URL%/api/v1}"
API_URL="${API_URL%/}"
BASE_API="${API_URL}/api/v1"

debug() {
    if [[ "$DEBUG" == true ]]; then
        echo -e "${YELLOW}[DEBUG]${NC} $*" >&2
    fi
}

debug "Server URL: $API_URL"
debug "API base:   $BASE_API"
debug "Filter:     $FILTER"

# ── Step 1: Login to get JWT token ────────────────────────────────────────────

if [[ "$JSON_OUTPUT" != true ]]; then
    echo -e "${BLUE}Logging in to PatchMon...${NC}"
fi

login_tmp=$(mktemp)
login_payload=$(jq -nc --arg u "$USERNAME" --arg p "$PASSWORD" '{username: $u, password: $p}')
debug "Login payload: $(echo "$login_payload" | jq -c '{username: .username, password: "***"}')"
login_http_code=$(curl -sk -w "%{http_code}" \
    -o "$login_tmp" \
    -X POST \
    -H "Content-Type: application/json" \
    -d "$login_payload" \
    "${BASE_API}/auth/login" 2>/dev/null) || true

login_body=$(<"$login_tmp")
rm -f "$login_tmp"

debug "Login HTTP $login_http_code"

if [[ "$login_http_code" != "200" ]]; then
    error_msg=$(echo "$login_body" | jq -r '.error // .message // "Unknown error"' 2>/dev/null || echo "$login_body")
    echo -e "${RED}Error: Login failed (HTTP $login_http_code) — $error_msg${NC}" >&2
    exit 1
fi

# Check if TFA is required
requires_tfa=$(echo "$login_body" | jq -r '.requiresTfa // false' 2>/dev/null)
if [[ "$requires_tfa" == "true" ]]; then
    echo -e "${RED}Error: This account has Two-Factor Authentication enabled.${NC}" >&2
    echo -e "${YELLOW}TFA-protected accounts cannot be used with this script.${NC}" >&2
    echo -e "${YELLOW}Consider creating a separate service account without TFA for scripting.${NC}" >&2
    exit 1
fi

JWT_TOKEN=$(echo "$login_body" | jq -r '.token // empty' 2>/dev/null)

if [[ -z "$JWT_TOKEN" ]]; then
    echo -e "${RED}Error: No token received from login response${NC}" >&2
    debug "Response: $login_body"
    exit 1
fi

debug "JWT token obtained (${#JWT_TOKEN} chars)"

# ── Step 2: Fetch all hosts with update counts ───────────────────────────────
# GET /api/v1/dashboard/hosts returns each host with updatesCount,
# securityUpdatesCount, totalPackagesCount — all in one request!

if [[ "$JSON_OUTPUT" != true ]]; then
    echo -e "${BLUE}Fetching hosts with update status...${NC}"
fi

hosts_tmp=$(mktemp)
hosts_http_code=$(curl -sk -w "%{http_code}" \
    -o "$hosts_tmp" \
    -H "Authorization: Bearer $JWT_TOKEN" \
    -H "Content-Type: application/json" \
    "${BASE_API}/dashboard/hosts" 2>/dev/null) || true

hosts_body=$(<"$hosts_tmp")
rm -f "$hosts_tmp"

debug "Dashboard/hosts HTTP $hosts_http_code"

if [[ "$hosts_http_code" != "200" ]]; then
    error_msg=$(echo "$hosts_body" | jq -r '.error // .message // "Unknown error"' 2>/dev/null || echo "$hosts_body")
    echo -e "${RED}Error: Failed to fetch hosts (HTTP $hosts_http_code) — $error_msg${NC}" >&2
    exit 1
fi

# Validate response is a JSON array
if ! echo "$hosts_body" | jq -e 'type == "array"' >/dev/null 2>&1; then
    echo -e "${RED}Error: Unexpected response from /dashboard/hosts (expected JSON array)${NC}" >&2
    debug "Response: $(echo "$hosts_body" | head -c 500)"
    exit 1
fi

host_count=$(echo "$hosts_body" | jq 'length')
debug "Found $host_count hosts"

if [[ "$host_count" -eq 0 ]]; then
    if [[ "$JSON_OUTPUT" == true ]]; then
        echo '{"hosts":[],"total_hosts":0,"matched_hosts":0,"filter":"'"$FILTER"'"}'
    else
        echo -e "${YELLOW}No hosts found${NC}"
    fi
    exit 0
fi

# ── Step 3: Build jq filter expression based on --filter mode ─────────────────

case "$FILTER" in
    all)      jq_filter='true' ;; 
    outdated) jq_filter='.updatesCount > 0' ;;
    security) jq_filter='.securityUpdatesCount > 0' ;;
    uptodate) jq_filter='.updatesCount == 0' ;;
esac

# Human-readable label for the filter
case "$FILTER" in
    all)      filter_label="all hosts" ;;
    outdated) filter_label="hosts with pending updates" ;;
    security) filter_label="hosts with security updates" ;;
    uptodate) filter_label="hosts that are up to date" ;;
esac

# ── Step 4: Output results ───────────────────────────────────────────────────

if [[ "$JSON_OUTPUT" == true ]]; then
    # Pure jq pipeline — fast and clean
    echo "$hosts_body" | jq -c \
        --argjson total "$host_count" \
        --arg filter "$FILTER" \
        '{
            hosts: [
                .[] | select('"$jq_filter"') |
                {
                    id: .id,
                    friendly_name: .friendly_name,
                    hostname: .hostname,
                    ip: .ip,
                    os_type: .os_type,
                    os_version: .os_version,
                    outdated_packages: .updatesCount,
                    security_updates: .securityUpdatesCount,
                    total_packages: .totalPackagesCount,
                    last_update: .last_update,
                    status: .effectiveStatus,
                    host_groups: [.host_group_memberships[]?.host_groups // empty | {id: .id, name: .name}]
                }
            ],
            total_hosts: $total,
            matched_hosts: ([.[] | select('"$jq_filter"')] | length),
            filter: $filter
        }'
else
    echo ""
    matched=0

    while IFS= read -r host; do
        friendly_name=$(echo "$host" | jq -r '.friendly_name // .hostname // "Unknown"')
        hostname_val=$(echo "$host" | jq -r '.hostname // "Unknown"')
        ip=$(echo "$host" | jq -r '.ip // "N/A"')
        outdated=$(echo "$host" | jq -r '.updatesCount // 0')
        security=$(echo "$host" | jq -r '.securityUpdatesCount // 0')

        debug "$friendly_name → outdated=$outdated, security=$security"

        ((matched++))

        if [[ "$FILTER" == "uptodate" ]]; then
            echo -e "  ${GREEN}✓${NC} ${BOLD}${friendly_name}${NC} (${hostname_val} / ${ip})"
        elif [[ "$outdated" -gt 0 ]]; then
            echo -e "  ${YELLOW}●${NC} ${BOLD}${friendly_name}${NC} (${hostname_val} / ${ip})"
            echo -e "    ${YELLOW}${outdated}${NC} updates pending"
            if [[ "$security" -gt 0 ]]; then
                echo -e "    ${RED}${security} security updates${NC}"
            fi
        else
            echo -e "  ${GREEN}✓${NC} ${BOLD}${friendly_name}${NC} (${hostname_val} / ${ip})"
        fi
    done < <(echo "$hosts_body" | jq -c "[.[] | select($jq_filter)][]")

    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}Filter:${NC}                   $filter_label"
    echo -e "${BOLD}Matched:${NC}                  ${matched} / ${host_count} hosts"
fi
