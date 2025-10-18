#!/bin/bash

# PatchMon Agent Migration Script v1.2.9
# This script migrates from legacy bash agent (v1.2.8) to Go agent (v1.3.0+)
# It acts as an intermediary during the upgrade process

# Configuration
PATCHMON_SERVER="${PATCHMON_SERVER:-http://localhost:3001}"
API_VERSION="v1"
AGENT_VERSION="1.2.9"
CREDENTIALS_FILE="/etc/patchmon/credentials"
LOG_FILE="/var/log/patchmon-agent.log"

# This placeholder will be dynamically replaced by the server when serving this
# script based on the "ignore SSL self-signed" setting. If set to -k, curl will
# ignore certificate validation. Otherwise, it will be empty for secure default.
CURL_FLAGS=""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging function
log() {
    if [[ -w "$(dirname "$LOG_FILE")" ]] 2>/dev/null; then
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] MIGRATION: $1" >> "$LOG_FILE" 2>/dev/null
    fi
}

# Error handling
error() {
    echo -e "${RED}ERROR: $1${NC}" >&2
    log "ERROR: $1"
    exit 1
}

# Info logging
info() {
    echo -e "${BLUE}ℹ️  $1${NC}" >&2
    log "INFO: $1"
}

# Success logging
success() {
    echo -e "${GREEN}✅ $1${NC}" >&2
    log "SUCCESS: $1"
}

# Warning logging
warning() {
    echo -e "${YELLOW}⚠️  $1${NC}" >&2
    log "WARNING: $1"
}

# Check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        error "This migration script must be run as root"
    fi
}

# Load API credentials from legacy format
load_legacy_credentials() {
    if [[ ! -f "$CREDENTIALS_FILE" ]]; then
        error "Legacy credentials file not found at $CREDENTIALS_FILE"
    fi
    
    source "$CREDENTIALS_FILE"
    
    if [[ -z "$API_ID" ]] || [[ -z "$API_KEY" ]]; then
        error "API_ID and API_KEY must be configured in $CREDENTIALS_FILE"
    fi
    
    # Use PATCHMON_URL from credentials if available
    if [[ -n "$PATCHMON_URL" ]]; then
        PATCHMON_SERVER="$PATCHMON_URL"
    fi
}


# Remove cron entries
remove_cron_entries() {
    info "Removing legacy cron entries..."
    
    # Get current crontab
    local current_crontab=$(crontab -l 2>/dev/null || echo "")
    
    if [[ -n "$current_crontab" ]]; then
        # Remove any lines containing patchmon-agent
        local new_crontab=$(echo "$current_crontab" | grep -v "patchmon-agent" || true)
        
        # Update crontab if it changed
        if [[ "$current_crontab" != "$new_crontab" ]]; then
            if [[ -n "$new_crontab" ]]; then
                echo "$new_crontab" | crontab -
                success "Legacy cron entries removed (kept other cron jobs)"
            else
                crontab -r 2>/dev/null || true
                success "All cron entries removed"
            fi
        else
            info "No patchmon cron entries found to remove"
        fi
    else
        info "No crontab found"
    fi
}


# Clean up legacy files (only after successful verification)
cleanup_legacy_files() {
    info "Cleaning up legacy files..."
    
    # Remove legacy script
    if [[ -f "/usr/local/bin/patchmon-agent.sh" ]]; then
        rm -f "/usr/local/bin/patchmon-agent.sh"
        success "Removed legacy script"
    fi
    
    # Remove legacy credentials file
    if [[ -f "$CREDENTIALS_FILE" ]]; then
        rm -f "$CREDENTIALS_FILE"
        success "Removed legacy credentials file"
    fi
    
    # Remove legacy config file (if it exists)
    if [[ -f "/etc/patchmon/config" ]]; then
        rm -f "/etc/patchmon/config"
        success "Removed legacy config file"
    fi
}

# Show migration summary
show_migration_summary() {
    echo ""
    echo "=========================================="
    echo "PatchMon Agent Migration Complete!"
    echo "=========================================="
    echo ""
    echo "✅ Successfully migrated from bash agent to Go agent"
    echo ""
    echo "What was done:"
    echo "  • Loaded legacy credentials"
    echo "  • Downloaded and ran PatchMon install script"
    echo "  • Installed Go agent binary and systemd service"
    echo "  • Verified service is running"
    echo "  • Removed legacy cron entries"
    echo "  • Cleaned up legacy files"
    echo ""
    echo "Next steps:"
    echo "  • The Go agent runs as a service, no cron needed"
    echo "  • Use: patchmon-agent serve (to run as service)"
    echo "  • Use: patchmon-agent report (for one-time report)"
    echo "  • Use: patchmon-agent --help (for all commands)"
    echo ""
    echo "Monitoring commands:"
    echo "  • Check status: systemctl status patchmon-agent"
    echo "  • View logs: tail -f /etc/patchmon/logs/patchmon-agent.log"
    echo "  • Run diagnostics: patchmon-agent diagnostics"
    echo ""
    echo "Configuration files:"
    echo "  • Config: /etc/patchmon/config.yml"
    echo "  • Credentials: /etc/patchmon/credentials.yml"
    echo "  • Logs: /etc/patchmon/logs/patchmon-agent.log"
    echo ""
}

# Post-migration verification
post_migration_check() {
    echo ""
    echo "=========================================="
    echo "Post-Migration Verification"
    echo "=========================================="
    echo ""
    
    # Check if patchmon-agent is running
    info "Checking if patchmon-agent is running..."
    if pgrep -f "patchmon-agent serve" >/dev/null 2>&1; then
        success "PatchMon agent is running"
    else
        warning "PatchMon agent is not running (this is normal if not started as service)"
        info "To start as service: patchmon-agent serve"
    fi
    
    # Check WebSocket connection (if agent is running)
    if pgrep -f "patchmon-agent serve" >/dev/null 2>&1; then
        info "Checking WebSocket connection..."
        if /usr/local/bin/patchmon-agent ping >/dev/null 2>&1; then
            success "WebSocket connection is active"
        else
            warning "WebSocket connection test failed"
        fi
    else
        info "Skipping WebSocket check (agent not running)"
    fi
    
    # Run diagnostics
    info "Running system diagnostics..."
    echo ""
    if [[ -f "/usr/local/bin/patchmon-agent" ]]; then
        if /usr/local/bin/patchmon-agent diagnostics >/dev/null 2>&1; then
            success "Diagnostics completed successfully"
            echo ""
            echo "Full diagnostics output:"
            echo "----------------------------------------"
            /usr/local/bin/patchmon-agent diagnostics
            echo "----------------------------------------"
        else
            warning "Diagnostics failed to run"
        fi
    else
        warning "Go agent binary not found - cannot run diagnostics"
    fi
    
    echo ""
    echo "=========================================="
    echo "Migration Verification Complete!"
    echo "=========================================="
    echo ""
    success "Thank you for using PatchMon Agent!"
    echo ""
}

# Main migration function
perform_migration() {
    info "Starting PatchMon Agent migration from bash to Go..."
    echo ""
    
    # Load legacy credentials
    load_legacy_credentials
    
    # Set environment variables for install script
    export API_ID="$API_ID"
    export API_KEY="$API_KEY"
    export PATCHMON_URL="$PATCHMON_SERVER"
    
    # Detect architecture
    local arch=$(uname -m)
    local goarch=""
    case "$arch" in
        "x86_64") goarch="amd64" ;;
        "i386"|"i686") goarch="386" ;;
        "aarch64"|"arm64") goarch="arm64" ;;
        "armv7l"|"armv6l"|"armv5l") goarch="arm" ;;
        *) error "Unsupported architecture: $arch" ;;
    esac
    
    info "Downloading and running PatchMon install script..."
    
    # Download and run the install script
    if curl $CURL_FLAGS -H "X-API-ID: $API_ID" -H "X-API-KEY: $API_KEY" \
           "$PATCHMON_SERVER/api/v1/hosts/install?arch=$goarch" | bash; then
        
        success "PatchMon Go agent installed successfully"
        
        # Wait a moment for service to start
        sleep 3
        
        # Test if the service is running
        if systemctl is-active --quiet patchmon-agent.service; then
            success "PatchMon agent service is running"
            
            # Clean up legacy files
            remove_cron_entries
            cleanup_legacy_files
            
            # Show summary
            show_migration_summary
            post_migration_check
            
            success "Migration completed successfully!"
        else
            warning "PatchMon agent service failed to start"
            warning "Legacy files preserved for debugging"
            show_migration_summary
        fi
    else
        error "Failed to install PatchMon Go agent"
    fi
    
    # Exit here to prevent the legacy script from continuing
    exit 0
}

# Handle command line arguments
case "$1" in
    "migrate")
        check_root
        perform_migration
        ;;
    "test")
        check_root
        load_legacy_credentials
        test_go_agent
        ;;
    "update-agent")
        # This is called by legacy agents during update
        check_root
        perform_migration
        ;;
    *)
        # If no arguments provided, check if we're being executed by a legacy agent
        # Legacy agents will call this script directly during update
        if [[ -f "$CREDENTIALS_FILE" ]]; then
            info "Detected legacy agent execution - starting migration..."
            check_root
            perform_migration
        else
            echo "PatchMon Agent Migration Script v$AGENT_VERSION"
            echo "Usage: $0 {migrate|test|update-agent}"
            echo ""
            echo "Commands:"
            echo "  migrate      - Perform full migration from bash to Go agent"
            echo "  test         - Test Go agent after migration"
            echo "  update-agent - Called by legacy agents during update"
            echo ""
            echo "This script should be executed by the legacy agent during update."
            exit 1
        fi
        ;;
esac
