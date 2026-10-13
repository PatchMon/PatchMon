#!/bin/bash
set -eo pipefail  # Exit on error, pipe failures (removed -u as we handle unset vars explicitly)

# Trap to catch errors only (not normal exits)
trap 'echo "[ERROR] Script failed at line $LINENO with exit code $?"' ERR

SCRIPT_VERSION="2.2.0"
echo "[DEBUG] Script Version: $SCRIPT_VERSION ($(date +%Y-%m-%d\ %H:%M:%S))"

# =============================================================================
# PatchMon Proxmox LXC Auto-Enrollment Script
# =============================================================================
# This script discovers LXC containers on a Proxmox host and automatically
# enrolls them into PatchMon for patch management.
#
# Usage:
#   1. Set environment variables or edit configuration below
#   2. Run: bash proxmox_auto_enroll.sh
#
# Requirements:
#   - Must run on Proxmox host (requires 'pct' command)
#   - Auto-enrollment token from PatchMon
#   - Network access to PatchMon server
# =============================================================================

# ===== CONFIGURATION =====
PATCHMON_URL="${PATCHMON_URL:-https://patchmon.example.com}"
AUTO_ENROLLMENT_KEY="${AUTO_ENROLLMENT_KEY:-}"
AUTO_ENROLLMENT_SECRET="${AUTO_ENROLLMENT_SECRET:-}"
CURL_FLAGS="${CURL_FLAGS:--s}"
DRY_RUN="${DRY_RUN:-false}"
HOST_PREFIX="${HOST_PREFIX:-}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
FORCE_NEW_HOST="${FORCE_NEW_HOST:-false}"

# ===== COLOR OUTPUT =====
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ===== LOGGING FUNCTIONS =====
info() { echo -e "${GREEN}[INFO]${NC} $1"; return 0; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; return 0; }
error() { echo -e "${RED}[ERROR]${NC} $1" >&2; return 0; }
fatal() { echo -e "${RED}[ERROR]${NC} $1" >&2; exit 1; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; return 0; }
debug() { [[ "${DEBUG:-false}" == "true" ]] && echo -e "${BLUE}[DEBUG]${NC} $1" || true; return 0; }

# Under "curl | bash" stdin is the script itself, so a prompt must read the terminal directly
ask_yes_no() {
    local reply=""
    if [[ -t 0 ]]; then
        read -r -n 1 -p "$1" reply || true
    elif (exec </dev/tty) 2>/dev/null; then
        read -r -n 1 -p "$1" reply </dev/tty || true
    else
        warn "$1 no terminal available, assuming no"
        return 1
    fi
    echo ""
    [[ "$reply" =~ ^[Yy]$ ]]
}

# ===== BANNER =====
cat << "EOF"
╔═══════════════════════════════════════════════════════════════╗
║                                                               ║
║   ____       _       _     __  __                            ║
║  |  _ \ __ _| |_ ___| |__ |  \/  | ___  _ __                ║
║  | |_) / _` | __/ __| '_ \| |\/| |/ _ \| '_ \               ║
║  |  __/ (_| | || (__| | | | |  | | (_) | | | |              ║
║  |_|   \__,_|\__\___|_| |_|_|  |_|\___/|_| |_|              ║
║                                                               ║
║         Proxmox LXC Auto-Enrollment Script                   ║
║                                                               ║
╚═══════════════════════════════════════════════════════════════╝
EOF
echo ""

# ===== VALIDATION =====
info "Validating configuration..."

if [[ -z "$AUTO_ENROLLMENT_KEY" ]] || [[ -z "$AUTO_ENROLLMENT_SECRET" ]]; then
    fatal "AUTO_ENROLLMENT_KEY and AUTO_ENROLLMENT_SECRET must be set"
fi

if [[ -z "$PATCHMON_URL" ]]; then
    fatal "PATCHMON_URL must be set"
fi

# Check if running on Proxmox
if ! command -v pct &> /dev/null; then
    fatal "This script must run on a Proxmox host (pct command not found)"
fi

# Check for required commands
for cmd in curl jq; do
    if ! command -v $cmd &> /dev/null; then
        fatal "Required command '$cmd' not found. Please install it first."
    fi
done

info "Configuration validated successfully"
info "PatchMon Server: $PATCHMON_URL"
info "Dry Run Mode: $DRY_RUN"
echo ""

# ===== DISCOVER LXC CONTAINERS =====
info "Discovering LXC containers..."
# Only VMID and Status are positional; Name shifts when the Lock column is populated
lxc_list=$(pct list 2>/dev/null | awk 'NR > 1 && $1 ~ /^[0-9]+$/ {print $1, $2}') || fatal "Could not list LXC containers (pct list failed)"

if [[ -z "$lxc_list" ]]; then
    warn "No LXC containers found on this Proxmox host"
    exit 0
fi

# Count containers
total_containers=$(printf '%s\n' "$lxc_list" | wc -l | tr -d ' ')
info "Found $total_containers LXC container(s)"
echo ""

info "Initializing statistics..."
# ===== STATISTICS =====
enrolled_count=0
skipped_count=0
failed_count=0

# Track containers with dpkg errors for later recovery
declare -A dpkg_error_containers

# Track all failed containers for summary
declare -A failed_containers
info "Statistics initialized"

# Must stay stable across reruns: the server dedupes enrolments on machine_id and name
derive_machine_id() {
    local line
    while IFS= read -r line; do
        line=$(printf '%s' "$line" | tr -d '[:space:]')
        if [[ -n "$line" && "$line" != "uninitialized" ]]; then
            printf '%s\n' "$line"
            return 0
        fi
    done <<< "$1"
    printf 'proxmox-lxc-%s-%s\n' "$3" "$2"
}

# Run inside containers through pct exec, so keep these POSIX sh; the node re-checks their output
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

force_new_host_for() {
    [[ "$FORCE_NEW_HOST" == "true" ]] && return 0
    [[ ",${FORCE_NEW_HOST//[[:space:]]/,}," == *",$1,"* ]]
}

node_id=$( (tr -d '[:space:]' < /etc/machine-id) 2>/dev/null || true)
[[ -n "$node_id" ]] || node_id=$(hostname)

# ===== PROCESS CONTAINERS =====
info "Starting container processing loop..."
while read -r vmid status; do
    [[ -n "$vmid" ]] || continue
    name=$(pct config "$vmid" 2>/dev/null | sed -n 's/^hostname:[[:space:]]*//p' || true)
    [[ -n "$name" ]] || name="ct$vmid"

    info "Processing LXC $vmid: $name (status: $status)"

    # pct exec cannot reach a container that is not running
    if [[ "$status" != "running" ]]; then
        warn "  Skipping $name - container is $status, agent installation requires it running"
        ((skipped_count++)) || true
        echo ""
        continue
    fi

    # Get container details
    debug "  Gathering container information..."
    hostname=$(timeout 5 pct exec "$vmid" -- hostname 2>/dev/null </dev/null || echo "$name")
    ip_address=$(timeout 5 pct exec "$vmid" -- hostname -I 2>/dev/null </dev/null | awk '{print $1}' || echo "")
    if [ -z "$ip_address" ]; then
        ip_address=$(timeout 5 pct exec "$vmid" -- ip route get 1.1.1.1 2>/dev/null </dev/null | awk '{for (i = 1; i < NF; i++) if ($i == "src") { print $(i + 1); exit }}' || echo "")
    fi
    [ -n "$ip_address" ] || ip_address="unknown"
    os_info=$(timeout 5 pct exec "$vmid" -- cat /etc/os-release 2>/dev/null </dev/null | grep "^PRETTY_NAME=" | cut -d'"' -f2 || echo "unknown")
    
    # Detect container architecture
    debug "  Detecting container architecture..."
    arch_raw=$(timeout 5 pct exec "$vmid" -- uname -m 2>/dev/null </dev/null || echo "unknown")
    
    # Map architecture to supported values
    case "$arch_raw" in
        "x86_64")
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
    
    debug "  Detected architecture: $arch_raw -> $architecture"
    
    container_ids=$(timeout 5 pct exec "$vmid" -- sh -c "cat /etc/machine-id 2>/dev/null; echo; cat /var/lib/dbus/machine-id 2>/dev/null" </dev/null 2>/dev/null || true)
    machine_id=$(derive_machine_id "$container_ids" "$vmid" "$node_id")

    friendly_name="${HOST_PREFIX}${hostname}"

    info "  Hostname: $hostname"
    info "  IP Address: $ip_address"
    info "  OS: $os_info"
    info "  Architecture: $architecture ($arch_raw)"
    info "  Machine ID: ${machine_id:0:16}..."

    if [[ "$DRY_RUN" == "true" ]]; then
        info "  [DRY RUN] Would enroll: $friendly_name"
        ((enrolled_count++)) || true
        echo ""
        continue
    fi

    force_new=false
    if force_new_host_for "$vmid"; then
        force_new=true
        warn "  FORCE_NEW_HOST: setting aside this container's PatchMon credentials and host_id, a new host will be created"
        if ! set_aside_dir=$(timeout 10 pct exec "$vmid" -- sh -c "$(declare -f set_aside_identity); set_aside_identity /etc/patchmon" </dev/null 2>/dev/null); then
            error "  ✗ Could not move the existing configuration out of /etc/patchmon in $friendly_name"
            failed_containers["$vmid"]="$friendly_name|FORCE_NEW_HOST could not set aside the old configuration|"
            ((failed_count++)) || true
            echo ""
            sleep 1
            continue
        fi
        set_aside_dir=$(identity_value "dir=$set_aside_dir" dir)
        [[ -n "$set_aside_dir" ]] && info "  Previous configuration moved to $set_aside_dir"
    fi
    put_back_identity() {
        [[ "$force_new" == "true" && -n "$set_aside_dir" ]] || return 0
        timeout 10 pct exec "$vmid" -- sh -c "$(declare -f restore_identity); restore_identity /etc/patchmon \"\$1\"" _ "$set_aside_dir" </dev/null >/dev/null 2>&1 || true
        warn "    No new host was created, so the container's previous configuration was put back"
    }

    # Check if agent is already installed and working BEFORE enrollment
    info "  Checking if agent is already configured..."
    config_check=$(timeout 10 pct exec "$vmid" -- sh -c "
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
    " 2>/dev/null </dev/null || echo "error")

    if [[ "$config_check" == "ping_success" ]]; then
        info "  ✓ Host already enrolled and agent ping successful - skipping enrollment"
        ((skipped_count++)) || true
        echo ""
        continue
    elif [[ "$config_check" == "ping_failed" ]]; then
        warn "  ⚠ Agent configuration exists but ping failed - repairing"
    elif [[ "$config_check" == "binary_missing" ]]; then
        warn "  ⚠ Config exists but agent binary missing - repairing"
    elif [[ "$config_check" == "not_configured" ]]; then
        info "  ℹ Agent not yet configured - proceeding with enrollment"
    else
        warn "  ⚠ Could not check agent status - proceeding with enrollment"
    fi

    install_url="$PATCHMON_URL/api/v1/hosts/install?arch=$architecture"
    if [[ "$FORCE_INSTALL" == "true" ]]; then
        install_url="$install_url&force=true"
    fi

    existing_api_id=""
    existing_api_key=""
    existing_host_id=""
    if [[ "$force_new" != "true" ]]; then
        identity=$(timeout 10 pct exec "$vmid" -- sh -c "$(declare -f patchmon_identity); patchmon_identity /etc/patchmon" 2>/dev/null </dev/null || true)
        if same_server "$(identity_value "$identity" server)" "$PATCHMON_URL"; then
            existing_api_id=$(identity_value "$identity" api_id)
            existing_api_key=$(identity_value "$identity" api_key)
            existing_host_id=$(identity_value "$identity" host_id)
        elif [[ -n "$(identity_value "$identity" api_id)" ]]; then
            warn "  Existing agent configuration points at a different PatchMon server; not reusing its credentials"
        fi
    fi

    api_id=""
    api_key=""
    reused_credentials=false
    if [[ -n "$existing_api_id" && -n "$existing_api_key" ]]; then
        info "  Found existing credentials ($existing_api_id), checking them with PatchMon..."
        cred_code=$(curl $CURL_FLAGS -o /dev/null -w "%{http_code}" \
            -H "X-API-ID: $existing_api_id" \
            -H "X-API-KEY: $existing_api_key" \
            "$install_url" 2>/dev/null) || cred_code="000"
        case "$cred_code" in
            200)
                info "  ✓ Existing credentials are valid - reinstalling the agent with them"
                api_id="$existing_api_id"
                api_key="$existing_api_key"
                reused_credentials=true
                ;;
            401)
                warn "  Existing credentials are no longer valid - enrolling"
                ;;
            *)
                error "  ✗ Could not check existing credentials for $friendly_name (HTTP $cred_code); not enrolling, so a temporary failure cannot create a duplicate"
                failed_containers["$vmid"]="$friendly_name|Credential check failed (HTTP $cred_code)|"
                ((failed_count++)) || true
                echo ""
                sleep 1
                continue
                ;;
        esac
    fi

    if [[ "$reused_credentials" != "true" ]]; then
        # Call PatchMon auto-enrollment API
        info "  Enrolling $friendly_name in PatchMon..."
        if [[ -n "$existing_host_id" ]]; then
            info "  Sending existing host_id $existing_host_id"
        fi

        enroll_payload=$(jq -n \
            --arg friendly_name "$friendly_name" \
            --arg machine_id "$machine_id" \
            --arg host_id "$existing_host_id" \
            --arg vmid "$vmid" \
            --arg proxmox_node "$(hostname)" \
            --arg ip_address "$ip_address" \
            --arg os_info "$os_info" \
            '{friendly_name: $friendly_name, machine_id: $machine_id, metadata: {vmid: $vmid, proxmox_node: $proxmox_node, ip_address: $ip_address, os_info: $os_info}} + (if $host_id != "" then {host_id: $host_id} else {} end)')

        response=$(curl $CURL_FLAGS -X POST \
            -H "X-Auto-Enrollment-Key: $AUTO_ENROLLMENT_KEY" \
            -H "X-Auto-Enrollment-Secret: $AUTO_ENROLLMENT_SECRET" \
            -H "Content-Type: application/json" \
            -d "$enroll_payload" \
            "$PATCHMON_URL/api/v1/auto-enrollment/enroll" \
            -w "\n%{http_code}" 2>&1) || true

        http_code=$(echo "$response" | tail -n 1)
        body=$(echo "$response" | sed '$d')
        existing_name=$(echo "$body" | jq -r '.friendly_name // empty' 2>/dev/null || true)

        case "$(enrol_outcome "$http_code" "$body")" in
            created|reenrolled)
                api_id=$(echo "$body" | jq -r '.host.api_id // empty' 2>/dev/null || echo "")
                api_key=$(echo "$body" | jq -r '.host.api_key // empty' 2>/dev/null || echo "")
                new_host_id=$(echo "$body" | jq -r '.host.id // empty' 2>/dev/null || echo "")

                if [[ -z "$api_id" ]] || [[ -z "$api_key" ]]; then
                    put_back_identity
                    error "  ✗ Failed to parse API credentials from the enrolment response for $friendly_name"
                    failed_containers["$vmid"]="$friendly_name|Could not parse API credentials|$body"
                    ((failed_count++)) || true
                    echo ""
                    sleep 1
                    continue
                fi

                if [[ "$http_code" == "200" ]]; then
                    info "  ✓ Credentials re-issued for the existing host: $api_id"
                else
                    info "  ✓ Host enrolled successfully: $api_id"
                fi
                timeout 10 pct exec "$vmid" -- sh -c "$(declare -f write_stub_config); write_stub_config \"\$@\"" _ /etc/patchmon "$PATCHMON_URL" "$new_host_id" </dev/null >/dev/null 2>&1 || true
                ;;
            host_reporting)
                put_back_identity
                warn "  ⊘ $friendly_name is already reporting to PatchMon${existing_name:+ as $existing_name}; skipping"
                warn "    Usually a cloned container, or credentials regenerated in PatchMon but not applied. Reinstall with the host's install command, or set FORCE_NEW_HOST=true to enrol it as a separate host."
                ((skipped_count++)) || true
                echo ""
                sleep 1
                continue
                ;;
            host_not_reenrollable)
                put_back_identity
                warn "  ⊘ $friendly_name was added to PatchMon another way${existing_name:+ as $existing_name}; skipping"
                warn "    Use the install command on the host's page in PatchMon."
                ((skipped_count++)) || true
                echo ""
                sleep 1
                continue
                ;;
            already_enrolled)
                put_back_identity
                if [[ "$force_new" == "true" ]]; then
                    warn "  ⊘ FORCE_NEW_HOST: PatchMon already has a host with this container's machine ID and name${existing_name:+ ($existing_name)}: the container it was cloned from, or a host an earlier FORCE_NEW_HOST run already created for it. Do not remove it. If it is the original, give this container its own /etc/machine-id or a HOST_PREFIX; if it is this container's new host, rerun without FORCE_NEW_HOST"
                else
                    warn "  ⊘ $friendly_name is already enrolled in PatchMon${existing_name:+ as $existing_name}; skipping"
                    warn "    Remove the host in PatchMon before it can be enrolled again"
                fi
                ((skipped_count++)) || true
                echo ""
                sleep 1
                continue
                ;;
            retry_later)
                put_back_identity
                error "  ✗ PatchMon asked to retry later (HTTP 429) for $friendly_name"
                failed_containers["$vmid"]="$friendly_name|HTTP 429, retry on the next run|$body"
                ((failed_count++)) || true
                echo ""
                sleep 1
                continue
                ;;
            *)
                put_back_identity
                error "  ✗ Failed to enroll $friendly_name - HTTP $http_code"
                debug "  Response: $body"
                failed_containers["$vmid"]="$friendly_name|HTTP $http_code enrollment failed|$body"
                ((failed_count++)) || true
                echo ""
                sleep 1
                continue
                ;;
        esac
    fi

    # Ensure curl is installed in the container
    info "  Checking for curl in container..."
    curl_check=$(timeout 10 pct exec "$vmid" -- sh -c "command -v curl >/dev/null 2>&1 && echo 'installed' || echo 'missing'" 2>/dev/null </dev/null || echo "error")
    
    if [[ "$curl_check" == "missing" ]]; then
        info "  Installing curl in container..."
        
        # Detect package manager and install curl
        curl_install_rc=0
        curl_install_output=$(timeout 60 pct exec "$vmid" -- sh -c "
            if command -v apt-get >/dev/null 2>&1; then
                export DEBIAN_FRONTEND=noninteractive
                apt-get update -qq && apt-get install -y -qq curl
            elif command -v yum >/dev/null 2>&1; then
                yum install -y -q curl
            elif command -v dnf >/dev/null 2>&1; then
                dnf install -y -q curl
            elif command -v apk >/dev/null 2>&1; then
                apk add --no-cache curl
            else
                echo 'ERROR: No supported package manager found'
                exit 1
            fi
        " 2>&1 </dev/null) || curl_install_rc=$?
        
        if [[ $curl_install_rc -ne 0 ]]; then
            if [[ "$curl_install_output" == *"ERROR: No supported package manager"* ]]; then
                warn "  ✗ Could not install curl - no supported package manager found"
                failed_containers["$vmid"]="$friendly_name|No package manager for curl|$curl_install_output"
            else
                warn "  ✗ Could not install curl in $friendly_name (exit: $curl_install_rc)"
                failed_containers["$vmid"]="$friendly_name|curl installation failed (exit $curl_install_rc)|$curl_install_output"
            fi
            ((failed_count++)) || true
            echo ""
            sleep 1
            continue
        else
            info "  ✓ curl installed successfully"
        fi
    else
        info "  ✓ curl already installed"
    fi

    # Install PatchMon agent in container
    info "  Installing PatchMon agent..."
    
    if [[ "$FORCE_INSTALL" == "true" ]]; then
        info "  Using force mode - will bypass broken packages"
    fi
    info "  Using architecture: $architecture"
    
    # Reset exit code for this container
    install_exit_code=0
    
    # Download and execute in separate steps to avoid stdin issues with piping
    # Pass CURL_FLAGS as environment variable to container
    # Use sh -c for POSIX compatibility (Alpine uses ash, not bash)
    install_output=$(timeout 180 pct exec "$vmid" -- sh -c "
        export CURL_FLAGS='$CURL_FLAGS'
        cd /tmp
        curl \$CURL_FLAGS \
            -H \"X-API-ID: $api_id\" \
            -H \"X-API-KEY: $api_key\" \
            -o patchmon-install.sh \
            '$install_url' && \
        sh patchmon-install.sh && \
        rm -f patchmon-install.sh
    " 2>&1 </dev/null) || install_exit_code=$?

    # Check both exit code AND success message in output for reliability
    if [[ $install_exit_code -eq 0 ]] || [[ "$install_output" == *"PatchMon Agent installation completed successfully"* ]]; then
        if [[ "$reused_credentials" == "true" ]]; then
            info "  ✓ Agent repaired with its existing credentials in $friendly_name"
        else
            info "  ✓ Agent installed successfully in $friendly_name"
        fi
        ((enrolled_count++)) || true
    elif [[ $install_exit_code -eq 124 ]]; then
        warn "  ⏱ Agent installation timed out (>180s) in $friendly_name"
        info "  Install output: $install_output"
        # Store failure details
        failed_containers["$vmid"]="$friendly_name|Timeout (>180s)|$install_output"
        ((failed_count++)) || true
    else
        # Check if it's a dpkg error
        if [[ "$install_output" == *"dpkg was interrupted"* ]] || [[ "$install_output" == *"dpkg --configure -a"* ]]; then
            warn "  ⚠ Failed due to dpkg error in $friendly_name (can be fixed)"
            dpkg_error_containers["$vmid"]="$friendly_name:$api_id:$api_key"
            # Store failure details
            failed_containers["$vmid"]="$friendly_name|dpkg error|$install_output"
        else
            warn "  ✗ Failed to install agent in $friendly_name (exit: $install_exit_code)"
            # Store failure details
            failed_containers["$vmid"]="$friendly_name|Exit code $install_exit_code|$install_output"
        fi
        info "  Install output: $install_output"
        ((failed_count++)) || true
    fi

    echo ""
    sleep 1  # Rate limiting between containers

done <<< "$lxc_list"

# ===== SUMMARY =====
echo ""
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║                     ENROLLMENT SUMMARY                        ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo ""
info "Total Containers Found: $total_containers"
info "Successfully Enrolled:  $enrolled_count"
info "Skipped:                $skipped_count"
info "Failed:                 $failed_count"
echo ""

# ===== FAILURE DETAILS =====
if [[ ${#failed_containers[@]} -gt 0 ]]; then
    echo "╔═══════════════════════════════════════════════════════════════╗"
    echo "║                     FAILURE DETAILS                           ║"
    echo "╚═══════════════════════════════════════════════════════════════╝"
    echo ""
    
    for vmid in "${!failed_containers[@]}"; do
        entry="${failed_containers[$vmid]}"
        name="${entry%%|*}"
        entry="${entry#*|}"
        reason="${entry%%|*}"
        output="${entry#*|}"
        
        warn "Container $vmid: $name"
        info "  Reason: $reason"
        info "  Last 5 lines of output:"
        
        # Get last 5 lines of output
        last_5_lines=$(echo "$output" | tail -n 5)
        
        # Display each line with proper indentation
        while IFS= read -r line; do
            echo "    $line"
        done <<< "$last_5_lines"
        
        echo ""
    done
fi

if [[ "$DRY_RUN" == "true" ]]; then
    warn "This was a DRY RUN - no actual changes were made"
    warn "Set DRY_RUN=false to perform actual enrollment"
fi

# ===== DPKG ERROR RECOVERY =====
if [[ ${#dpkg_error_containers[@]} -gt 0 ]]; then
    echo ""
    echo "╔═══════════════════════════════════════════════════════════════╗"
    echo "║              DPKG ERROR RECOVERY AVAILABLE                    ║"
    echo "╚═══════════════════════════════════════════════════════════════╝"
    echo ""
    warn "Detected ${#dpkg_error_containers[@]} container(s) with dpkg errors:"
    for vmid in "${!dpkg_error_containers[@]}"; do
        IFS=':' read -r name api_id api_key <<< "${dpkg_error_containers[$vmid]}"
        info "  • Container $vmid: $name"
    done
    echo ""
    
    if ask_yes_no "Would you like to fix dpkg errors and retry installation? (y/N): "; then
        echo ""
        info "Starting dpkg recovery process..."
        echo ""
        
        recovered_count=0
        
        for vmid in "${!dpkg_error_containers[@]}"; do
            IFS=':' read -r name api_id api_key <<< "${dpkg_error_containers[$vmid]}"
            
            info "Fixing dpkg in container $vmid ($name)..."
            
            # Run dpkg --configure -a
            dpkg_rc=0
            dpkg_output=$(timeout 60 pct exec "$vmid" -- dpkg --configure -a 2>&1 </dev/null) || dpkg_rc=$?
            
            if [[ $dpkg_rc -eq 0 ]]; then
                info "  ✓ dpkg fixed successfully"
                
                # Retry agent installation
                info "  Retrying agent installation..."
                
                install_exit_code=0
                # Pass CURL_FLAGS as environment variable to container
                install_output=$(timeout 180 pct exec "$vmid" -- sh -c "
                    export CURL_FLAGS='$CURL_FLAGS'
                    cd /tmp
                    curl \$CURL_FLAGS \
                        -H \"X-API-ID: $api_id\" \
                        -H \"X-API-KEY: $api_key\" \
                        -o patchmon-install.sh \
                        '$PATCHMON_URL/api/v1/hosts/install?arch=$architecture' && \
                    sh patchmon-install.sh && \
                    rm -f patchmon-install.sh
                " 2>&1 </dev/null) || install_exit_code=$?
                
                if [[ $install_exit_code -eq 0 ]] || [[ "$install_output" == *"PatchMon Agent installation completed successfully"* ]]; then
                    info "  ✓ Agent installed successfully in $name"
                    ((recovered_count++)) || true
                    ((enrolled_count++)) || true
                    ((failed_count--)) || true
                else
                    warn "  ✗ Agent installation still failed (exit: $install_exit_code)"
                fi
            else
                warn "  ✗ Failed to fix dpkg in $name (exit: $dpkg_rc)"
                info "  dpkg output: $dpkg_output"
            fi
            
            echo ""
        done
        
        echo ""
        info "Recovery complete: $recovered_count container(s) recovered"
        echo ""
    fi
fi

if [[ $failed_count -gt 0 ]]; then
    warn "Some containers failed to enroll. Check the logs above for details."
    exit 1
fi

info "Auto-enrollment complete! ✓"
exit 0
