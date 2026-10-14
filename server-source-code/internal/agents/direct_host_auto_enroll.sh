#!/bin/sh
# PatchMon Direct Host Auto-Enrollment Script
# POSIX-compliant shell script (works with dash, ash, bash, etc.)
# Usage: curl -s "https://patchmon.example.com/api/v1/auto-enrollment/script?type=direct-host&token_key=KEY&token_secret=SECRET" | sh

set -e

SCRIPT_VERSION="1.2.0"

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
#   Enrol as a new host, ignoring any existing credentials and host_id (for
#   example on a cloned machine):
#   curl -s "...same URL..." | FORCE_NEW_HOST=true sh
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
FORCE_NEW_HOST="${FORCE_NEW_HOST:-false}"

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

json_escape() {
    printf '%s' "$1" | tr -d '\n\r\t' | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

patchmon_identity() {
    _pi_dir="${1:-/etc/patchmon}"
    _pi_id=""
    _pi_key=""
    _pi_host=""
    for _pi_file in "$_pi_dir/credentials.yml" $(ls -t "$_pi_dir"/credentials.yml.backup.* 2>/dev/null); do
        [ -f "$_pi_file" ] || continue
        _pi_id=$(sed -n 's/^api_id:[[:space:]]*//p' "$_pi_file" | head -n 1 | tr -d "\"' \r")
        _pi_key=$(sed -n 's/^api_key:[[:space:]]*//p' "$_pi_file" | head -n 1 | tr -d "\"' \r")
        if [ -n "$_pi_id" ] && [ -n "$_pi_key" ]; then
            break
        fi
        _pi_id=""
        _pi_key=""
    done
    _pi_server=""
    for _pi_file in "$_pi_dir/config.yml" $(ls -t "$_pi_dir"/config.yml.backup.* 2>/dev/null); do
        [ -f "$_pi_file" ] || continue
        _pi_server=$(sed -n 's/^patchmon_server:[[:space:]]*//p' "$_pi_file" | head -n 1 | tr -d "\"' \r")
        [ -n "$_pi_server" ] || continue
        _pi_host=$(sed -n 's/^host_id:[[:space:]]*//p' "$_pi_file" | head -n 1 | tr -d "\"' \r")
        break
    done
    case "$_pi_id" in *[!A-Za-z0-9_]*) _pi_id="" ;; esac
    case "$_pi_key" in *[!A-Za-z0-9]*) _pi_key="" ;; esac
    case "$_pi_host" in *[!A-Za-z0-9-]*) _pi_host="" ;; esac
    case "$_pi_server" in *[!A-Za-z0-9:/._-]*) _pi_server="" ;; esac
    printf 'api_id=%s\napi_key=%s\nhost_id=%s\nserver=%s\n' "$_pi_id" "$_pi_key" "$_pi_host" "$_pi_server"
}

identity_value() {
    _iv=$(printf '%s\n' "$1" | sed -n "s/^$2=//p" | head -n 1)
    case "$_iv" in *[!A-Za-z0-9:/._-]*) _iv="" ;; esac
    printf '%s' "$_iv"
}

same_server() {
    [ -n "$1" ] && [ "${1%/}" = "${2%/}" ]
}

enrol_outcome() {
    case "$1" in
        201) echo "created" ;;
        200) echo "reenrolled" ;;
        409)
            _eo_code=$(printf '%s' "$2" | grep -o '"code":"[^"]*"' | head -n 1 | cut -d'"' -f4 || true)
            case "$_eo_code" in
                host_reporting|host_not_reenrollable) echo "$_eo_code" ;;
                *) echo "already_enrolled" ;;
            esac
            ;;
        429) echo "retry_later" ;;
        *) echo "failed" ;;
    esac
}

# Go sorts map keys, so host.id is the last "id" in the body, after any host_group.id.
enrolled_host_id() {
    printf '%s' "$1" | grep -o '"id":"[^"]*"' | tail -n 1 | cut -d'"' -f4 || true
}

set_aside_identity() {
    _sa_dir="${1:-/etc/patchmon}"
    _sa_to="$_sa_dir/pre-force-new-$(date +%Y%m%d_%H%M%S)"
    for _sa_f in "$_sa_dir"/config.yml "$_sa_dir"/config.yml.backup.* "$_sa_dir"/credentials.yml "$_sa_dir"/credentials.yml.backup.*; do
        [ -f "$_sa_f" ] || continue
        mkdir -p "$_sa_to" && chmod 700 "$_sa_to" && mv "$_sa_f" "$_sa_to/" || return 1
    done
    if [ -d "$_sa_to" ]; then
        printf '%s\n' "$_sa_to"
    fi
    return 0
}

restore_identity() {
    case "$2" in "$1"/pre-force-new-*) ;; *) return 0 ;; esac
    [ -d "$2" ] || return 0
    for _ri_f in "$2"/*; do
        [ -e "$_ri_f" ] || continue
        [ -e "$1/${_ri_f##*/}" ] || mv "$_ri_f" "$1/"
    done
    rmdir "$2" 2>/dev/null || true
    return 0
}

write_stub_config() {
    [ -f "$1/config.yml" ] && return 0
    case "$3" in ""|*[!A-Za-z0-9-]*) return 0 ;; esac
    mkdir -p "$1" || return 0
    (umask 077 && printf 'patchmon_server: "%s"\nhost_id: "%s"\n' "$2" "$3" > "$1/config.yml") || true
}

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

# Same sources and order as the install script, so the server sees one ID per machine
get_machine_id() {
    if [ "$(uname -s 2>/dev/null)" = "FreeBSD" ]; then
        if [ -f /etc/hostid ]; then
            cat /etc/hostid
            return
        fi
        _uuid=$(sysctl -n kern.hostuuid 2>/dev/null)
        if [ -n "$_uuid" ]; then
            echo "$_uuid"
            return
        fi
        echo "patchmon-freebsd-$(hostname 2>/dev/null)-$(date +%s 2>/dev/null)"
        return
    fi
    if [ -f /etc/machine-id ]; then
        cat /etc/machine-id
    elif [ -f /var/lib/dbus/machine-id ]; then
        cat /var/lib/dbus/machine-id
    else
        echo "patchmon-$(cat /sys/class/dmi/id/product_uuid 2>/dev/null || cat /proc/sys/kernel/random/uuid 2>/dev/null)"
    fi
}
machine_id=$(get_machine_id 2>/dev/null | head -n 1 || echo "")

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

if [ "$FORCE_NEW_HOST" = "true" ]; then
    warn "FORCE_NEW_HOST=true: setting aside this machine's PatchMon credentials and host_id, a new host will be created"
    if ! set_aside_dir=$(set_aside_identity /etc/patchmon); then
        error "Could not move the existing configuration out of /etc/patchmon"
    fi
    if [ -n "$set_aside_dir" ]; then
        info "Previous configuration moved to $set_aside_dir"
        force_enrolled=false
        trap 'if [ "$force_enrolled" != "true" ]; then restore_identity /etc/patchmon "$set_aside_dir"; warn "No new host was created, so the previous configuration was put back"; fi' EXIT
        trap 'exit 130' INT TERM HUP
    fi
fi

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
    warn "Agent configuration exists but ping failed - repairing"
elif [ "$config_check" = "binary_missing" ]; then
    warn "Config exists but agent binary missing - repairing"
elif [ "$config_check" = "not_configured" ]; then
    info "Agent not yet configured - proceeding with enrollment"
else
    warn "Could not check agent status - proceeding with enrollment"
fi
echo ""

install_url="$PATCHMON_URL/api/v1/hosts/install?arch=$architecture&os=$os_type"
if [ "$FORCE_INSTALL" = "true" ]; then
    install_url="$install_url&force=true"
    info "Using force mode - will bypass broken packages"
fi

run_install() {
    info "Installing PatchMon agent..."
    info "Using architecture: $architecture ($os_type)"
    install_exit_code=0
    install_output=$(sh "$1" </dev/null 2>&1) || install_exit_code=$?
    rm -f "$1"
    if [ "$install_exit_code" -eq 0 ] || echo "$install_output" | grep -q "PatchMon Agent installation completed successfully"; then
        success "Agent installed successfully"
    else
        error "Failed to install agent (exit: $install_exit_code)"
    fi
}

fetch_install_script() {
    install_script=$(mktemp)
    fetch_exit=0
    fetch_code=$(curl $CURL_FLAGS -o "$install_script" -w "%{http_code}" \
        -H "X-API-ID: $1" \
        -H "X-API-KEY: $2" \
        "$install_url") || fetch_exit=$?
    if [ "$fetch_exit" -ne 0 ]; then
        fetch_code="000"
    fi
}

existing_api_id=""
existing_api_key=""
existing_host_id=""
if [ "$FORCE_NEW_HOST" != "true" ]; then
    identity=$(patchmon_identity /etc/patchmon)
    if same_server "$(identity_value "$identity" server)" "$PATCHMON_URL"; then
        existing_api_id=$(identity_value "$identity" api_id)
        existing_api_key=$(identity_value "$identity" api_key)
        existing_host_id=$(identity_value "$identity" host_id)
    elif [ -n "$(identity_value "$identity" api_id)" ]; then
        warn "Existing agent configuration points at a different PatchMon server; not reusing its credentials"
    fi
fi

# ===== REUSE EXISTING CREDENTIALS =====
if [ -n "$existing_api_id" ] && [ -n "$existing_api_key" ]; then
    info "Found existing credentials ($existing_api_id), checking them with PatchMon..."
    fetch_install_script "$existing_api_id" "$existing_api_key"
    case "$fetch_code" in
        200)
            success "Existing credentials are valid - reinstalling the agent with them"
            run_install "$install_script"
            echo ""
            success "Agent repaired with its existing credentials, no new enrolment was needed"
            exit 0
            ;;
        401)
            rm -f "$install_script"
            warn "Existing credentials are no longer valid - enrolling"
            ;;
        *)
            rm -f "$install_script"
            error "Could not check the existing credentials with $PATCHMON_URL (HTTP $fetch_code). Not enrolling, so that a temporary failure cannot create a duplicate host. Retry later."
            ;;
    esac
fi

# ===== ENROLL HOST =====
info "Enrolling $friendly_name in PatchMon..."

host_id_field=""
if [ -n "$existing_host_id" ]; then
    info "Sending existing host_id $existing_host_id"
    host_id_field=$(printf ',"host_id":"%s"' "$existing_host_id")
fi

json_payload=$(printf '{"friendly_name":"%s","machine_id":"%s"%s,"metadata":{"hostname":"%s","ip_address":"%s","os_info":"%s","architecture":"%s"}}' \
    "$(json_escape "$friendly_name")" \
    "$(json_escape "$machine_id")" \
    "$host_id_field" \
    "$(json_escape "$hostname")" \
    "$(json_escape "$ip_address")" \
    "$(json_escape "$os_info")" \
    "$architecture")

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
existing_name=$(echo "$body" | grep -o '"friendly_name":"[^"]*"' | cut -d'"' -f4 || echo "")

case "$(enrol_outcome "$http_code" "$body")" in
    created|reenrolled)
        # Use grep and cut instead of jq since jq may not be installed
        api_id=$(echo "$body" | grep -o '"api_id":"[^"]*' | cut -d'"' -f4 || echo "")
        api_key=$(echo "$body" | grep -o '"api_key":"[^"]*' | cut -d'"' -f4 || echo "")
        new_host_id=$(enrolled_host_id "$body")

        if [ -z "$api_id" ] || [ -z "$api_key" ]; then
            error "Failed to parse API credentials from response"
        fi

        if [ "$http_code" = "200" ]; then
            success "Credentials re-issued for the existing host: $api_id"
        else
            success "Host enrolled successfully: $api_id"
        fi
        force_enrolled=true
        write_stub_config /etc/patchmon "$PATCHMON_URL" "$new_host_id"
        echo ""

        fetch_install_script "$api_id" "$api_key"
        if [ "$fetch_code" != "200" ]; then
            rm -f "$install_script"
            rerun_hint="Rerun this script to retry"
            [ "$FORCE_NEW_HOST" = "true" ] && rerun_hint="Rerun this script without FORCE_NEW_HOST to retry"
            error "Failed to download the install script (HTTP $fetch_code). $rerun_hint; the host will not be enrolled twice."
        fi
        run_install "$install_script"
        ;;
    host_reporting)
        warn "PatchMon says this host${existing_name:+ ($existing_name)} is already reporting; skipping"
        warn "This is usually a cloned machine, or credentials regenerated in PatchMon but not applied here."
        warn "Reinstall it with the install command on the host's page, or rerun with FORCE_NEW_HOST=true to enrol this machine as a separate host."
        exit 0
        ;;
    host_not_reenrollable)
        warn "This host${existing_name:+ ($existing_name)} was added to PatchMon another way; skipping"
        warn "Use the install command on the host's page in PatchMon."
        exit 0
        ;;
    already_enrolled)
        if [ "$FORCE_NEW_HOST" = "true" ]; then
            printf "%b\n" "${RED}[ERROR]${NC} FORCE_NEW_HOST: PatchMon already has a host with this machine ID and name${existing_name:+ ($existing_name)}: the machine this one was cloned from, or a host an earlier FORCE_NEW_HOST run already created for it. Do not remove it. If it is the original, rerun with a different FRIENDLY_NAME or give this machine its own /etc/machine-id; if it is this machine's new host, rerun without FORCE_NEW_HOST." >&2
            exit 1
        fi
        if [ -n "$existing_name" ]; then
            warn "This host is already enrolled in PatchMon as '$existing_name'; skipping"
        else
            warn "This host is already enrolled in PatchMon; skipping"
        fi
        warn "Remove the host in PatchMon before it can be enrolled again"
        exit 0
        ;;
    retry_later)
        printf "%b\n" "${RED}[ERROR]${NC} PatchMon asked to retry later (HTTP 429): $body" >&2
        exit 1
        ;;
    *)
        printf "%b\n" "${RED}[ERROR]${NC} Failed to enroll $friendly_name - HTTP $http_code" >&2
        printf "%b\n" "Response: $body" >&2
        exit 1
        ;;
esac

echo ""
success "Auto-enrollment complete!"
exit 0
