#!/bin/sh

# PatchMon Agent Removal Script
# POSIX-compliant shell script (works with dash, ash, bash, etc.)
# Usage: curl -s {PATCHMON_URL}/api/v1/hosts/remove | sh
# This script completely removes PatchMon from the system

set -e

# This placeholder will be dynamically replaced by the server when serving this
# script based on the "ignore SSL self-signed" setting for any curl calls in
# future (left for consistency with install script).
CURL_FLAGS=""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Functions
error() {
    printf "%b\n" "${RED}❌ ERROR: $1${NC}" >&2
    exit 1
}

info() {
    printf "%b\n" "${BLUE}ℹ️  $1${NC}"
}

success() {
    printf "%b\n" "${GREEN}✅ $1${NC}"
}

warning() {
    printf "%b\n" "${YELLOW}⚠️  $1${NC}"
}

# Check if running as root
if [ "$(id -u)" -ne 0 ]; then
   error "This script must be run as root (use sudo)"
fi

info "🗑️  Starting PatchMon Agent Removal..."
echo ""

# Step 1: Stop any running PatchMon processes
info "🛑 Stopping PatchMon processes..."
if pgrep -f "patchmon-agent.sh" >/dev/null; then
    warning "Found running PatchMon processes, stopping them..."
    pkill -f "patchmon-agent.sh" || true
    sleep 2
    success "PatchMon processes stopped"
else
    info "No running PatchMon processes found"
fi

# Step 2: Remove crontab entries
info "📅 Removing PatchMon crontab entries..."
if crontab -l 2>/dev/null | grep -q "patchmon-agent.sh"; then
    warning "Found PatchMon crontab entries, removing them..."
    crontab -l 2>/dev/null | grep -v "patchmon-agent.sh" | crontab -
    success "Crontab entries removed"
else
    info "No PatchMon crontab entries found"
fi

# Step 3: Remove agent script
info "📄 Removing agent script..."
if [ -f "/usr/local/bin/patchmon-agent.sh" ]; then
    warning "Removing agent script: /usr/local/bin/patchmon-agent.sh"
    rm -f /usr/local/bin/patchmon-agent.sh
    success "Agent script removed"
else
    info "Agent script not found"
fi

# Step 4: Remove configuration directory and files
info "📁 Removing configuration files..."
if [ -d "/etc/patchmon" ]; then
    warning "Removing configuration directory: /etc/patchmon"
    
    # Show what's being removed
    info "📋 Files in /etc/patchmon:"
    ls -la /etc/patchmon/ 2>/dev/null | grep -v "^total" | while read -r line; do
        echo "   $line"
    done
    
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

# Step 6: Clean up backup files (optional)
info "🧹 Cleaning up backup files..."
BACKUP_COUNT=0

# Count credential backups
CRED_BACKUPS=$(ls /etc/patchmon/credentials.backup.* 2>/dev/null | wc -l || echo "0")
if [ "$CRED_BACKUPS" -gt 0 ]; then
    BACKUP_COUNT=$((BACKUP_COUNT + CRED_BACKUPS))
fi

# Count agent backups
AGENT_BACKUPS=$(ls /usr/local/bin/patchmon-agent.sh.backup.* 2>/dev/null | wc -l || echo "0")
if [ "$AGENT_BACKUPS" -gt 0 ]; then
    BACKUP_COUNT=$((BACKUP_COUNT + AGENT_BACKUPS))
fi

# Count log backups
LOG_BACKUPS=$(ls /var/log/patchmon-agent.log.old.* 2>/dev/null | wc -l || echo "0")
if [ "$LOG_BACKUPS" -gt 0 ]; then
    BACKUP_COUNT=$((BACKUP_COUNT + LOG_BACKUPS))
fi

if [ "$BACKUP_COUNT" -gt 0 ]; then
    warning "Found $BACKUP_COUNT backup files"
    echo ""
    printf "%b\n" "${YELLOW}📋 Backup files found:${NC}"
    
    # Show credential backups
    if [ "$CRED_BACKUPS" -gt 0 ]; then
        echo "   Credential backups:"
        ls /etc/patchmon/credentials.backup.* 2>/dev/null | while read -r file; do
            echo "     • $file"
        done
    fi
    
    # Show agent backups
    if [ "$AGENT_BACKUPS" -gt 0 ]; then
        echo "   Agent script backups:"
        ls /usr/local/bin/patchmon-agent.sh.backup.* 2>/dev/null | while read -r file; do
            echo "     • $file"
        done
    fi
    
    # Show log backups
    if [ "$LOG_BACKUPS" -gt 0 ]; then
        echo "   Log file backups:"
        ls /var/log/patchmon-agent.log.old.* 2>/dev/null | while read -r file; do
            echo "     • $file"
        done
    fi
    
    echo ""
    printf "%b\n" "${BLUE}💡 Note: Backup files are preserved for safety${NC}"
    printf "%b\n" "${BLUE}💡 You can remove them manually if not needed${NC}"
else
    info "No backup files found"
fi

# Step 7: Remove dependencies (optional)
info "📦 Checking for PatchMon-specific dependencies..."
if command -v jq >/dev/null 2>&1; then
    warning "jq is installed (used by PatchMon)"
    printf "%b\n" "${BLUE}💡 Note: jq may be used by other applications${NC}"
    printf "%b\n" "${BLUE}💡 Consider keeping it unless you're sure it's not needed${NC}"
else
    info "jq not found"
fi

if command -v curl >/dev/null 2>&1; then
    warning "curl is installed (used by PatchMon)"
    printf "%b\n" "${BLUE}💡 Note: curl is commonly used by many applications${NC}"
    printf "%b\n" "${BLUE}💡 Consider keeping it unless you're sure it's not needed${NC}"
else
    info "curl not found"
fi

# Step 8: Final verification
info "🔍 Verifying removal..."
REMAINING_FILES=0

if [ -f "/usr/local/bin/patchmon-agent.sh" ]; then
    REMAINING_FILES=$((REMAINING_FILES + 1))
fi

if [ -d "/etc/patchmon" ]; then
    REMAINING_FILES=$((REMAINING_FILES + 1))
fi

if [ -f "/var/log/patchmon-agent.log" ]; then
    REMAINING_FILES=$((REMAINING_FILES + 1))
fi

if crontab -l 2>/dev/null | grep -q "patchmon-agent.sh"; then
    REMAINING_FILES=$((REMAINING_FILES + 1))
fi

if [ "$REMAINING_FILES" -eq 0 ]; then
    success "✅ PatchMon has been completely removed from the system!"
else
    warning "⚠️  Some PatchMon files may still remain ($REMAINING_FILES items)"
    printf "%b\n" "${BLUE}💡 You may need to remove them manually${NC}"
fi

echo ""
printf "%b\n" "${GREEN}📋 Removal Summary:${NC}"
echo "   • Agent script: Removed"
echo "   • Configuration files: Removed"
echo "   • Log files: Removed"
echo "   • Crontab entries: Removed"
echo "   • Running processes: Stopped"
echo "   • Backup files: Preserved (if any)"
echo ""
printf "%b\n" "${BLUE}🔧 Manual cleanup (if needed):${NC}"
echo "   • Remove backup files: rm /etc/patchmon/credentials.backup.* /usr/local/bin/patchmon-agent.sh.backup.* /var/log/patchmon-agent.log.old.*"
echo "   • Remove dependencies: apt remove jq curl (if not needed by other apps)"
echo ""
success "🎉 PatchMon removal completed!"
