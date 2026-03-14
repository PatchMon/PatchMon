#!/bin/sh

# PatchMon Agent Removal Script
# POSIX-compliant shell script (works with dash, ash, bash, etc.)
# Usage: curl -s {PATCHMON_URL}/api/v1/hosts/remove | sudo sh
#        curl -s {PATCHMON_URL}/api/v1/hosts/remove | sudo REMOVE_BACKUPS=1 sh
#        curl -s {PATCHMON_URL}/api/v1/hosts/remove | sudo SILENT=1 sh
# This script completely removes PatchMon from the system

set -e

# This placeholder will be dynamically replaced by the server when serving this
# script based on the "ignore SSL self-signed" setting for any curl calls in
# future (left for consistency with install script).
CURL_FLAGS=""

# Detect if running in silent mode (only with explicit SILENT env var)
SILENT_MODE=0
if [ -n "$SILENT" ]; then
    SILENT_MODE=1
fi

# Check if backup files should be removed (default: preserve for safety)
# Usage: REMOVE_BACKUPS=1 when piping the script
REMOVE_BACKUPS="${REMOVE_BACKUPS:-0}"

# Colors for output (disabled in silent mode)
if [ "$SILENT_MODE" -eq 0 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

# Functions
error() {
    printf "%b\n" "${RED}❌ ERROR: $1${NC}" >&2
    exit 1
}

info() {
    if [ "$SILENT_MODE" -eq 0 ]; then
        printf "%b\n" "${BLUE}ℹ️  $1${NC}"
    fi
}

success() {
    if [ "$SILENT_MODE" -eq 0 ]; then
        printf "%b\n" "${GREEN}✅ $1${NC}"
    fi
}

warning() {
    if [ "$SILENT_MODE" -eq 0 ]; then
        printf "%b\n" "${YELLOW}⚠️  $1${NC}"
    fi
}

# Check if running as root
if [ "$(id -u)" -ne 0 ]; then
   error "This script must be run as root (use sudo)"
fi

info "🗑️  Starting PatchMon Agent Removal..."
[ "$SILENT_MODE" -eq 0 ] && echo ""

# Step 1: Stop systemd/OpenRC service if it exists
info "🛑 Stopping PatchMon service..."
SERVICE_STOPPED=0

# Check for systemd service
if command -v systemctl >/dev/null 2>&1; then
    info "📋 Checking systemd service status..."
    
    # Check if service is active
    if systemctl is-active --quiet patchmon-agent.service 2>/dev/null; then
        SERVICE_STATUS=$(systemctl is-active patchmon-agent.service 2>/dev/null || echo "unknown")
        warning "Service is active (status: $SERVICE_STATUS). Stopping it now..."
        if systemctl stop patchmon-agent.service 2>/dev/null; then
            success "✓ Service stopped successfully"
            SERVICE_STOPPED=1
        else
            warning "✗ Failed to stop service (continuing anyway...)"
        fi
        # Verify it stopped
        sleep 1
        if systemctl is-active --quiet patchmon-agent.service 2>/dev/null; then
            warning "⚠️  Service is STILL ACTIVE after stop command!"
        else
            info "✓ Verified: Service is no longer active"
        fi
    else
        info "Service is not active"
    fi
    
    # Check if service is enabled
    if systemctl is-enabled --quiet patchmon-agent.service 2>/dev/null; then
        ENABLED_STATUS=$(systemctl is-enabled patchmon-agent.service 2>/dev/null || echo "unknown")
        warning "Service is enabled (status: $ENABLED_STATUS). Disabling it now..."
        if systemctl disable patchmon-agent.service 2>/dev/null; then
            success "✓ Service disabled successfully"
        else
            warning "✗ Failed to disable service (may already be disabled)"
        fi
    else
        info "Service is not enabled"
    fi
    
    # Check for service file
    if [ -f "/etc/systemd/system/patchmon-agent.service" ]; then
        warning "Found service file: /etc/systemd/system/patchmon-agent.service"
        info "Removing service file..."
        if rm -f /etc/systemd/system/patchmon-agent.service 2>/dev/null; then
            success "✓ Service file removed"
        else
            warning "✗ Failed to remove service file (check permissions)"
        fi
        
        info "Reloading systemd daemon..."
        if systemctl daemon-reload 2>/dev/null; then
            success "✓ Systemd daemon reloaded"
        else
            warning "✗ Failed to reload systemd daemon"
        fi
        
        SERVICE_STOPPED=1
        
        # Verify the file is gone
        if [ -f "/etc/systemd/system/patchmon-agent.service" ]; then
            warning "⚠️  Service file STILL EXISTS after removal!"
        else
            info "✓ Verified: Service file removed"
        fi
    else
        info "Service file not found at /etc/systemd/system/patchmon-agent.service"
    fi
    
    # Final status check
    info "📊 Final systemd status check..."
    FINAL_STATUS=$(systemctl is-active patchmon-agent.service 2>&1 || echo "not-found")
    info "Service status: $FINAL_STATUS"
fi

# Check for OpenRC service (Alpine Linux)
if command -v rc-service >/dev/null 2>&1; then
    if rc-service patchmon-agent status >/dev/null 2>&1; then
        warning "Stopping OpenRC service..."
        rc-service patchmon-agent stop || true
        SERVICE_STOPPED=1
    fi
    
    if rc-update show default 2>/dev/null | grep -q "patchmon-agent"; then
        warning "Removing from runlevel..."
        rc-update del patchmon-agent default || true
    fi
    
    if [ -f "/etc/init.d/patchmon-agent" ]; then
        warning "Removing OpenRC service file..."
        rm -f /etc/init.d/patchmon-agent
        success "OpenRC service removed"
        SERVICE_STOPPED=1
    fi
fi

# Stop any remaining running processes (legacy or manual starts)
info "🔍 Checking for running PatchMon processes..."
if pgrep -f "patchmon-agent" >/dev/null; then
    PROCESS_COUNT=$(pgrep -f "patchmon-agent" | wc -l | tr -d ' ')
    warning "Found $PROCESS_COUNT running PatchMon process(es)"
    
    # Show process details
    if [ "$SILENT_MODE" -eq 0 ]; then
        info "Process details:"
        ps aux | grep "[p]atchmon-agent" | while IFS= read -r line; do
            echo "   $line"
        done
    fi
    
    warning "Sending SIGTERM to all patchmon-agent processes..."
    if pkill -f "patchmon-agent" 2>/dev/null; then
        success "✓ Sent SIGTERM signal"
    else
        warning "Failed to send SIGTERM (processes may have already stopped)"
    fi
    
    sleep 2
    
    # Check if processes still exist
    if pgrep -f "patchmon-agent" >/dev/null; then
        REMAINING=$(pgrep -f "patchmon-agent" | wc -l | tr -d ' ')
        warning "⚠️  $REMAINING process(es) still running! Sending SIGKILL..."
        pkill -9 -f "patchmon-agent" 2>/dev/null || true
        sleep 1
        
        if pgrep -f "patchmon-agent" >/dev/null; then
            warning "⚠️  CRITICAL: Processes still running after SIGKILL!"
        else
            success "✓ All processes terminated"
        fi
    else
        success "✓ All processes stopped successfully"
    fi
    
    SERVICE_STOPPED=1
else
    info "No running PatchMon processes found"
fi

if [ "$SERVICE_STOPPED" -eq 1 ]; then
    success "PatchMon service/processes stopped"
else
    info "No running PatchMon service or processes found"
fi

# Step 2: Remove crontab entries
info "📅 Removing PatchMon crontab entries..."
if crontab -l 2>/dev/null | grep -q "patchmon-agent"; then
    warning "Found PatchMon crontab entries, removing them..."
    crontab -l 2>/dev/null | grep -v "patchmon-agent" | crontab -
    success "Crontab entries removed"
else
    info "No PatchMon crontab entries found"
fi

# Step 3: Remove agent binaries and scripts
info "📄 Removing agent binaries and scripts..."
AGENTS_REMOVED=0

# Remove Go agent binary
if [ -f "/usr/local/bin/patchmon-agent" ]; then
    warning "Removing Go agent binary: /usr/local/bin/patchmon-agent"
    rm -f /usr/local/bin/patchmon-agent
    AGENTS_REMOVED=1
fi

# Remove legacy shell script agent
if [ -f "/usr/local/bin/patchmon-agent.sh" ]; then
    warning "Removing legacy agent script: /usr/local/bin/patchmon-agent.sh"
    rm -f /usr/local/bin/patchmon-agent.sh
    AGENTS_REMOVED=1
fi

# Remove backup files for Go agent
if ls /usr/local/bin/patchmon-agent.backup.* >/dev/null 2>&1; then
    warning "Removing Go agent backup files..."
    rm -f /usr/local/bin/patchmon-agent.backup.*
    AGENTS_REMOVED=1
fi

# Remove backup files for legacy shell script
if ls /usr/local/bin/patchmon-agent.sh.backup.* >/dev/null 2>&1; then
    warning "Removing legacy agent backup files..."
    rm -f /usr/local/bin/patchmon-agent.sh.backup.*
    AGENTS_REMOVED=1
fi

if [ "$AGENTS_REMOVED" -eq 1 ]; then
    success "Agent binaries and scripts removed"
else
    info "No agent binaries or scripts found"
fi

# Step 4: Remove configuration directory and files
info "📁 Removing configuration files..."
if [ -d "/etc/patchmon" ]; then
    warning "Removing configuration directory: /etc/patchmon"
    
    # Show what's being removed (only in verbose mode)
    if [ "$SILENT_MODE" -eq 0 ]; then
        info "📋 Files in /etc/patchmon:"
        ls -la /etc/patchmon/ 2>/dev/null | grep -v "^total" | while read -r line; do
            echo "   $line"
        done
    fi
    
    # Remove the directory
    rm -rf /etc/patchmon
    success "Configuration directory removed"
else
    info "Configuration directory not found"
fi

# Step 5: Remove log files
info "📝 Removing log files..."
if [ -f "/var/log/patchmon-agent.log" ]; then
    warning "Removing log file: /var/log/patchmon-agent.log"
    rm -f /var/log/patchmon-agent.log
    success "Log file removed"
else
    info "Log file not found"
fi

# Step 6: Clean up backup files
info "🧹 Checking backup files..."
BACKUP_COUNT=0
BACKUP_REMOVED=0

if [ "$REMOVE_BACKUPS" -eq 1 ]; then
    info "Removing backup files (REMOVE_BACKUPS=1)..."
    
    # Remove credential backups (already removed with /etc/patchmon directory, but check anyway)
    if ls /etc/patchmon/credentials.backup.* >/dev/null 2>&1; then
        CRED_BACKUPS=$(ls /etc/patchmon/credentials.backup.* 2>/dev/null | wc -l | tr -d ' ')
        warning "Removing $CRED_BACKUPS credential backup file(s)..."
        rm -f /etc/patchmon/credentials.backup.*
        BACKUP_COUNT=$((BACKUP_COUNT + CRED_BACKUPS))
        BACKUP_REMOVED=1
    fi
    
    # Remove config backups (already removed with /etc/patchmon directory, but check anyway)
    if ls /etc/patchmon/*.backup.* >/dev/null 2>&1; then
        CONFIG_BACKUPS=$(ls /etc/patchmon/*.backup.* 2>/dev/null | wc -l | tr -d ' ')
        warning "Removing $CONFIG_BACKUPS config backup file(s)..."
        rm -f /etc/patchmon/*.backup.*
        BACKUP_COUNT=$((BACKUP_COUNT + CONFIG_BACKUPS))
        BACKUP_REMOVED=1
    fi
    
    # Remove Go agent backups
    if ls /usr/local/bin/patchmon-agent.backup.* >/dev/null 2>&1; then
        GO_AGENT_BACKUPS=$(ls /usr/local/bin/patchmon-agent.backup.* 2>/dev/null | wc -l | tr -d ' ')
        warning "Removing $GO_AGENT_BACKUPS Go agent backup file(s)..."
        rm -f /usr/local/bin/patchmon-agent.backup.*
        BACKUP_COUNT=$((BACKUP_COUNT + GO_AGENT_BACKUPS))
        BACKUP_REMOVED=1
    fi
    
    # Remove legacy shell agent backups
    if ls /usr/local/bin/patchmon-agent.sh.backup.* >/dev/null 2>&1; then
        SHELL_AGENT_BACKUPS=$(ls /usr/local/bin/patchmon-agent.sh.backup.* 2>/dev/null | wc -l | tr -d ' ')
        warning "Removing $SHELL_AGENT_BACKUPS legacy agent backup file(s)..."
        rm -f /usr/local/bin/patchmon-agent.sh.backup.*
        BACKUP_COUNT=$((BACKUP_COUNT + SHELL_AGENT_BACKUPS))
        BACKUP_REMOVED=1
    fi
    
    # Remove log backups
    if ls /var/log/patchmon-agent.log.old.* >/dev/null 2>&1; then
        LOG_BACKUPS=$(ls /var/log/patchmon-agent.log.old.* 2>/dev/null | wc -l | tr -d ' ')
        warning "Removing $LOG_BACKUPS log backup file(s)..."
        rm -f /var/log/patchmon-agent.log.old.*
        BACKUP_COUNT=$((BACKUP_COUNT + LOG_BACKUPS))
        BACKUP_REMOVED=1
    fi
    
    if [ "$BACKUP_REMOVED" -eq 1 ]; then
        success "Removed $BACKUP_COUNT backup file(s)"
    else
        info "No backup files found to remove"
    fi
else
    # Just count backup files without removing
    CRED_BACKUPS=0
    CONFIG_BACKUPS=0
    GO_AGENT_BACKUPS=0
    SHELL_AGENT_BACKUPS=0
    LOG_BACKUPS=0
    
    if ls /etc/patchmon/credentials.backup.* >/dev/null 2>&1; then
        CRED_BACKUPS=$(ls /etc/patchmon/credentials.backup.* 2>/dev/null | wc -l | tr -d ' ')
    fi
    
    if ls /etc/patchmon/*.backup.* >/dev/null 2>&1; then
        CONFIG_BACKUPS=$(ls /etc/patchmon/*.backup.* 2>/dev/null | wc -l | tr -d ' ')
    fi
    
    if ls /usr/local/bin/patchmon-agent.backup.* >/dev/null 2>&1; then
        GO_AGENT_BACKUPS=$(ls /usr/local/bin/patchmon-agent.backup.* 2>/dev/null | wc -l | tr -d ' ')
    fi
    
    if ls /usr/local/bin/patchmon-agent.sh.backup.* >/dev/null 2>&1; then
        SHELL_AGENT_BACKUPS=$(ls /usr/local/bin/patchmon-agent.sh.backup.* 2>/dev/null | wc -l | tr -d ' ')
    fi
    
    if ls /var/log/patchmon-agent.log.old.* >/dev/null 2>&1; then
        LOG_BACKUPS=$(ls /var/log/patchmon-agent.log.old.* 2>/dev/null | wc -l | tr -d ' ')
    fi
    
    BACKUP_COUNT=$((CRED_BACKUPS + CONFIG_BACKUPS + GO_AGENT_BACKUPS + SHELL_AGENT_BACKUPS + LOG_BACKUPS))
    
    if [ "$BACKUP_COUNT" -gt 0 ]; then
        info "Found $BACKUP_COUNT backup file(s) - preserved for safety"
        if [ "$SILENT_MODE" -eq 0 ]; then
            printf "%b\n" "${BLUE}💡 To remove backups, run with: REMOVE_BACKUPS=1${NC}"
        fi
    else
        info "No backup files found"
    fi
fi

# Step 7: Final verification
info "🔍 Verifying removal..."
REMAINING_FILES=0

if [ -f "/usr/local/bin/patchmon-agent" ]; then
    REMAINING_FILES=$((REMAINING_FILES + 1))
fi

if [ -f "/usr/local/bin/patchmon-agent.sh" ]; then
    REMAINING_FILES=$((REMAINING_FILES + 1))
fi

if [ -f "/etc/systemd/system/patchmon-agent.service" ]; then
    REMAINING_FILES=$((REMAINING_FILES + 1))
fi

if [ -f "/etc/init.d/patchmon-agent" ]; then
    REMAINING_FILES=$((REMAINING_FILES + 1))
fi

if [ -d "/etc/patchmon" ]; then
    REMAINING_FILES=$((REMAINING_FILES + 1))
fi

if [ -f "/var/log/patchmon-agent.log" ]; then
    REMAINING_FILES=$((REMAINING_FILES + 1))
fi

if crontab -l 2>/dev/null | grep -q "patchmon-agent"; then
    REMAINING_FILES=$((REMAINING_FILES + 1))
fi

if [ "$REMAINING_FILES" -eq 0 ]; then
    success "✅ PatchMon has been completely removed from the system!"
else
    warning "⚠️  Some PatchMon files may still remain ($REMAINING_FILES items)"
    if [ "$SILENT_MODE" -eq 0 ]; then
        printf "%b\n" "${BLUE}💡 You may need to remove them manually${NC}"
    fi
fi

if [ "$SILENT_MODE" -eq 0 ]; then
    echo ""
    printf "%b\n" "${GREEN}📋 Removal Summary:${NC}"
    echo "   • Agent binaries: Removed"
    echo "   • System services: Removed (systemd/OpenRC)"
    echo "   • Configuration files: Removed"
    echo "   • Log files: Removed"
    echo "   • Crontab entries: Removed"
    echo "   • Running processes: Stopped"
    if [ "$REMOVE_BACKUPS" -eq 1 ]; then
        echo "   • Backup files: Removed"
    else
        echo "   • Backup files: Preserved (${BACKUP_COUNT} files)"
    fi
    echo ""
    if [ "$REMOVE_BACKUPS" -eq 0 ] && [ "$BACKUP_COUNT" -gt 0 ]; then
        printf "%b\n" "${BLUE}🔧 Manual cleanup (if needed):${NC}"
        echo "   • Remove backup files: curl -s \${PATCHMON_URL}/api/v1/hosts/remove | sudo REMOVE_BACKUPS=1 sh"
        echo ""
    fi
fi
success "🎉 PatchMon removal completed!"
