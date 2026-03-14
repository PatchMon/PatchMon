#!/bin/bash
set -eo pipefail  # Exit on error, pipe failures (removed -u as we handle unset vars explicitly)

# Trap to catch errors only (not normal exits)
trap 'echo "[ERROR] Script failed at line $LINENO with exit code $?"' ERR

SCRIPT_VERSION="2.0.0"
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
SKIP_STOPPED="${SKIP_STOPPED:-true}"
PARALLEL_INSTALL="${PARALLEL_INSTALL:-false}"
MAX_PARALLEL="${MAX_PARALLEL:-5}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"

# ===== COLOR OUTPUT =====
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ===== LOGGING FUNCTIONS =====
info() { echo -e "${GREEN}[INFO]${NC} $1"; return 0; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; return 0; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; return 0; }
debug() { [[ "${DEBUG:-false}" == "true" ]] && echo -e "${BLUE}[DEBUG]${NC} $1" || true; return 0; }

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
    error "AUTO_ENROLLMENT_KEY and AUTO_ENROLLMENT_SECRET must be set"
fi

if [[ -z "$PATCHMON_URL" ]]; then
    error "PATCHMON_URL must be set"
fi

# Check if running on Proxmox
if ! command -v pct &> /dev/null; then
    error "This script must run on a Proxmox host (pct command not found)"
fi

# Check for required commands
for cmd in curl jq; do
    if ! command -v $cmd &> /dev/null; then
        error "Required command '$cmd' not found. Please install it first."
    fi
done

info "Configuration validated successfully"
info "PatchMon Server: $PATCHMON_URL"
info "Dry Run Mode: $DRY_RUN"
info "Skip Stopped Containers: $SKIP_STOPPED"
echo ""

# ===== DISCOVER LXC CONTAINERS =====
info "Discovering LXC containers..."
lxc_list=$(pct list | tail -n +2)  # Skip header

if [[ -z "$lxc_list" ]]; then
    warn "No LXC containers found on this Proxmox host"
    exit 0
fi

# Count containers
total_containers=$(echo "$lxc_list" | wc -l)
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

# ===== PROCESS CONTAINERS =====
info "Starting container processing loop..."
while IFS= read -r line; do
    info "[DEBUG] Read line from lxc_list"
    vmid=$(echo "$line" | awk '{print $1}')
    status=$(echo "$line" | awk '{print $2}')
    name=$(echo "$line" | awk '{print $3}')

    info "Processing LXC $vmid: $name (status: $status)"

    # Skip stopped containers if configured
    if [[ "$status" != "running" ]] && [[ "$SKIP_STOPPED" == "true" ]]; then
        warn "  Skipping $name - container not running"
        ((skipped_count++)) || true
        echo ""
        continue
    fi

    # Check if container is stopped
    if [[ "$status" != "running" ]]; then
        warn "  Container $name is stopped - cannot gather info or install agent"
        ((skipped_count++)) || true
        echo ""
        continue
    fi

    # Get container details
    debug "  Gathering container information..."
    hostname=$(timeout 5 pct exec "$vmid" -- hostname 2>/dev/null </dev/null || echo "$name")
    ip_address=$(timeout 5 pct exec "$vmid" -- hostname -I 2>/dev/null </dev/null | awk '{print $1}' || echo "unknown")
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
    
    # Get machine ID from container (use sh -c for POSIX compatibility with Alpine/ash)
    machine_id=$(timeout 5 pct exec "$vmid" -- sh -c "cat /etc/machine-id 2>/dev/null || cat /var/lib/dbus/machine-id 2>/dev/null || echo 'proxmox-lxc-$vmid-'$(cat /proc/sys/kernel/random/uuid)" </dev/null 2>/dev/null || echo "proxmox-lxc-$vmid-unknown")

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
        warn "  ⚠ Agent configuration exists but ping failed - will re-enroll and reinstall"
    elif [[ "$config_check" == "binary_missing" ]]; then
        warn "  ⚠ Config exists but agent binary missing - will re-enroll and reinstall"
    elif [[ "$config_check" == "not_configured" ]]; then
        info "  ℹ Agent not yet configured - proceeding with enrollment"
    else
        warn "  ⚠ Could not check agent status - proceeding with enrollment"
    fi

    # Call PatchMon auto-enrollment API
    info "  Enrolling $friendly_name in PatchMon..."
    
    response=$(curl $CURL_FLAGS -X POST \
        -H "X-Auto-Enrollment-Key: $AUTO_ENROLLMENT_KEY" \
        -H "X-Auto-Enrollment-Secret: $AUTO_ENROLLMENT_SECRET" \
        -H "Content-Type: application/json" \
        -d "{
            \"friendly_name\": \"$friendly_name\",
            \"machine_id\": \"$machine_id\",
            \"metadata\": {
                \"vmid\": \"$vmid\",
                \"proxmox_node\": \"$(hostname)\",
                \"ip_address\": \"$ip_address\",
                \"os_info\": \"$os_info\"
            }
        }" \
        "$PATCHMON_URL/api/v1/auto-enrollment/enroll" \
        -w "\n%{http_code}" 2>&1)

    http_code=$(echo "$response" | tail -n 1)
    body=$(echo "$response" | sed '$d')

    if [[ "$http_code" == "201" ]]; then
        api_id=$(echo "$body" | jq -r '.host.api_id' 2>/dev/null || echo "")
        api_key=$(echo "$body" | jq -r '.host.api_key' 2>/dev/null || echo "")

        if [[ -z "$api_id" ]] || [[ -z "$api_key" ]]; then
            error "  Failed to parse API credentials from response"
        fi

        info "  ✓ Host enrolled successfully: $api_id"

        # Ensure curl is installed in the container
        info "  Checking for curl in container..."
        curl_check=$(timeout 10 pct exec "$vmid" -- sh -c "command -v curl >/dev/null 2>&1 && echo 'installed' || echo 'missing'" 2>/dev/null </dev/null || echo "error")
        
        if [[ "$curl_check" == "missing" ]]; then
            info "  Installing curl in container..."
            
            # Detect package manager and install curl
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
            " 2>&1 </dev/null) || true
            
            if [[ "$curl_install_output" == *"ERROR: No supported package manager"* ]]; then
                warn "  ✗ Could not install curl - no supported package manager found"
                failed_containers["$vmid"]="$friendly_name|No package manager for curl|$curl_install_output"
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
        
        # Build install URL with force flag and architecture if enabled
        install_url="$PATCHMON_URL/api/v1/hosts/install?arch=$architecture"
        if [[ "$FORCE_INSTALL" == "true" ]]; then
            install_url="$install_url&force=true"
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
            info "  ✓ Agent installed successfully in $friendly_name"
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

    elif [[ "$http_code" == "409" ]]; then
        warn "  ⊘ Host $friendly_name already enrolled - skipping"
        ((skipped_count++)) || true
    elif [[ "$http_code" == "429" ]]; then
        error "  ✗ Rate limit exceeded - maximum hosts per day reached"
        failed_containers["$vmid"]="$friendly_name|Rate limit exceeded|$body"
        ((failed_count++)) || true
    else
        error "  ✗ Failed to enroll $friendly_name - HTTP $http_code"
        debug "  Response: $body"
        failed_containers["$vmid"]="$friendly_name|HTTP $http_code enrollment failed|$body"
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
        IFS='|' read -r name reason output <<< "${failed_containers[$vmid]}"
        
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
    
    # Ask user if they want to fix dpkg errors
    read -p "Would you like to fix dpkg errors and retry installation? (y/N): " -n 1 -r
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo ""
        info "Starting dpkg recovery process..."
        echo ""
        
        recovered_count=0
        
        for vmid in "${!dpkg_error_containers[@]}"; do
            IFS=':' read -r name api_id api_key <<< "${dpkg_error_containers[$vmid]}"
            
            info "Fixing dpkg in container $vmid ($name)..."
            
            # Run dpkg --configure -a
            dpkg_output=$(timeout 60 pct exec "$vmid" -- dpkg --configure -a 2>&1 </dev/null || true)
            
            if [[ $? -eq 0 ]]; then
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
                warn "  ✗ Failed to fix dpkg in $name"
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

