#!/bin/sh
# PatchMon Direct Host Auto-Enrollment Script
# POSIX-compliant shell script (works with dash, ash, bash, etc.)
# Usage: curl -s "https://patchmon.example.com/api/v1/auto-enrollment/script?type=direct-host&token_key=KEY&token_secret=SECRET" | sh

set -e

SCRIPT_VERSION="1.0.0"

# =============================================================================
# PatchMon Direct Host Auto-Enrollment Script
# =============================================================================
# This script automatically enrolls the current host into PatchMon for patch
# management.
#
# Usage:
#   curl -s "https://patchmon.example.com/api/v1/auto-enrollment/script?type=direct-host&token_key=KEY&token_secret=SECRET" | sh
#
#   With custom friendly name:
#   curl -s "https://patchmon.example.com/api/v1/auto-enrollment/script?type=direct-host&token_key=KEY&token_secret=SECRET" | FRIENDLY_NAME="My Server" sh
#
# Requirements:
#   - Run as root or with sudo
#   - Auto-enrollment token from PatchMon
#   - Network access to PatchMon server
# =============================================================================

# ===== CONFIGURATION =====
PATCHMON_URL="${PATCHMON_URL:-https://patchmon.example.com}"
AUTO_ENROLLMENT_KEY="${AUTO_ENROLLMENT_KEY:-}"
AUTO_ENROLLMENT_SECRET="${AUTO_ENROLLMENT_SECRET:-}"
CURL_FLAGS="${CURL_FLAGS:--s}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
FRIENDLY_NAME="${FRIENDLY_NAME:-}" # Optional: Custom friendly name for the host

# ===== COLOR OUTPUT =====
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ===== LOGGING FUNCTIONS =====
info() { printf "%b\n" "${GREEN}[INFO]${NC} $1"; }
warn() { printf "%b\n" "${YELLOW}[WARN]${NC} $1"; }
error() { printf "%b\n" "${RED}[ERROR]${NC} $1" >&2; exit 1; }
success() { printf "%b\n" "${GREEN}[SUCCESS]${NC} $1"; }
debug() { [ "${DEBUG:-false}" = "true" ] && printf "%b\n" "${BLUE}[DEBUG]${NC} $1" || true; }

# ===== BANNER =====
cat << "EOF"
╔═══════════════════════════════════════════════════════════════╗
║                                                               ║
║   ____       _       _     __  __                             ║
║  |  _ \ __ _| |_ ___| |__ |  \/  | ___  _ __                  ║
║  | |_) / _` | __/ __| '_ \| |\/| |/ _ \| '_ \                 ║
║  |  __/ (_| | || (__| | | | |  | | (_) | | | |                ║
║  |_|   \__,_|\__\___|_| |_|_|  |_|\___/|_| |_|                ║
║                                                               ║
║            Direct Host Auto-Enrollment Script                 ║
║                                                               ║
╚═══════════════════════════════════════════════════════════════╝
EOF
echo ""

# ===== VALIDATION =====
info "Validating configuration..."

if [ -z "$AUTO_ENROLLMENT_KEY" ] || [ -z "$AUTO_ENROLLMENT_SECRET" ]; then
    error "AUTO_ENROLLMENT_KEY and AUTO_ENROLLMENT_SECRET must be set"
fi

if [ -z "$PATCHMON_URL" ]; then
    error "PATCHMON_URL must be set"
fi

# Check if running as root
if [ "$(id -u)" -ne 0 ]; then
   error "This script must be run as root (use sudo)"
fi

# Check for required commands
for cmd in curl; do
    if ! command -v $cmd >/dev/null 2>&1; then
        error "Required command '$cmd' not found. Please install it first."
    fi
done

info "Configuration validated successfully"
info "PatchMon Server: $PATCHMON_URL"
echo ""

# ===== GATHER HOST INFORMATION =====
info "Gathering host information..."

# Get hostname
hostname=""
if command -v hostname >/dev/null 2>&1; then
    hostname=$(hostname 2>/dev/null || echo "")
fi
if [ -z "$hostname" ] && command -v hostnamectl >/dev/null 2>&1; then
    hostname=$(hostnamectl --static 2>/dev/null || echo "")
fi
if [ -z "$hostname" ] && [ -f /etc/hostname ]; then
    hostname=$(head -n 1 /etc/hostname 2>/dev/null || echo "")
fi
if [ -z "$hostname" ]; then
    hostname=$(uname -n 2>/dev/null || echo "")
fi
if [ -z "$hostname" ]; then
    error "Could not determine hostname. Install 'hostname' or set /etc/hostname, then retry."
fi

# Use FRIENDLY_NAME env var if provided, otherwise use hostname
if [ -n "$FRIENDLY_NAME" ]; then
    friendly_name="$FRIENDLY_NAME"
    info "Using custom friendly name: $friendly_name"
else
    friendly_name="$hostname"
fi

# Try to get machine_id (optional, for tracking)
machine_id=""
if [ -f /etc/machine-id ]; then
    machine_id=$(cat /etc/machine-id 2>/dev/null || echo "")
elif [ -f /var/lib/dbus/machine-id ]; then
    machine_id=$(cat /var/lib/dbus/machine-id 2>/dev/null || echo "")
fi

# Get OS information
os_info="unknown"
if [ -f /etc/os-release ]; then
    os_info=$(grep "^PRETTY_NAME=" /etc/os-release 2>/dev/null | cut -d'"' -f2 || echo "unknown")
fi

# Get IP address (first non-loopback)
ip_address=""
if command -v hostname >/dev/null 2>&1; then
    ip_address=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "")
fi
if [ -z "$ip_address" ] && command -v ip >/dev/null 2>&1; then
    ip_address=$(ip route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i < NF; i++) if ($i == "src") { print $(i + 1); exit }}' || echo "")
    if [ -z "$ip_address" ]; then
        ip_address=$(ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}' | cut -d'/' -f1 | head -n 1 || echo "")
    fi
fi
if [ -z "$ip_address" ] && command -v ifconfig >/dev/null 2>&1; then
    ip_address=$(ifconfig 2>/dev/null | awk '/inet /{addr = $2; sub(/^addr:/, "", addr); if (addr !~ /^127\./) { print addr; exit }}' || echo "")
fi
if [ -z "$ip_address" ]; then
    warn "Could not determine an IP address for this host"
    ip_address="unknown"
fi

# Detect architecture
arch_raw=$(uname -m 2>/dev/null || echo "unknown")
case "$arch_raw" in
    "x86_64"|"amd64")
        architecture="amd64"
        ;;
    "i386"|"i686")
        architecture="386"
        ;;
    "aarch64"|"arm64")
        architecture="arm64"
        ;;
    "armv7l"|"armv6l"|"arm")
        architecture="arm"
        ;;
    *)
        warn "  ⚠ Unknown architecture '$arch_raw', defaulting to amd64"
        architecture="amd64"
        ;;
esac

# Detect OS. The server cannot work this out for itself at this point: the host
# record is created with os_type "unknown" and no expected_platform, so it has
# no signal until the agent's first report, which cannot happen until the
# correct binary is installed.
case "$(uname -s 2>/dev/null)" in
    "FreeBSD")
        os_type="freebsd"
        ;;
    *)
        os_type="linux"
        ;;
esac

info "Hostname: $hostname"
info "Friendly Name: $friendly_name"
info "IP Address: $ip_address"
info "OS: $os_info"
info "Architecture: $architecture"
if [ -n "$machine_id" ]; then
    # POSIX-compliant substring (first 16 chars)
    machine_id_short=$(printf "%.16s" "$machine_id")
    info "Machine ID: ${machine_id_short}..."
else
    info "Machine ID: (not available)"
fi
echo ""

# ===== CHECK IF AGENT ALREADY INSTALLED =====
info "Checking if agent is already configured..."

config_check=$(sh -c "
    if [ -f /etc/patchmon/config.yml ] && [ -f /etc/patchmon/credentials.yml ]; then
        if [ -f /usr/local/bin/patchmon-agent ]; then
            # Try to ping using existing configuration
            if /usr/local/bin/patchmon-agent ping >/dev/null 2>&1; then
                echo 'ping_success'
            else
                echo 'ping_failed'
            fi
        else
            echo 'binary_missing'
        fi
    else
        echo 'not_configured'
    fi
" 2>/dev/null || echo "error")

if [ "$config_check" = "ping_success" ]; then
    success "Host already enrolled and agent ping successful - nothing to do"
    exit 0
elif [ "$config_check" = "ping_failed" ]; then
    warn "Agent configuration exists but ping failed - will reinstall"
elif [ "$config_check" = "binary_missing" ]; then
    warn "Config exists but agent binary missing - will reinstall"
elif [ "$config_check" = "not_configured" ]; then
    info "Agent not yet configured - proceeding with enrollment"
else
    warn "Could not check agent status - proceeding with enrollment"
fi
echo ""

# ===== ENROLL HOST =====
info "Enrolling $friendly_name in PatchMon..."

# Build JSON payload
json_payload=$(cat <<EOF
{
    "friendly_name": "$friendly_name",
    "metadata": {
        "hostname": "$hostname",
        "ip_address": "$ip_address",
        "os_info": "$os_info",
        "architecture": "$architecture"
    }
}
EOF
)

# Add machine_id if available
if [ -n "$machine_id" ]; then
    json_payload=$(echo "$json_payload" | sed "s/\"friendly_name\"/\"machine_id\": \"$machine_id\",\n    \"friendly_name\"/")
fi

curl_exit=0
response=$(curl $CURL_FLAGS --show-error -X POST \
    -H "X-Auto-Enrollment-Key: $AUTO_ENROLLMENT_KEY" \
    -H "X-Auto-Enrollment-Secret: $AUTO_ENROLLMENT_SECRET" \
    -H "Content-Type: application/json" \
    -d "$json_payload" \
    "$PATCHMON_URL/api/v1/auto-enrollment/enroll" \
    -w "\n%{http_code}" 2>&1) || curl_exit=$? #if curl fails, we store the exit code

if [ "$curl_exit" -ne 0 ]; then
    error "Could not reach $PATCHMON_URL - $(echo "$response" | sed '$d')"
fi

http_code=$(echo "$response" | tail -n 1)
body=$(echo "$response" | sed '$d')

if [ "$http_code" = "201" ]; then
    # Use grep and cut instead of jq since jq may not be installed
    api_id=$(echo "$body" | grep -o '"api_id":"[^"]*' | cut -d'"' -f4 || echo "")
    api_key=$(echo "$body" | grep -o '"api_key":"[^"]*' | cut -d'"' -f4 || echo "")

    if [ -z "$api_id" ] || [ -z "$api_key" ]; then
        error "Failed to parse API credentials from response"
    fi

    success "Host enrolled successfully: $api_id"
    echo ""

    # ===== INSTALL AGENT =====
    info "Installing PatchMon agent..."

    # Build install URL with force flag, architecture and OS
    install_url="$PATCHMON_URL/api/v1/hosts/install?arch=$architecture&os=$os_type"
    if [ "$FORCE_INSTALL" = "true" ]; then
        install_url="$install_url&force=true"
        info "Using force mode - will bypass broken packages"
    fi
    info "Using architecture: $architecture ($os_type)"
    
    # Download and execute installation script
    install_exit_code=0
    install_output=$(curl $CURL_FLAGS \
        -H "X-API-ID: $api_id" \
        -H "X-API-KEY: $api_key" \
        "$install_url" | sh 2>&1) || install_exit_code=$?

    # Check both exit code AND success message in output
    if [ "$install_exit_code" -eq 0 ] || echo "$install_output" | grep -q "PatchMon Agent installation completed successfully"; then
        success "Agent installed successfully"
    else
        error "Failed to install agent (exit: $install_exit_code)"
    fi
else
    printf "%b\n" "${RED}[ERROR]${NC} Failed to enroll $friendly_name - HTTP $http_code" >&2
    printf "%b\n" "Response: $body" >&2
    exit 1
fi

echo ""
success "Auto-enrollment complete!"
exit 0
