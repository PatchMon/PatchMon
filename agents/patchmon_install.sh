#!/bin/sh
# PatchMon Agent Installation Script
# POSIX-compliant shell script (works with dash, ash, bash, etc.)
# Usage: curl -s {PATCHMON_URL}/api/v1/hosts/install -H "X-API-ID: {API_ID}" -H "X-API-KEY: {API_KEY}" | sh

set -e

# This placeholder will be dynamically replaced by the server when serving this
# script based on the "ignore SSL self-signed" setting. If set to -k, curl will
# ignore certificate validation. Otherwise, it will be empty for secure default.
# CURL_FLAGS is now set via environment variables by the backend

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Functions
error() {
    printf "%b\n" "${RED}ERROR: $1${NC}" >&2
    exit 1
}

info() {
    printf "%b\n" "${BLUE}INFO: $1${NC}"
}

success() {
    printf "%b\n" "${GREEN}SUCCESS: $1${NC}"
}

warning() {
    printf "%b\n" "${YELLOW}WARNING: $1${NC}"
}

# Check if running as root
if [ "$(id -u)" -ne 0 ]; then
   error "This script must be run as root (use sudo)"
fi

# Verify system datetime and timezone
verify_datetime() {
    info "Verifying system datetime and timezone..."
    
    # Get current system time
    system_time=$(date)
    timezone=$(timedatectl show --property=Timezone --value 2>/dev/null || echo "Unknown")
    
    # Display current datetime info
    echo ""
    printf "%b\n" "${BLUE}Current System Date/Time:${NC}"
    echo "   • Date/Time: $system_time"
    echo "   • Timezone: $timezone"
    echo ""
    
    # Check if we can read from stdin (interactive terminal)
    if [ -t 0 ]; then
        # Interactive terminal - ask user
        printf "Does this date/time look correct to you? (y/N): "
        read -r response
        case "$response" in
            [Yy]*) 
                success "Date/time verification passed"
                echo ""
                return 0
                ;;
            *)
            echo ""
            printf "%b\n" "${RED}Date/time verification failed${NC}"
            echo ""
            printf "%b\n" "${YELLOW}Please fix the date/time and re-run the installation script:${NC}"
            echo "   sudo timedatectl set-time 'YYYY-MM-DD HH:MM:SS'"
            echo "   sudo timedatectl set-timezone 'America/New_York'  # or your timezone"
            echo "   sudo timedatectl list-timezones  # to see available timezones"
            echo ""
                printf "%b\n" "${BLUE}After fixing the date/time, re-run this installation script.${NC}"
                error "Installation cancelled - please fix date/time and re-run"
                ;;
        esac
    else
        # Non-interactive (piped from curl) - show warning and continue
        printf "%b\n" "${YELLOW}Non-interactive installation detected${NC}"
        echo ""
        echo "Please verify the date/time shown above is correct."
        echo "If the date/time is incorrect, it may cause issues with:"
        echo "   • Logging timestamps"
        echo "   • Scheduled updates"
        echo "   • Data synchronization"
        echo ""
        printf "%b\n" "${GREEN}Continuing with installation...${NC}"
        success "Date/time verification completed (assumed correct)"
        echo ""
    fi
}

# Run datetime verification
verify_datetime

# Clean up old files (keep only last 3 of each type)
cleanup_old_files() {
    # Clean up old credential backups
    ls -t /etc/patchmon/credentials.yml.backup.* 2>/dev/null | tail -n +4 | xargs -r rm -f
    
    # Clean up old config backups
    ls -t /etc/patchmon/config.yml.backup.* 2>/dev/null | tail -n +4 | xargs -r rm -f
    
    # Clean up old agent backups
    ls -t /usr/local/bin/patchmon-agent.backup.* 2>/dev/null | tail -n +4 | xargs -r rm -f
    
    # Clean up old log files
    ls -t /etc/patchmon/logs/patchmon-agent.log.old.* 2>/dev/null | tail -n +4 | xargs -r rm -f
    
    # Clean up old shell script backups (if any exist)
    ls -t /usr/local/bin/patchmon-agent.sh.backup.* 2>/dev/null | tail -n +4 | xargs -r rm -f
    
    # Clean up old credentials backups (if any exist)
    ls -t /etc/patchmon/credentials.backup.* 2>/dev/null | tail -n +4 | xargs -r rm -f
}

# Run cleanup at start
cleanup_old_files

# Generate or retrieve machine ID
get_machine_id() {
    # FreeBSD: use hostid or kern.hostuuid
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
    # Linux: try multiple sources
    if [ -f /etc/machine-id ]; then
        cat /etc/machine-id
    elif [ -f /var/lib/dbus/machine-id ]; then
        cat /var/lib/dbus/machine-id
    else
        # Fallback: generate from hardware info (less ideal but works)
        echo "patchmon-$(cat /sys/class/dmi/id/product_uuid 2>/dev/null || cat /proc/sys/kernel/random/uuid 2>/dev/null)"
    fi
}

# Parse arguments from environment (passed via HTTP headers)
if [ -z "$PATCHMON_URL" ] || [ -z "$API_ID" ] || [ -z "$API_KEY" ]; then
    error "Missing required parameters. This script should be called via the PatchMon web interface."
fi

# Default PATCHMON_OS to linux if not set (backward compatibility when os param not in URL)
PATCHMON_OS="${PATCHMON_OS:-linux}"

# Auto-detect architecture if not explicitly set
if [ -z "$ARCHITECTURE" ]; then
    arch_raw=$(uname -m 2>/dev/null || echo "unknown")
    
    # Map architecture to supported values
    case "$arch_raw" in
        "x86_64")
            ARCHITECTURE="amd64"
            ;;
        "i386"|"i686")
            ARCHITECTURE="386"
            ;;
        "aarch64"|"arm64")
            ARCHITECTURE="arm64"
            ;;
        "armv7l"|"armv6l"|"arm")
            ARCHITECTURE="arm"
            ;;
        *)
            warning "Unknown architecture '$arch_raw', defaulting to amd64"
            ARCHITECTURE="amd64"
            ;;
    esac
fi

# Validate architecture (FreeBSD supports amd64 and arm64 only)
if [ "$PATCHMON_OS" = "freebsd" ]; then
    if [ "$ARCHITECTURE" != "amd64" ] && [ "$ARCHITECTURE" != "arm64" ]; then
        error "Invalid architecture '$ARCHITECTURE' for FreeBSD. Must be one of: amd64, arm64"
    fi
else
    if [ "$ARCHITECTURE" != "amd64" ] && [ "$ARCHITECTURE" != "386" ] && [ "$ARCHITECTURE" != "arm64" ] && [ "$ARCHITECTURE" != "arm" ]; then
        error "Invalid architecture '$ARCHITECTURE'. Must be one of: amd64, 386, arm64, arm"
    fi
fi

# Check if --force flag is set (for bypassing broken packages)
FORCE_INSTALL="${FORCE_INSTALL:-false}"
case "$*" in
    *"--force"*) FORCE_INSTALL="true" ;;
esac
if [ "$FORCE_INSTALL" = "true" ]; then
    FORCE_INSTALL="true"
    warning "Force mode enabled - will bypass broken packages"
fi

# Get unique machine ID for this host
MACHINE_ID=$(get_machine_id)
export MACHINE_ID

info "Starting PatchMon Agent Installation..."
info "Server: $PATCHMON_URL"
info "API ID: $(echo "$API_ID" | cut -c1-16)..."
info "Machine ID: $(echo "$MACHINE_ID" | cut -c1-16)..."
info "Architecture: $ARCHITECTURE"
info "Platform: $PATCHMON_OS"

# Install required dependencies
info "Installing required dependencies..."
echo ""

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to install packages with error handling
install_apt_packages() {
    # Space-separated list of packages
    _packages="$*"
    _missing_packages=""
    
    # Check which packages are missing
    for pkg in $_packages; do
        if ! command_exists "$pkg"; then
            _missing_packages="$_missing_packages $pkg"
        fi
    done
    
    # Trim leading space
    _missing_packages=$(echo "$_missing_packages" | sed 's/^ //')
    
    if [ -z "$_missing_packages" ]; then
        success "All required packages are already installed"
        return 0
    fi
    
    info "Need to install: $_missing_packages"
    
    # Build apt-get command based on force mode
    _apt_cmd="apt-get install $_missing_packages -y"
    
    if [ "$FORCE_INSTALL" = "true" ]; then
        info "Using force mode - bypassing broken packages..."
        _apt_cmd="$_apt_cmd -o APT::Get::Fix-Broken=false -o DPkg::Options::=\"--force-confold\" -o DPkg::Options::=\"--force-confdef\""
    fi
    
    # Try to install packages
    if eval "$_apt_cmd" 2>&1 | tee /tmp/patchmon_apt_install.log; then
        success "Packages installed successfully"
        return 0
    else
        warning "Package installation encountered issues, checking if required tools are available..."
        
        # Verify critical dependencies are actually available
        _all_ok=true
        for pkg in $_packages; do
            if ! command_exists "$pkg"; then
                if [ "$FORCE_INSTALL" = "true" ]; then
                    error "Critical dependency '$pkg' is not available even with --force. Please install manually."
                else
                    error "Critical dependency '$pkg' is not available. Try again with --force flag or install manually: apt-get install $pkg"
                fi
                _all_ok=false
            fi
        done
        
        if $_all_ok; then
            success "All required tools are available despite installation warnings"
            return 0
        else
            return 1
        fi
    fi
}

# Function to check and install packages for yum/dnf
install_yum_dnf_packages() {
    _pkg_manager="$1"
    shift
    _packages="$*"
    _missing_packages=""
    
    # Check which packages are missing
    for pkg in $_packages; do
        if ! command_exists "$pkg"; then
            _missing_packages="$_missing_packages $pkg"
        fi
    done
    
    # Trim leading space
    _missing_packages=$(echo "$_missing_packages" | sed 's/^ //')
    
    if [ -z "$_missing_packages" ]; then
        success "All required packages are already installed"
        return 0
    fi
    
    info "Need to install: $_missing_packages"
    
    if [ "$_pkg_manager" = "yum" ]; then
        yum install -y $_missing_packages
    else
        dnf install -y $_missing_packages
    fi
}

# Function to check and install packages for zypper
install_zypper_packages() {
    _packages="$*"
    _missing_packages=""
    
    # Check which packages are missing
    for pkg in $_packages; do
        if ! command_exists "$pkg"; then
            _missing_packages="$_missing_packages $pkg"
        fi
    done
    
    # Trim leading space
    _missing_packages=$(echo "$_missing_packages" | sed 's/^ //')
    
    if [ -z "$_missing_packages" ]; then
        success "All required packages are already installed"
        return 0
    fi
    
    info "Need to install: $_missing_packages"
    zypper install -y $_missing_packages
}

# Function to check and install packages for pacman
install_pacman_packages() {
    _packages="$*"
    _missing_packages=""
    
    # Check which packages are missing
    for pkg in $_packages; do
        if ! command_exists "$pkg"; then
            _missing_packages="$_missing_packages $pkg"
        fi
    done
    
    # Trim leading space
    _missing_packages=$(echo "$_missing_packages" | sed 's/^ //')
    
    if [ -z "$_missing_packages" ]; then
        success "All required packages are already installed"
        return 0
    fi
    
    info "Need to install: $_missing_packages"
    pacman -S --noconfirm $_missing_packages
}

# Function to check and install packages for apk
install_apk_packages() {
    _packages="$*"
    _missing_packages=""
    
    # Check which packages are missing
    for pkg in $_packages; do
        if ! command_exists "$pkg"; then
            _missing_packages="$_missing_packages $pkg"
        fi
    done
    
    # Trim leading space
    _missing_packages=$(echo "$_missing_packages" | sed 's/^ //')
    
    if [ -z "$_missing_packages" ]; then
        success "All required packages are already installed"
        return 0
    fi
    
    info "Need to install: $_missing_packages"
    
    # Update package index before installation
    info "Updating package index..."
    apk update -q || true
    
    # Build apk command
    _apk_cmd="apk add --no-cache $_missing_packages"
    
    # Try to install packages
    if eval "$_apk_cmd" 2>&1 | tee /tmp/patchmon_apk_install.log; then
        success "Packages installed successfully"
        return 0
    else
        warning "Package installation encountered issues, checking if required tools are available..."
        
        # Verify critical dependencies are actually available
        _all_ok=true
        for pkg in $_packages; do
            if ! command_exists "$pkg"; then
                if [ "$FORCE_INSTALL" = "true" ]; then
                    error "Critical dependency '$pkg' is not available even with --force. Please install manually."
                else
                    error "Critical dependency '$pkg' is not available. Try again with --force flag or install manually: apk add $pkg"
                fi
                _all_ok=false
            fi
        done
        
        if $_all_ok; then
            success "All required tools are available despite installation warnings"
            return 0
        else
            return 1
        fi
    fi
}

# Detect package manager and install jq, curl, and bc
if command -v apt-get >/dev/null 2>&1; then
    # Debian/Ubuntu
    info "Detected apt-get (Debian/Ubuntu)"
    echo ""
    
    # Check for broken packages
    if dpkg -l | grep -q "^iH\|^iF" 2>/dev/null; then
        if [ "$FORCE_INSTALL" = "true" ]; then
            warning "Detected broken packages on system - force mode will work around them"
        else
            warning "Broken packages detected on system"
            warning "If installation fails, retry with: curl -s {URL}/api/v1/hosts/install --force -H ..."
        fi
    fi
    
    info "Updating package lists..."
    apt-get update || true
    echo ""
    info "Installing jq, curl, and bc..."
    install_apt_packages jq curl bc
elif command -v yum >/dev/null 2>&1; then
    # CentOS/RHEL 7
    info "Detected yum (CentOS/RHEL 7)"
    echo ""
    info "Installing jq, curl, and bc..."
    install_yum_dnf_packages yum jq curl bc
elif command -v dnf >/dev/null 2>&1; then
    # CentOS/RHEL 8+/Fedora
    info "Detected dnf (CentOS/RHEL 8+/Fedora)"
    echo ""
    info "Installing jq, curl, and bc..."
    install_yum_dnf_packages dnf jq curl bc
elif command -v zypper >/dev/null 2>&1; then
    # openSUSE
    info "Detected zypper (openSUSE)"
    echo ""
    info "Installing jq, curl, and bc..."
    install_zypper_packages jq curl bc
elif command -v pacman >/dev/null 2>&1; then
    # Arch Linux
    info "Detected pacman (Arch Linux)"
    echo ""
    info "Installing jq, curl, and bc..."
    install_pacman_packages jq curl bc
elif command -v apk >/dev/null 2>&1; then
    # Alpine Linux
    info "Detected apk (Alpine Linux)"
    echo ""
    info "Installing jq, curl, and bc..."
    install_apk_packages jq curl bc
elif [ "$(uname -s 2>/dev/null)" = "FreeBSD" ] || [ "$PATCHMON_OS" = "freebsd" ]; then
    # FreeBSD/pfSense: only curl is required; agent does not use jq/bc. Skip pkg if curl already present
    # (on pfSense, pkg repos may be unconfigured and "pkg install curl" can fail with "no match")
    info "Detected FreeBSD (pkg)"
    echo ""
    if ! command -v curl >/dev/null 2>&1; then
        info "Ensuring curl is installed..."
        if command -v pkg >/dev/null 2>&1; then
            pkg install -y curl >/dev/null 2>&1 || true
        fi
        if ! command -v curl >/dev/null 2>&1; then
            warning "curl not found. On FreeBSD: pkg install curl"
            warning "On pfSense: install curl from System > Package Manager, or enable FreeBSD pkg repos."
        fi
    fi
else
    warning "Could not detect package manager. Please ensure 'jq', 'curl', and 'bc' are installed manually."
fi

echo ""
success "Dependencies installation completed"
echo ""

# Step 1: Handle existing configuration directory
info "Setting up configuration directory..."

# Check if configuration directory already exists
if [ -d "/etc/patchmon" ]; then
    warning "Configuration directory already exists at /etc/patchmon"
    warning "Preserving existing configuration files"
    
    # List existing files for user awareness
    info "Existing files in /etc/patchmon:"
    ls -la /etc/patchmon/ 2>/dev/null | grep -v "^total" | while read -r line; do
        echo "   $line"
    done
else
    info "Creating new configuration directory..."
    mkdir -p /etc/patchmon
fi

# Check if agent is already configured and working (before we overwrite anything)
info "Checking if agent is already configured..."

if [ -f /etc/patchmon/config.yml ] && [ -f /etc/patchmon/credentials.yml ]; then
    if [ -f /usr/local/bin/patchmon-agent ]; then
        info "Found existing agent configuration"
        info "Testing existing configuration with ping..."
        
        if /usr/local/bin/patchmon-agent ping >/dev/null 2>&1; then
            success "Agent is already configured and ping successful"
            info "Existing configuration is working - skipping installation"
            info ""
            info "If you want to reinstall, remove the configuration files first:"
            info "  sudo rm -f /etc/patchmon/config.yml /etc/patchmon/credentials.yml"
            echo ""
            exit 0
        else
            warning "Agent configuration exists but ping failed"
            warning "Will move existing configuration and reinstall"
            echo ""
        fi
    else
        warning "Configuration files exist but agent binary is missing"
        warning "Will move existing configuration and reinstall"
        echo ""
    fi
else
    success "Agent not yet configured - proceeding with installation"
    echo ""
fi

# Step 2: Create configuration files
info "Creating configuration files..."

# Check if config file already exists
if [ -f "/etc/patchmon/config.yml" ]; then
    warning "Config file already exists at /etc/patchmon/config.yml"
    warning "Moving existing file out of the way for fresh installation"
    
    # Clean up old config backups (keep only last 3)
    ls -t /etc/patchmon/config.yml.backup.* 2>/dev/null | tail -n +4 | xargs -r rm -f
    
    # Move existing file out of the way
    mv /etc/patchmon/config.yml /etc/patchmon/config.yml.backup.$(date +%Y%m%d_%H%M%S)
    info "Moved existing config to: /etc/patchmon/config.yml.backup.$(date +%Y%m%d_%H%M%S)"
fi

# Check if credentials file already exists
if [ -f "/etc/patchmon/credentials.yml" ]; then
    warning "Credentials file already exists at /etc/patchmon/credentials.yml"
    warning "Moving existing file out of the way for fresh installation"
    
    # Clean up old credential backups (keep only last 3)
    ls -t /etc/patchmon/credentials.yml.backup.* 2>/dev/null | tail -n +4 | xargs -r rm -f
    
    # Move existing file out of the way
    mv /etc/patchmon/credentials.yml /etc/patchmon/credentials.yml.backup.$(date +%Y%m%d_%H%M%S)
    info "Moved existing credentials to: /etc/patchmon/credentials.yml.backup.$(date +%Y%m%d_%H%M%S)"
fi

# Clean up old credentials file if it exists (from previous installations)
if [ -f "/etc/patchmon/credentials" ]; then
    warning "Found old credentials file, removing it..."
    rm -f /etc/patchmon/credentials
    info "Removed old credentials file"
fi

# Create main config file
cat > /etc/patchmon/config.yml << EOF
# PatchMon Agent Configuration
# Generated on $(date)
patchmon_server: "$PATCHMON_URL"
api_version: "v1"
credentials_file: "/etc/patchmon/credentials.yml"
log_file: "/etc/patchmon/logs/patchmon-agent.log"
log_level: "info"
skip_ssl_verify: ${SKIP_SSL_VERIFY:-false}
integrations:
  docker: false
  compliance: "disabled"
  ssh-proxy-enabled: false
EOF

# Create credentials file
cat > /etc/patchmon/credentials.yml << EOF
# PatchMon API Credentials
# Generated on $(date)
api_id: "$API_ID"
api_key: "$API_KEY"
EOF

chmod 600 /etc/patchmon/config.yml
chmod 600 /etc/patchmon/credentials.yml

# Step 3: Download the PatchMon agent binary using API credentials
info "Downloading PatchMon agent binary..."

# Determine the binary filename based on platform and architecture
BINARY_NAME="patchmon-agent-${PATCHMON_OS}-${ARCHITECTURE}"

# Check if agent binary already exists
if [ -f "/usr/local/bin/patchmon-agent" ]; then
    warning "Agent binary already exists at /usr/local/bin/patchmon-agent"
    warning "Moving existing file out of the way for fresh installation"
    
    # Clean up old agent backups (keep only last 3)
    ls -t /usr/local/bin/patchmon-agent.backup.* 2>/dev/null | tail -n +4 | xargs -r rm -f
    
    # Move existing file out of the way
    mv /usr/local/bin/patchmon-agent /usr/local/bin/patchmon-agent.backup.$(date +%Y%m%d_%H%M%S)
    info "Moved existing agent to: /usr/local/bin/patchmon-agent.backup.$(date +%Y%m%d_%H%M%S)"
fi

# Clean up old shell script if it exists (from previous installations)
if [ -f "/usr/local/bin/patchmon-agent.sh" ]; then
    warning "Found old shell script agent, removing it..."
    rm -f /usr/local/bin/patchmon-agent.sh
    info "Removed old shell script agent"
fi

# Download the binary
curl $CURL_FLAGS \
    -H "X-API-ID: $API_ID" \
    -H "X-API-KEY: $API_KEY" \
    "$PATCHMON_URL/api/v1/hosts/agent/download?arch=$ARCHITECTURE&os=$PATCHMON_OS&force=binary" \
    -o /usr/local/bin/patchmon-agent

chmod +x /usr/local/bin/patchmon-agent

# Get the agent version from the binary
AGENT_VERSION=$(/usr/local/bin/patchmon-agent version 2>/dev/null || echo "Unknown")
info "Agent version: $AGENT_VERSION"

# Handle existing log files and create log directory
info "Setting up log directory..."

# Create log directory if it doesn't exist
mkdir -p /etc/patchmon/logs

# Handle existing log files
if [ -f "/etc/patchmon/logs/patchmon-agent.log" ]; then
    warning "Existing log file found at /etc/patchmon/logs/patchmon-agent.log"
    warning "Rotating log file for fresh start"
    
    # Rotate the log file
    mv /etc/patchmon/logs/patchmon-agent.log /etc/patchmon/logs/patchmon-agent.log.old.$(date +%Y%m%d_%H%M%S)
    info "Log file rotated to: /etc/patchmon/logs/patchmon-agent.log.old.$(date +%Y%m%d_%H%M%S)"
fi

# Step 4: Test the configuration
info "Testing API credentials and connectivity..."
if /usr/local/bin/patchmon-agent ping; then
    success "TEST: API credentials are valid and server is reachable"
else
    error "Failed to validate API credentials or reach server"
fi

# Step 5: Setup service for WebSocket connection
# Note: The service will automatically send an initial report on startup (see serve.go)
# Detect init system and create appropriate service
if command -v systemctl >/dev/null 2>&1; then
    # Systemd is available
    info "Setting up systemd service..."
    
    # Stop and disable existing service if it exists
    if systemctl is-active --quiet patchmon-agent.service 2>/dev/null; then
        warning "Stopping existing PatchMon agent service..."
        systemctl stop patchmon-agent.service
    fi
    
    if systemctl is-enabled --quiet patchmon-agent.service 2>/dev/null; then
        warning "Disabling existing PatchMon agent service..."
        systemctl disable patchmon-agent.service
    fi
    
    # Create systemd service file
    cat > /etc/systemd/system/patchmon-agent.service << EOF
[Unit]
Description=PatchMon Agent Service
After=network.target
Wants=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/patchmon-agent serve
Restart=always
RestartSec=10
WorkingDirectory=/etc/patchmon

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=patchmon-agent

[Install]
WantedBy=multi-user.target
EOF
    
    # Clean up old crontab entries if they exist (from previous installations)
    if crontab -l 2>/dev/null | grep -q "patchmon-agent"; then
        warning "Found old crontab entries, removing them..."
        crontab -l 2>/dev/null | grep -v "patchmon-agent" | crontab -
        info "Removed old crontab entries"
    fi
    
    # Reload systemd and enable/start the service
    systemctl daemon-reload
    systemctl enable patchmon-agent.service
    systemctl start patchmon-agent.service
    
    # Check if service started successfully
    if systemctl is-active --quiet patchmon-agent.service; then
        success "PatchMon Agent service started successfully"
        info "WebSocket connection established"
    else
        warning "Service may have failed to start. Check status with: systemctl status patchmon-agent"
    fi
    
    SERVICE_TYPE="systemd"
elif [ -d /etc/init.d ] && command -v rc-service >/dev/null 2>&1; then
    # OpenRC is available (Alpine Linux)
    info "Setting up OpenRC service..."
    
    # Stop and disable existing service if it exists
    if rc-service patchmon-agent status >/dev/null 2>&1; then
        warning "Stopping existing PatchMon agent service..."
        rc-service patchmon-agent stop
    fi
    
    if rc-update show default 2>/dev/null | grep -q "patchmon-agent"; then
        warning "Disabling existing PatchMon agent service..."
        rc-update del patchmon-agent default
    fi
    
    # Create OpenRC service file
    # Use supervise-daemon for automatic restart on crash/exit (available in OpenRC 0.35+, Alpine 3.9+)
    # This is critical for auto-updates: the agent exits after replacing its binary,
    # and supervise-daemon automatically restarts it with the new binary.
    cat > /etc/init.d/patchmon-agent << 'EOF'
#!/sbin/openrc-run

name="patchmon-agent"
description="PatchMon Agent Service"
command="/usr/local/bin/patchmon-agent"
command_args="serve"
command_user="root"
pidfile="/var/run/patchmon-agent.pid"
supervisor=supervise-daemon
supervise_daemon_args="--chdir /etc/patchmon"
respawn_delay=10
respawn_max=5
respawn_period=60

depend() {
    need net
    after net
}
EOF
    
    chmod +x /etc/init.d/patchmon-agent
    
    # Clean up old crontab entries if they exist (from previous installations)
    if crontab -l 2>/dev/null | grep -q "patchmon-agent"; then
        warning "Found old crontab entries, removing them..."
        crontab -l 2>/dev/null | grep -v "patchmon-agent" | crontab -
        info "Removed old crontab entries"
    fi
    
    # Enable and start the service
    rc-update add patchmon-agent default
    rc-service patchmon-agent start
    
    # Check if service started successfully
    if rc-service patchmon-agent status >/dev/null 2>&1; then
        success "PatchMon Agent service started successfully"
        info "WebSocket connection established"
    else
        warning "Service may have failed to start. Check status with: rc-service patchmon-agent status"
    fi
    
    SERVICE_TYPE="openrc"
elif [ "$(uname -s 2>/dev/null)" = "FreeBSD" ] || [ "$PATCHMON_OS" = "freebsd" ]; then
    # FreeBSD: use rc.d
    info "Setting up FreeBSD rc.d service..."
    RCD_SCRIPT="/usr/local/etc/rc.d/patchmon_agent"
    mkdir -p /usr/local/etc/rc.d
    cat > "$RCD_SCRIPT" << 'EOF'
#!/bin/sh
# PROVIDE: patchmon_agent
# REQUIRE: NETWORK
# KEYWORD: nojail

. /etc/rc.subr

name="patchmon_agent"
rcvar="${name}_enable"
pidfile="/var/run/${name}.pid"

start_cmd="${name}_start"
stop_cmd="${name}_stop"
status_cmd="${name}_status"

patchmon_agent_start()
{
    echo "Starting ${name}."
    /usr/sbin/daemon -f -P ${pidfile} -r /usr/local/bin/patchmon-agent serve
}

patchmon_agent_stop()
{
    if [ -f ${pidfile} ]; then
        echo "Stopping ${name}."
        kill $(cat ${pidfile}) 2>/dev/null
        rm -f ${pidfile}
    else
        echo "${name} is not running."
    fi
}

patchmon_agent_status()
{
    if [ -f ${pidfile} ] && kill -0 $(cat ${pidfile}) 2>/dev/null; then
        echo "${name} is running as pid $(cat ${pidfile})."
    else
        echo "${name} is not running."
        return 1
    fi
}

load_rc_config $name
run_rc_command "$1"
EOF
    chmod +x "$RCD_SCRIPT"
    if command -v service >/dev/null 2>&1; then
        service patchmon_agent enable 2>/dev/null || true
        service patchmon_agent start
        sleep 1
        if service patchmon_agent status 2>/dev/null | grep -q "running"; then
            success "PatchMon Agent service started successfully"
            info "WebSocket connection established"
        else
            warning "Service may have failed to start. Check: service patchmon_agent status"
        fi
    else
        echo "patchmon_agent_enable=\"YES\"" >> /etc/rc.conf.local 2>/dev/null || true
        "$RCD_SCRIPT" start
        success "PatchMon Agent service configured"
    fi
    SERVICE_TYPE="rc.d"
else
    # No init system detected, use crontab as fallback
    warning "No init system detected (systemd, OpenRC, or FreeBSD). Using crontab for service management."
    
    # Clean up old crontab entries if they exist
    if crontab -l 2>/dev/null | grep -q "patchmon-agent"; then
        warning "Found old crontab entries, removing them..."
        crontab -l 2>/dev/null | grep -v "patchmon-agent" | crontab -
        info "Removed old crontab entries"
    fi
    
    # Add crontab entry to run the agent
    (crontab -l 2>/dev/null; echo "@reboot /usr/local/bin/patchmon-agent serve >/dev/null 2>&1") | crontab -
    info "Added crontab entry for PatchMon agent"
    
    # Start the agent manually
    /usr/local/bin/patchmon-agent serve >/dev/null 2>&1 &
    success "PatchMon Agent started in background"
    info "WebSocket connection established"
    
    SERVICE_TYPE="crontab"
fi

# Installation complete
success "PatchMon Agent installation completed successfully!"
echo ""
printf "%b\n" "${GREEN}Installation Summary:${NC}"
echo "   • Configuration directory: /etc/patchmon"
echo "   • Agent binary installed: /usr/local/bin/patchmon-agent"
echo "   • Architecture: $ARCHITECTURE"
if [ "$(uname -s 2>/dev/null)" = "FreeBSD" ] || [ "$PATCHMON_OS" = "freebsd" ]; then
    echo "   • Dependencies installed: curl"
else
    echo "   • Dependencies installed: jq, curl, bc"
fi
if [ "$SERVICE_TYPE" = "systemd" ]; then
    echo "   • Systemd service configured and running"
elif [ "$SERVICE_TYPE" = "openrc" ]; then
    echo "   • OpenRC service configured and running"
elif [ "$SERVICE_TYPE" = "rc.d" ]; then
    echo "   • FreeBSD rc.d service configured and running"
else
    echo "   • Service configured via crontab"
fi
echo "   • API credentials configured and tested"
echo "   • WebSocket connection established"
echo "   • Logs directory: /etc/patchmon/logs"

# Check for moved files and show them
MOVED_FILES=$(ls /etc/patchmon/credentials.yml.backup.* /etc/patchmon/config.yml.backup.* /usr/local/bin/patchmon-agent.backup.* /etc/patchmon/logs/patchmon-agent.log.old.* /usr/local/bin/patchmon-agent.sh.backup.* /etc/patchmon/credentials.backup.* 2>/dev/null || true)
if [ -n "$MOVED_FILES" ]; then
    echo ""
    printf "%b\n" "${YELLOW}Files Moved for Fresh Installation:${NC}"
    echo "$MOVED_FILES" | while read -r moved_file; do
        echo "   • $moved_file"
    done
    echo ""
    printf "%b\n" "${BLUE}Note: Old files are automatically cleaned up (keeping last 3)${NC}"
fi

echo ""
printf "%b\n" "${BLUE}Management Commands:${NC}"
echo "   • Test connection: /usr/local/bin/patchmon-agent ping"
echo "   • Manual report: /usr/local/bin/patchmon-agent report"
echo "   • Check status: /usr/local/bin/patchmon-agent diagnostics"
if [ "$SERVICE_TYPE" = "systemd" ]; then
    echo "   • Service status: systemctl status patchmon-agent"
    echo "   • Service logs: journalctl -u patchmon-agent -f"
    echo "   • Restart service: systemctl restart patchmon-agent"
elif [ "$SERVICE_TYPE" = "openrc" ]; then
    echo "   • Service status: rc-service patchmon-agent status"
    echo "   • Service logs: tail -f /etc/patchmon/logs/patchmon-agent.log"
    echo "   • Restart service: rc-service patchmon-agent restart"
elif [ "$SERVICE_TYPE" = "rc.d" ]; then
    echo "   • Service status: service patchmon_agent status"
    echo "   • Service logs: tail -f /etc/patchmon/logs/patchmon-agent.log"
    echo "   • Restart service: service patchmon_agent restart"
else
    echo "   • Service logs: tail -f /etc/patchmon/logs/patchmon-agent.log"
    echo "   • Restart service: pkill -f 'patchmon-agent serve' && /usr/local/bin/patchmon-agent serve &"
fi
echo ""
success "Your system is now being monitored by PatchMon!"
